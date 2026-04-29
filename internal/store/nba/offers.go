package nba

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FA offer status constants — kept in sync with the CHECK constraint in migration 008.
const (
	OfferPendingAgent   = "pending_agent"
	OfferAgentRejected  = "agent_rejected"
	OfferAgentCountered = "agent_countered"
	OfferPendingTeam    = "pending_team"
	OfferAwaitingMatch  = "awaiting_match"
	OfferMatched        = "matched"
	OfferWalked         = "walked"
	OfferFinalized      = "finalized"
	OfferWithdrawn      = "withdrawn"
)

// Match window length per CBA.
const MatchWindowDuration = 48 * time.Hour

// Offer represents a single contract offer in the FA flow.
type Offer struct {
	ID                  string
	PlayerID            string
	AgencyID            *string
	OfferingTeamID      string
	BirdRightsTeamID    *string
	Years               int
	StartingSalary      float64
	RaisePct            float64
	ExceptionUsed       string
	Notes               string
	ParentOfferID       *string
	Status              string
	SubmittedBy         string
	DecidedBy           *string
	SubmittedAt         time.Time
	DecidedAt           *time.Time
	MatchWindowOpensAt  *time.Time
	MatchWindowClosesAt *time.Time
	FinalizedAt         *time.Time

	// Hydrated for display by some queries
	PlayerName       string
	AgencyName       string
	OfferingTeamName string
	BirdRightsTeamName string
}

// TotalValue computes the contract's total value across all years given the raise %.
// Year 1 = starting_salary; year N = starting_salary * (1 + raise_pct/100)^(N-1).
func (o *Offer) TotalValue() float64 {
	total := 0.0
	salary := o.StartingSalary
	for y := 0; y < o.Years; y++ {
		total += salary
		salary *= (1.0 + o.RaisePct/100.0)
	}
	return total
}

// AAV is the average annual value of the offer.
func (o *Offer) AAV() float64 {
	if o.Years == 0 {
		return 0
	}
	return o.TotalValue() / float64(o.Years)
}

// AnnualSalaries returns the year-by-year salary list (length == Years).
func (o *Offer) AnnualSalaries() []float64 {
	out := make([]float64, o.Years)
	salary := o.StartingSalary
	for y := 0; y < o.Years; y++ {
		out[y] = salary
		salary *= (1.0 + o.RaisePct/100.0)
	}
	return out
}

// CreateOfferParams is the input shape for SubmitOffer.
type CreateOfferParams struct {
	PlayerID       string
	OfferingTeamID string
	SubmittedBy    string // user_id
	Years          int
	StartingSalary float64
	RaisePct       float64
	ExceptionUsed  string
	Notes          string
}

// SubmitOffer creates a new pending_agent offer. Resolves the player's current agency
// and bird-rights team automatically. Logs an event row.
func SubmitOffer(nbaDB *pgxpool.Pool, p CreateOfferParams) (*Offer, error) {
	ctx := context.Background()
	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Resolve player's agency + bird-rights team (current team_id, only if Pending)
	var agencyID *string
	var birdTeamID *string
	var faClass string
	err = tx.QueryRow(ctx, `
		SELECT agency_id::TEXT, team_id::TEXT, fa_class
		FROM players WHERE id = $1
	`, p.PlayerID).Scan(&agencyID, &birdTeamID, &faClass)
	if err != nil {
		return nil, fmt.Errorf("resolving player: %w", err)
	}

	// Bird-rights team only matters for Pending players (UFA cap hold or RFA QO).
	// Free Agents have no current team, so no match window.
	if faClass != "Pending" {
		birdTeamID = nil
	}

	var offerID string
	err = tx.QueryRow(ctx, `
		INSERT INTO fa_offers
		    (player_id, agency_id, offering_team_id, bird_rights_team_id,
		     years, starting_salary, raise_pct, exception_used, notes,
		     status, submitted_by)
		VALUES ($1, $2::uuid, $3, $4::uuid, $5, $6, $7, $8, $9, 'pending_agent', $10)
		RETURNING id
	`, p.PlayerID, agencyID, p.OfferingTeamID, birdTeamID,
		p.Years, p.StartingSalary, p.RaisePct, p.ExceptionUsed, p.Notes,
		p.SubmittedBy).Scan(&offerID)
	if err != nil {
		return nil, fmt.Errorf("inserting offer: %w", err)
	}

	if err := logEvent(ctx, tx, offerID, p.SubmittedBy, "submitted", p.Notes); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return GetOffer(nbaDB, offerID)
}

// AgentAccept transitions a pending_agent offer.
//   - If the offer has a bird_rights_team_id (RFA / Pending) → status awaiting_match,
//     match window timestamps set.
//   - Otherwise (Free Agent / no bird rights) → status finalized, contract written immediately.
func AgentAccept(nbaDB *pgxpool.Pool, offerID, actorUserID, notes string) error {
	ctx := context.Background()
	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var birdTeamID *string
	if err := tx.QueryRow(ctx,
		`SELECT status, bird_rights_team_id::TEXT FROM fa_offers WHERE id = $1 FOR UPDATE`,
		offerID).Scan(&status, &birdTeamID); err != nil {
		return err
	}
	if status != OfferPendingAgent && status != OfferPendingTeam {
		return fmt.Errorf("offer not in agent-actionable state (got %s)", status)
	}

	if birdTeamID != nil && *birdTeamID != "" {
		// RFA: open 48hr match window
		opensAt := time.Now().UTC()
		closesAt := opensAt.Add(MatchWindowDuration)
		if _, err := tx.Exec(ctx, `
			UPDATE fa_offers SET status = $1, decided_by = $2, decided_at = NOW(),
			    match_window_opens_at = $3, match_window_closes_at = $4
			WHERE id = $5
		`, OfferAwaitingMatch, actorUserID, opensAt, closesAt, offerID); err != nil {
			return err
		}
	} else {
		// UFA / Free Agent: finalize directly
		if err := finalizeInTx(ctx, tx, offerID, actorUserID); err != nil {
			return err
		}
	}

	if err := logEvent(ctx, tx, offerID, actorUserID, "accepted", notes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AgentReject closes an offer with no signing.
func AgentReject(nbaDB *pgxpool.Pool, offerID, actorUserID, notes string) error {
	ctx := context.Background()
	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE fa_offers SET status = $1, decided_by = $2, decided_at = NOW()
		 WHERE id = $3 AND status IN ('pending_agent','pending_team')`,
		OfferAgentRejected, actorUserID, offerID); err != nil {
		return err
	}
	if err := logEvent(ctx, tx, offerID, actorUserID, "rejected", notes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AgentCounter creates a child offer with new terms; original is marked agent_countered.
// The child's status is pending_team (waiting for the offering team to accept/reject).
func AgentCounter(nbaDB *pgxpool.Pool, parentOfferID, actorUserID string, years int, startingSalary, raisePct float64, exceptionUsed, notes string) (*Offer, error) {
	ctx := context.Background()
	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Mark parent agent_countered
	tag, err := tx.Exec(ctx,
		`UPDATE fa_offers SET status = $1, decided_by = $2, decided_at = NOW()
		 WHERE id = $3 AND status = 'pending_agent'`,
		OfferAgentCountered, actorUserID, parentOfferID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, errors.New("parent offer not in pending_agent state")
	}

	// Create child copy carrying forward player/agency/teams + new terms; status pending_team
	var childID string
	err = tx.QueryRow(ctx, `
		INSERT INTO fa_offers (player_id, agency_id, offering_team_id, bird_rights_team_id,
		    years, starting_salary, raise_pct, exception_used, notes, parent_offer_id,
		    status, submitted_by)
		SELECT player_id, agency_id, offering_team_id, bird_rights_team_id,
		       $1, $2, $3, $4, $5, id, 'pending_team', $6
		FROM fa_offers WHERE id = $7
		RETURNING id
	`, years, startingSalary, raisePct, exceptionUsed, notes, actorUserID, parentOfferID).Scan(&childID)
	if err != nil {
		return nil, err
	}

	if err := logEvent(ctx, tx, parentOfferID, actorUserID, "countered", notes); err != nil {
		return nil, err
	}
	if err := logEvent(ctx, tx, childID, actorUserID, "submitted", "(counter offer from agent)"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return GetOffer(nbaDB, childID)
}

// TeamAcceptCounter — offering team accepts agent's counter. Treated like a fresh
// pending_agent → accepted by agent immediately (since terms came from the agent already).
// Re-uses AgentAccept's resolution logic.
func TeamAcceptCounter(nbaDB *pgxpool.Pool, offerID, actorUserID, notes string) error {
	ctx := context.Background()
	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var birdTeamID *string
	if err := tx.QueryRow(ctx,
		`SELECT status, bird_rights_team_id::TEXT FROM fa_offers WHERE id = $1 FOR UPDATE`,
		offerID).Scan(&status, &birdTeamID); err != nil {
		return err
	}
	if status != OfferPendingTeam {
		return fmt.Errorf("offer not pending_team (got %s)", status)
	}

	if birdTeamID != nil && *birdTeamID != "" {
		opensAt := time.Now().UTC()
		closesAt := opensAt.Add(MatchWindowDuration)
		if _, err := tx.Exec(ctx, `
			UPDATE fa_offers SET status = $1, decided_by = $2, decided_at = NOW(),
			    match_window_opens_at = $3, match_window_closes_at = $4
			WHERE id = $5
		`, OfferAwaitingMatch, actorUserID, opensAt, closesAt, offerID); err != nil {
			return err
		}
	} else {
		if err := finalizeInTx(ctx, tx, offerID, actorUserID); err != nil {
			return err
		}
	}

	if err := logEvent(ctx, tx, offerID, actorUserID, "team_accepted_counter", notes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// TeamRejectCounter — offering team walks away from agent's counter.
func TeamRejectCounter(nbaDB *pgxpool.Pool, offerID, actorUserID, notes string) error {
	ctx := context.Background()
	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE fa_offers SET status = $1, decided_by = $2, decided_at = NOW()
		 WHERE id = $3 AND status = 'pending_team'`,
		OfferAgentRejected, actorUserID, offerID); err != nil {
		return err
	}
	if err := logEvent(ctx, tx, offerID, actorUserID, "team_rejected_counter", notes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MatchOffer — bird-rights team matches; player STAYS, contract terms applied.
func MatchOffer(nbaDB *pgxpool.Pool, offerID, actorUserID, notes string) error {
	ctx := context.Background()
	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Read offer + bird-rights team
	var status string
	var playerID string
	var birdTeamID *string
	var years int
	var startingSalary, raisePct float64
	if err := tx.QueryRow(ctx, `
		SELECT status, player_id::TEXT, bird_rights_team_id::TEXT, years, starting_salary, raise_pct
		FROM fa_offers WHERE id = $1 FOR UPDATE
	`, offerID).Scan(&status, &playerID, &birdTeamID, &years, &startingSalary, &raisePct); err != nil {
		return err
	}
	if status != OfferAwaitingMatch {
		return fmt.Errorf("offer not awaiting_match (got %s)", status)
	}
	if birdTeamID == nil {
		return errors.New("offer has no bird-rights team to match")
	}

	if err := writeContractToPlayer(ctx, tx, playerID, *birdTeamID, years, startingSalary, raisePct); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE fa_offers SET status = $1, decided_by = $2, decided_at = NOW(),
		    finalized_at = NOW()
		WHERE id = $3
	`, OfferMatched, actorUserID, offerID); err != nil {
		return err
	}
	if err := logEvent(ctx, tx, offerID, actorUserID, "matched", notes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WalkOffer — bird-rights team declines to match; offer is finalized to offering team.
func WalkOffer(nbaDB *pgxpool.Pool, offerID, actorUserID, notes string) error {
	ctx := context.Background()
	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM fa_offers WHERE id = $1 FOR UPDATE`,
		offerID).Scan(&status); err != nil {
		return err
	}
	if status != OfferAwaitingMatch {
		return fmt.Errorf("offer not awaiting_match (got %s)", status)
	}

	if err := finalizeInTx(ctx, tx, offerID, actorUserID); err != nil {
		return err
	}
	if err := logEvent(ctx, tx, offerID, actorUserID, "walked", notes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WithdrawOffer — offering team pulls the offer before any decision.
func WithdrawOffer(nbaDB *pgxpool.Pool, offerID, actorUserID, notes string) error {
	ctx := context.Background()
	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE fa_offers SET status = $1, decided_by = $2, decided_at = NOW()
		WHERE id = $3 AND status IN ('pending_agent','pending_team','awaiting_match','agent_countered')
	`, OfferWithdrawn, actorUserID, offerID); err != nil {
		return err
	}
	if err := logEvent(ctx, tx, offerID, actorUserID, "withdrawn", notes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ExpireOpenMatchWindows scans for offers whose match window has closed without action,
// and finalizes them (the offering team gets the player). Returns count finalized.
func ExpireOpenMatchWindows(nbaDB *pgxpool.Pool) (int, error) {
	ctx := context.Background()
	rows, err := nbaDB.Query(ctx, `
		SELECT id::TEXT FROM fa_offers
		WHERE status = 'awaiting_match' AND match_window_closes_at < NOW()
	`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()

	count := 0
	for _, id := range ids {
		tx, err := nbaDB.Begin(ctx)
		if err != nil {
			return count, err
		}
		if err := finalizeInTx(ctx, tx, id, "00000000-0000-0000-0000-000000000000"); err != nil {
			tx.Rollback(ctx)
			continue
		}
		if err := logEvent(ctx, tx, id, "00000000-0000-0000-0000-000000000000", "match_window_expired", "auto-finalized after 48hr"); err != nil {
			tx.Rollback(ctx)
			continue
		}
		if err := tx.Commit(ctx); err == nil {
			count++
		}
	}
	return count, nil
}

// ----- Internal helpers ---------------------------------------------------

func logEvent(ctx context.Context, tx pgx.Tx, offerID, actorUserID, action, notes string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO fa_offer_events (offer_id, actor_user_id, action, notes)
		 VALUES ($1, $2, $3, NULLIF($4, ''))`,
		offerID, actorUserID, action, notes)
	return err
}

func finalizeInTx(ctx context.Context, tx pgx.Tx, offerID, actorUserID string) error {
	var playerID, offeringTeamID string
	var years int
	var startingSalary, raisePct float64
	if err := tx.QueryRow(ctx, `
		SELECT player_id::TEXT, offering_team_id::TEXT, years, starting_salary, raise_pct
		FROM fa_offers WHERE id = $1 FOR UPDATE
	`, offerID).Scan(&playerID, &offeringTeamID, &years, &startingSalary, &raisePct); err != nil {
		return err
	}
	if err := writeContractToPlayer(ctx, tx, playerID, offeringTeamID, years, startingSalary, raisePct); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE fa_offers SET status = $1, decided_by = $2, decided_at = NOW(),
		    finalized_at = NOW()
		WHERE id = $3
	`, OfferFinalized, actorUserID, offerID)
	return err
}

// writeContractToPlayer writes the offer's year-by-year salaries into the player's
// contract_YYYY columns starting at the active league year, and assigns team_id.
// Best-effort: if columns beyond contract_2040 would be needed, those overflow years
// are silently truncated.
func writeContractToPlayer(ctx context.Context, tx pgx.Tx, playerID, teamID string, years int, startingSalary, raisePct float64) error {
	salaries := make([]float64, years)
	salary := startingSalary
	for y := 0; y < years; y++ {
		salaries[y] = salary
		salary *= (1.0 + raisePct/100.0)
	}

	startYear := activeContractYear // from fa_class.go
	endYear := startYear + years - 1
	if endYear > 2040 {
		endYear = 2040
	}

	// Build SET clause dynamically; clear annotations for the affected years too.
	set := []string{"team_id = $1::uuid", "fa_status = NULL"}
	args := []interface{}{teamID}
	argIdx := 2
	for y := startYear; y <= endYear; y++ {
		offset := y - startYear
		if offset >= len(salaries) {
			break
		}
		amt := int64(salaries[offset] + 0.5)
		set = append(set, fmt.Sprintf("contract_%d = $%d", y, argIdx))
		args = append(args, fmt.Sprintf("$%d", amt))
		argIdx++
	}
	args = append(args, playerID)
	stmt := fmt.Sprintf("UPDATE players SET %s WHERE id = $%d",
		joinComma(set), argIdx)
	_, err := tx.Exec(ctx, stmt, args...)
	return err
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// ----- Read helpers --------------------------------------------------------

const offerSelect = `
	SELECT o.id::TEXT, o.player_id::TEXT, o.agency_id::TEXT, o.offering_team_id::TEXT,
	       o.bird_rights_team_id::TEXT, o.years, o.starting_salary, o.raise_pct,
	       o.exception_used, COALESCE(o.notes, ''), o.parent_offer_id::TEXT, o.status,
	       o.submitted_by::TEXT, o.decided_by::TEXT,
	       o.submitted_at, o.decided_at,
	       o.match_window_opens_at, o.match_window_closes_at, o.finalized_at,
	       p.first_name || ' ' || p.last_name AS player_name,
	       COALESCE(a.name, '') AS agency_name,
	       COALESCE(ot.name, '') AS offering_team_name,
	       COALESCE(brt.name, '') AS bird_rights_team_name
	FROM fa_offers o
	JOIN players p ON p.id = o.player_id
	LEFT JOIN agencies a ON a.id = o.agency_id
	LEFT JOIN teams ot  ON ot.id = o.offering_team_id
	LEFT JOIN teams brt ON brt.id = o.bird_rights_team_id
`

func scanOffer(row pgx.Row) (*Offer, error) {
	var o Offer
	if err := row.Scan(
		&o.ID, &o.PlayerID, &o.AgencyID, &o.OfferingTeamID,
		&o.BirdRightsTeamID, &o.Years, &o.StartingSalary, &o.RaisePct,
		&o.ExceptionUsed, &o.Notes, &o.ParentOfferID, &o.Status,
		&o.SubmittedBy, &o.DecidedBy,
		&o.SubmittedAt, &o.DecidedAt,
		&o.MatchWindowOpensAt, &o.MatchWindowClosesAt, &o.FinalizedAt,
		&o.PlayerName, &o.AgencyName, &o.OfferingTeamName, &o.BirdRightsTeamName,
	); err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOffer fetches a single offer with its hydrated joins.
func GetOffer(nbaDB *pgxpool.Pool, id string) (*Offer, error) {
	row := nbaDB.QueryRow(context.Background(), offerSelect+" WHERE o.id = $1", id)
	return scanOffer(row)
}

// ListPendingForAgency returns active offers awaiting the agent's decision.
func ListPendingForAgency(nbaDB *pgxpool.Pool, agencyID string) ([]*Offer, error) {
	rows, err := nbaDB.Query(context.Background(),
		offerSelect+` WHERE o.agency_id = $1 AND o.status IN ('pending_agent','pending_team','agent_countered','awaiting_match') ORDER BY o.submitted_at DESC`,
		agencyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Offer
	for rows.Next() {
		o, err := scanOffer(rows)
		if err == nil {
			out = append(out, o)
		}
	}
	return out, nil
}

// ListAwaitingMatchForTeam returns offers whose 48hr match clock is running for the
// given bird-rights team — these are the only actionable matches the team can decide on.
func ListAwaitingMatchForTeam(nbaDB *pgxpool.Pool, teamID string) ([]*Offer, error) {
	rows, err := nbaDB.Query(context.Background(),
		offerSelect+` WHERE o.bird_rights_team_id = $1 AND o.status = 'awaiting_match' ORDER BY o.match_window_closes_at ASC`,
		teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Offer
	for rows.Next() {
		o, err := scanOffer(rows)
		if err == nil {
			out = append(out, o)
		}
	}
	return out, nil
}

// ListOutgoingForTeam returns the team's own outgoing offers (any status).
func ListOutgoingForTeam(nbaDB *pgxpool.Pool, teamID string, limit int) ([]*Offer, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := nbaDB.Query(context.Background(),
		offerSelect+` WHERE o.offering_team_id = $1 ORDER BY o.submitted_at DESC LIMIT $2`,
		teamID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Offer
	for rows.Next() {
		o, err := scanOffer(rows)
		if err == nil {
			out = append(out, o)
		}
	}
	return out, nil
}

// OfferEvent is one row from fa_offer_events.
type OfferEvent struct {
	ID          string
	OfferID     string
	ActorUserID string
	Action      string
	Notes       string
	CreatedAt   time.Time
}

// ListEventsForOffer returns the audit log of a single offer in chronological order.
func ListEventsForOffer(nbaDB *pgxpool.Pool, offerID string) ([]OfferEvent, error) {
	rows, err := nbaDB.Query(context.Background(),
		`SELECT id::TEXT, offer_id::TEXT, actor_user_id::TEXT, action, COALESCE(notes,''), created_at
		 FROM fa_offer_events WHERE offer_id = $1 ORDER BY created_at ASC`,
		offerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OfferEvent
	for rows.Next() {
		var e OfferEvent
		if err := rows.Scan(&e.ID, &e.OfferID, &e.ActorUserID, &e.Action, &e.Notes, &e.CreatedAt); err == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

// AgenciesForUser returns the agencies a given user is a member of (used to gate the
// /nba/agent dashboard).
func AgenciesForUser(nbaDB *pgxpool.Pool, userID string) ([]string, error) {
	rows, err := nbaDB.Query(context.Background(),
		`SELECT agency_id::TEXT FROM agency_members WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			out = append(out, id)
		}
	}
	return out, nil
}
