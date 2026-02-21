package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PlayerAdminUpdate struct {
	ID          string `json:"id"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Position    string `json:"position"`
	MLBTeam     string `json:"mlb_team"`
	TeamID      string `json:"team_id"`
	LeagueID    string `json:"league_id"`
	Status40Man bool   `json:"status_40_man"`
	Status26Man bool   `json:"status_26_man"`
	StatusIL    string `json:"status_il"`
	OptionYears int    `json:"option_years_used"`
	IsIFA       bool   `json:"is_international_free_agent"`
	DFAOnly     bool   `json:"dfa_only"`
	Contracts   map[string]string `json:"contracts"`
	ContractOptionYears []int `json:"contract_option_years"`
	// Bid & FA fields (commissioner adjustment)
	FaStatus       string  `json:"fa_status"`
	PendingBidAmt  float64 `json:"pending_bid_amount"`
	PendingBidYrs  int     `json:"pending_bid_years"`
	PendingBidAAV  float64 `json:"pending_bid_aav"`
	PendingBidTeam string  `json:"pending_bid_team_id"`
	BidType        string  `json:"bid_type"`
}

// PlayerBidInfo holds bid/FA fields for the admin player editor.
type PlayerBidInfo struct {
	FaStatus       string  `json:"fa_status"`
	PendingBidAmt  float64 `json:"pending_bid_amount"`
	PendingBidYrs  int     `json:"pending_bid_years"`
	PendingBidAAV  float64 `json:"pending_bid_aav"`
	PendingBidTeam string  `json:"pending_bid_team_id"`
	BidType        string  `json:"bid_type"`
}

func GetPlayerBidInfo(db *pgxpool.Pool, playerID string) PlayerBidInfo {
	var info PlayerBidInfo
	var bidTeam *string
	db.QueryRow(context.Background(), `
		SELECT COALESCE(fa_status, ''), COALESCE(pending_bid_amount, 0),
		       COALESCE(pending_bid_years, 0), COALESCE(pending_bid_aav, 0),
		       pending_bid_team_id, COALESCE(bid_type, '')
		FROM players WHERE id = $1
	`, playerID).Scan(&info.FaStatus, &info.PendingBidAmt, &info.PendingBidYrs, &info.PendingBidAAV, &bidTeam, &info.BidType)
	if bidTeam != nil {
		info.PendingBidTeam = *bidTeam
	}
	return info
}

type DeadCapEntry struct {
	ID         string  `json:"id"`
	TeamName   string  `json:"team_name"`
	PlayerName string  `json:"player_name"`
	Amount     float64 `json:"amount"`
	Year       int     `json:"year"`
	Note       string  `json:"note"`
}

func AdminUpdatePlayer(db *pgxpool.Pool, u PlayerAdminUpdate) error {
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)

	var teamID *string
	if u.TeamID != "" { teamID = &u.TeamID }

	// Determine fa_status: use explicit value if provided, otherwise auto-calculate
	faStatus := u.FaStatus
	if faStatus == "" {
		if teamID == nil {
			faStatus = "available"
		} else {
			faStatus = "rostered"
		}
	}

	// Bid team ID — nullable
	var bidTeamID *string
	if u.PendingBidTeam != "" { bidTeamID = &u.PendingBidTeam }

	// Marshal contract option years to JSONB
	optYearsJSON, _ := json.Marshal(u.ContractOptionYears)
	if u.ContractOptionYears == nil {
		optYearsJSON = []byte("[]")
	}

	_, err = tx.Exec(ctx, `
		UPDATE players SET
			first_name = $1, last_name = $2, position = $3, mlb_team = $4,
			team_id = $5, league_id = $6,
			status_40_man = $7, status_26_man = $8, status_il = $9, option_years_used = $10,
			contract_2026 = $11, contract_2027 = $12, contract_2028 = $13, contract_2029 = $14, contract_2030 = $15,
			contract_2031 = $16, contract_2032 = $17, contract_2033 = $18, contract_2034 = $19, contract_2035 = $20,
			contract_2036 = $21, contract_2037 = $22, contract_2038 = $23, contract_2039 = $24, contract_2040 = $25,
			fa_status = $26,
			pending_bid_amount = $27, pending_bid_years = $28, pending_bid_aav = $29,
			pending_bid_team_id = $30, bid_type = $31,
			is_international_free_agent = $32,
			contract_option_years = $33::jsonb,
			dfa_only = $34
		WHERE id = $35
	`, u.FirstName, u.LastName, u.Position, u.MLBTeam, teamID, u.LeagueID,
		u.Status40Man, u.Status26Man, u.StatusIL, u.OptionYears,
		u.Contracts["2026"], u.Contracts["2027"], u.Contracts["2028"], u.Contracts["2029"], u.Contracts["2030"],
		u.Contracts["2031"], u.Contracts["2032"], u.Contracts["2033"], u.Contracts["2034"], u.Contracts["2035"],
		u.Contracts["2036"], u.Contracts["2037"], u.Contracts["2038"], u.Contracts["2039"], u.Contracts["2040"],
		faStatus,
		u.PendingBidAmt, u.PendingBidYrs, u.PendingBidAAV, bidTeamID, u.BidType,
		u.IsIFA,
		optYearsJSON,
		u.DFAOnly,
		u.ID)
	if err != nil { return err }
	return tx.Commit(ctx)
}

func AdminCreatePlayer(db *pgxpool.Pool, u PlayerAdminUpdate) (string, error) {
	var newID string
	err := db.QueryRow(context.Background(), `
		INSERT INTO players (first_name, last_name, position, mlb_team, league_id, fa_status)
		VALUES ($1, $2, $3, $4, $5, 'available')
		RETURNING id
	`, u.FirstName, u.LastName, u.Position, u.MLBTeam, u.LeagueID).Scan(&newID)
	if err != nil { return "", err }
	u.ID = newID
	err = AdminUpdatePlayer(db, u)
	return newID, err
}

func GetDeadCapForAdmin(db *pgxpool.Pool, teamID string) ([]DeadCapEntry, error) {
	query := `
		SELECT dc.id, t.name, COALESCE(p.first_name || ' ' || p.last_name, 'Manual Entry'), dc.amount, dc.year, dc.note
		FROM dead_cap_penalties dc
		JOIN teams t ON dc.team_id = t.id
		LEFT JOIN players p ON dc.player_id = p.id
	`
	var args []interface{}
	if teamID != "" {
		query += " WHERE dc.team_id = $1"
		args = append(args, teamID)
	}
	query += " ORDER BY dc.year DESC, t.name ASC"

	rows, err := db.Query(context.Background(), query, args...)
	if err != nil { return nil, err }
	defer rows.Close()

	var entries []DeadCapEntry
	for rows.Next() {
		var e DeadCapEntry
		if err := rows.Scan(&e.ID, &e.TeamName, &e.PlayerName, &e.Amount, &e.Year, &e.Note); err != nil { continue }
		entries = append(entries, e)
	}
	return entries, nil
}

func AddDeadCapPenalty(db *pgxpool.Pool, teamID, playerID string, amount float64, year int, note string) error {
	var pID *string
	if playerID != "" { pID = &playerID }
	
	_, err := db.Exec(context.Background(), `
		INSERT INTO dead_cap_penalties (team_id, player_id, amount, year, note)
		VALUES ($1, $2, $3, $4, $5)
	`, teamID, pID, amount, year, note)
	return err
}

func DeleteDeadCapPenalty(db *pgxpool.Pool, id string) error {
	_, err := db.Exec(context.Background(), "DELETE FROM dead_cap_penalties WHERE id = $1", id)
	return err
}

func CreatePendingAction(db *pgxpool.Pool, leagueID, teamID, actionType, summary string) error {
	_, err := db.Exec(context.Background(), `
		INSERT INTO pending_actions (league_id, team_id, action_type, summary, status)
		VALUES ($1, $2, $3, $4, 'PENDING')
	`, leagueID, teamID, actionType, summary)
	return err
}

// GenerateFODIDs batch-assigns FOD-XXXXX IDs to players that don't have one.
func GenerateFODIDs(db *pgxpool.Pool, batchSize int) (int, error) {
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Lock and get current counter
	var counter int
	err = tx.QueryRow(ctx, `SELECT value FROM system_counters WHERE key = 'fod_id_counter' FOR UPDATE`).Scan(&counter)
	if err != nil {
		return 0, fmt.Errorf("counter not found: %w", err)
	}

	// Get players without FOD IDs
	rows, err := tx.Query(ctx, `SELECT id FROM players WHERE fod_id IS NULL LIMIT $1`, batchSize)
	if err != nil {
		return 0, err
	}

	var playerIDs []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		playerIDs = append(playerIDs, id)
	}
	rows.Close()

	// Assign IDs
	for _, pid := range playerIDs {
		counter++
		fodID := fmt.Sprintf("FOD-%05d", counter)
		_, err = tx.Exec(ctx, `UPDATE players SET fod_id = $1 WHERE id = $2`, fodID, pid)
		if err != nil {
			return 0, err
		}
	}

	// Update counter
	_, err = tx.Exec(ctx, `UPDATE system_counters SET value = $1 WHERE key = 'fod_id_counter'`, counter)
	if err != nil {
		return 0, err
	}

	err = tx.Commit(ctx)
	return len(playerIDs), err
}