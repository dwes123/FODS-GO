package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TradeItem struct {
	PlayerID     string `json:"player_id"`
	PlayerName   string `json:"player_name"`
	SenderTeamID string `json:"sender_team_id"`
	RetainSalary bool   `json:"retain_salary"`
}

type TradeProposal struct {
	ID                string      `json:"id"`
	ProposingTeamID   string      `json:"proposing_team_id"`
	ProposingTeamName string      `json:"proposing_team_name"`
	ReceivingTeamID   string      `json:"receiving_team_id"`
	ReceivingTeamName string      `json:"receiving_team_name"`
	IsbpOffered       int         `json:"isbp_offered"`
	IsbpRequested     int         `json:"isbp_requested"`
	Status            string      `json:"status"`
	CreatedAt         time.Time   `json:"created_at"`
	Items             []TradeItem `json:"items"`
	ParentTradeID     string      `json:"parent_trade_id"`
}

// IsTradeWindowOpen checks if trades are allowed for the given league.
// Offseason (Oct 15 – Mar 15) always allows trades.
// During the season, checks league_dates for a trade_deadline entry.
func IsTradeWindowOpen(db *pgxpool.Pool, leagueID string) (bool, string) {
	now := time.Now()
	month := now.Month()
	day := now.Day()

	// Offseason: Oct 15 – Mar 15 → always open
	if month > 10 || month < 3 || (month == 10 && day >= 15) || (month == 3 && day <= 15) {
		return true, ""
	}

	// Check league_dates for trade_deadline this year
	var deadlineDate time.Time
	err := db.QueryRow(context.Background(), `
		SELECT event_date FROM league_dates
		WHERE league_id = $1 AND year = $2 AND date_type = 'trade_deadline'
	`, leagueID, now.Year()).Scan(&deadlineDate)

	if err != nil {
		// No deadline configured → allow trades
		return true, ""
	}

	if now.After(deadlineDate) {
		return false, fmt.Sprintf("The trade deadline for this league was %s. Trades are closed until the offseason.", deadlineDate.Format("January 2, 2006"))
	}

	return true, ""
}

func CreateTradeProposal(db *pgxpool.Pool, proposerID, receiverID string, offeredPlayers, requestedPlayers, retainedPlayers []string, isbpOffered, isbpRequested int, parentTradeID string) error {
	ctx := context.Background()

	// ISBP balance validation at proposal time
	if isbpOffered > 0 {
		var balance int
		db.QueryRow(ctx, `SELECT COALESCE(isbp_balance, 0) FROM teams WHERE id = $1`, proposerID).Scan(&balance)
		if isbpOffered > balance {
			return fmt.Errorf("insufficient ISBP balance: you have %d but are offering %d", balance, isbpOffered)
		}
	}
	if isbpRequested > 0 {
		var balance int
		db.QueryRow(ctx, `SELECT COALESCE(isbp_balance, 0) FROM teams WHERE id = $1`, receiverID).Scan(&balance)
		if isbpRequested > balance {
			return fmt.Errorf("insufficient ISBP balance: the other team has %d but you are requesting %d", balance, isbpRequested)
		}
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Create Trade record
	var tradeID string
	if parentTradeID != "" {
		err = tx.QueryRow(ctx, `
			INSERT INTO trades (proposing_team_id, receiving_team_id, status, isbp_offered, isbp_requested, parent_trade_id)
			VALUES ($1, $2, 'PROPOSED', $3, $4, $5)
			RETURNING id
		`, proposerID, receiverID, isbpOffered, isbpRequested, parentTradeID).Scan(&tradeID)
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO trades (proposing_team_id, receiving_team_id, status, isbp_offered, isbp_requested)
			VALUES ($1, $2, 'PROPOSED', $3, $4)
			RETURNING id
		`, proposerID, receiverID, isbpOffered, isbpRequested).Scan(&tradeID)
	}
	if err != nil {
		return err
	}

	// Mark parent trade as COUNTERED
	if parentTradeID != "" {
		_, err = tx.Exec(ctx, `UPDATE trades SET status = 'COUNTERED' WHERE id = $1 AND status = 'PROPOSED'`, parentTradeID)
		if err != nil {
			return err
		}
	}

	// Build retained set for quick lookup
	retainedSet := make(map[string]bool)
	for _, pID := range retainedPlayers {
		retainedSet[pID] = true
	}

	// 2. Add Offered Players
	for _, pID := range offeredPlayers {
		_, err = tx.Exec(ctx, `
			INSERT INTO trade_items (trade_id, sender_team_id, player_id, retain_salary)
			VALUES ($1, $2, $3, $4)
		`, tradeID, proposerID, pID, retainedSet[pID])
		if err != nil {
			return err
		}
	}

	// 3. Add Requested Players
	for _, pID := range requestedPlayers {
		_, err = tx.Exec(ctx, `
			INSERT INTO trade_items (trade_id, sender_team_id, player_id, retain_salary)
			VALUES ($1, $2, $3, $4)
		`, tradeID, receiverID, pID, retainedSet[pID])
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func GetPendingTrades(db *pgxpool.Pool, teamIDs []string) ([]TradeProposal, error) {
	rows, err := db.Query(context.Background(), `
		SELECT t.id, t.proposing_team_id, tp.name, t.receiving_team_id, tr.name, t.status, t.created_at,
		       t.isbp_offered, t.isbp_requested, COALESCE(t.parent_trade_id::TEXT, '')
		FROM trades t
		JOIN teams tp ON t.proposing_team_id = tp.id
		JOIN teams tr ON t.receiving_team_id = tr.id
		WHERE (t.proposing_team_id = ANY($1) OR t.receiving_team_id = ANY($1))
		AND t.status = 'PROPOSED'
		ORDER BY t.created_at DESC
	`, teamIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []TradeProposal
	for rows.Next() {
		var t TradeProposal
		if err := rows.Scan(&t.ID, &t.ProposingTeamID, &t.ProposingTeamName, &t.ReceivingTeamID, &t.ReceivingTeamName, &t.Status, &t.CreatedAt, &t.IsbpOffered, &t.IsbpRequested, &t.ParentTradeID); err != nil {
			continue
		}

		// Fetch items for this trade
		itemRows, err := db.Query(context.Background(), `
			SELECT ti.player_id, p.first_name || ' ' || p.last_name, ti.sender_team_id, COALESCE(ti.retain_salary, false)
			FROM trade_items ti
			JOIN players p ON ti.player_id = p.id
			WHERE ti.trade_id = $1
		`, t.ID)
		if err == nil {
			for itemRows.Next() {
				var item TradeItem
				if err := itemRows.Scan(&item.PlayerID, &item.PlayerName, &item.SenderTeamID, &item.RetainSalary); err == nil {
					t.Items = append(t.Items, item)
				}
			}
			itemRows.Close()
		}

		trades = append(trades, t)
	}
	return trades, nil
}

func GetTradeByID(db *pgxpool.Pool, tradeID string) (*TradeProposal, error) {
	ctx := context.Background()
	var t TradeProposal
	err := db.QueryRow(ctx, `
		SELECT t.id, t.proposing_team_id, tp.name, t.receiving_team_id, tr.name,
		       t.status, t.created_at, t.isbp_offered, t.isbp_requested,
		       COALESCE(t.parent_trade_id::TEXT, '')
		FROM trades t
		JOIN teams tp ON t.proposing_team_id = tp.id
		JOIN teams tr ON t.receiving_team_id = tr.id
		WHERE t.id = $1
	`, tradeID).Scan(&t.ID, &t.ProposingTeamID, &t.ProposingTeamName,
		&t.ReceivingTeamID, &t.ReceivingTeamName, &t.Status, &t.CreatedAt,
		&t.IsbpOffered, &t.IsbpRequested, &t.ParentTradeID)
	if err != nil {
		return nil, err
	}

	// Fetch items
	rows, err := db.Query(ctx, `
		SELECT ti.player_id, p.first_name || ' ' || p.last_name, ti.sender_team_id, COALESCE(ti.retain_salary, false)
		FROM trade_items ti
		JOIN players p ON ti.player_id = p.id
		WHERE ti.trade_id = $1
	`, tradeID)
	if err != nil {
		return &t, nil
	}
	defer rows.Close()

	for rows.Next() {
		var item TradeItem
		if err := rows.Scan(&item.PlayerID, &item.PlayerName, &item.SenderTeamID, &item.RetainSalary); err == nil {
			t.Items = append(t.Items, item)
		}
	}
	return &t, nil
}

func AcceptTrade(db *pgxpool.Pool, tradeID, acceptorUserID string) error {
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Fetch Trade Details
	var proposerID, receiverID, status string
	var isbpOffered, isbpRequested int
	err = tx.QueryRow(ctx, `
		SELECT proposing_team_id, receiving_team_id, status, isbp_offered, isbp_requested
		FROM trades WHERE id = $1
	`, tradeID).Scan(&proposerID, &receiverID, &status, &isbpOffered, &isbpRequested)

	if err != nil {
		return err
	}

	if status != "PROPOSED" {
		return fmt.Errorf("trade is not pending")
	}

	// 2. Verify Acceptor owns the Receiving Team
	var isOwner bool
	err = tx.QueryRow(ctx, `SELECT COUNT(*) > 0 FROM team_owners WHERE team_id = $1 AND user_id = $2`, receiverID, acceptorUserID).Scan(&isOwner)
	if err != nil || !isOwner {
		return fmt.Errorf("unauthorized to accept this trade")
	}

	// 2b. ISBP balance validation at acceptance time
	if isbpOffered > 0 {
		var balance int
		tx.QueryRow(ctx, `SELECT COALESCE(isbp_balance, 0) FROM teams WHERE id = $1`, proposerID).Scan(&balance)
		if isbpOffered > balance {
			return fmt.Errorf("proposing team has insufficient ISBP balance (%d available, %d required)", balance, isbpOffered)
		}
	}
	if isbpRequested > 0 {
		var balance int
		tx.QueryRow(ctx, `SELECT COALESCE(isbp_balance, 0) FROM teams WHERE id = $1`, receiverID).Scan(&balance)
		if isbpRequested > balance {
			return fmt.Errorf("receiving team has insufficient ISBP balance (%d available, %d required)", balance, isbpRequested)
		}
	}

	// 3. Process Players (Ownership Transfer & Retention)
	rows, err := tx.Query(ctx, `
		SELECT ti.player_id, ti.sender_team_id, COALESCE(ti.retain_salary, false),
		       p.first_name || ' ' || p.last_name
		FROM trade_items ti
		JOIN players p ON ti.player_id = p.id
		WHERE ti.trade_id = $1
	`, tradeID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type TradeMove struct {
		PlayerID     string
		SenderID     string
		TargetID     string
		PlayerName   string
		RetainSalary bool
	}
	var moves []TradeMove

	for rows.Next() {
		var pid, sender, name string
		var retain bool
		if err := rows.Scan(&pid, &sender, &retain, &name); err != nil {
			continue
		}
		target := receiverID
		if sender == receiverID {
			target = proposerID
		}
		moves = append(moves, TradeMove{pid, sender, target, name, retain})
	}
	rows.Close()

	// --- Retention Calculation Logic ---
	now := time.Now()
	currentYear := now.Year()
	openingDay := time.Date(currentYear, 3, 30, 0, 0, 0, 0, time.UTC) // Approx
	april30 := time.Date(currentYear, 4, 30, 23, 59, 59, 0, time.UTC)
	may31 := time.Date(currentYear, 5, 31, 23, 59, 59, 0, time.UTC)

	retentionPct := 0.0
	if now.After(openingDay) && now.Before(april30) {
		retentionPct = 0.10
	} else if now.After(april30) && now.Before(may31) {
		retentionPct = 0.25
	} else if now.After(may31) {
		retentionPct = 0.50
	}

	for _, m := range moves {
		// Update Ownership
		_, err = tx.Exec(ctx, `UPDATE players SET team_id = $1 WHERE id = $2`, m.TargetID, m.PlayerID)
		if err != nil {
			return err
		}

		var contractStr string
		err = tx.QueryRow(ctx, fmt.Sprintf("SELECT COALESCE(contract_%d, '') FROM players WHERE id = $1", currentYear), m.PlayerID).Scan(&contractStr)
		if err != nil || contractStr == "" {
			continue
		}
		// Clean string "$1,500,000" -> 1500000
		cleanStr := strings.ReplaceAll(strings.ReplaceAll(contractStr, "$", ""), ",", "")
		salary, _ := strconv.ParseFloat(cleanStr, 64)
		if salary <= 0 {
			continue
		}

		totalDeadCap := 0.0
		remaining := salary

		// 1. Mandatory date-based retention
		if retentionPct > 0 {
			dateDeadCap := salary * retentionPct
			totalDeadCap += dateDeadCap
			remaining -= dateDeadCap
		}

		// 2. Optional 50% retention (on remainder)
		if m.RetainSalary && remaining > 0 {
			extraRetention := remaining * 0.50
			totalDeadCap += extraRetention
			remaining -= extraRetention
		}

		if totalDeadCap > 0 {
			// Update Player Contract
			_, err = tx.Exec(ctx, fmt.Sprintf("UPDATE players SET contract_%d = $1 WHERE id = $2", currentYear), fmt.Sprintf("%.0f", remaining), m.PlayerID)
			if err != nil {
				return err
			}

			// Insert Dead Cap Record
			note := ""
			if retentionPct > 0 && m.RetainSalary {
				note = fmt.Sprintf("Pro-Rated (%.0f%%) + 50%% Retained", retentionPct*100)
			} else if m.RetainSalary {
				note = "50% Salary Retained"
			} else {
				note = fmt.Sprintf("Trade Retention (%.0f%%)", retentionPct*100)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO dead_cap_penalties (team_id, player_id, amount, year, note)
				VALUES ($1, $2, $3, $4, $5)
			`, m.SenderID, m.PlayerID, totalDeadCap, currentYear, note)
			if err != nil {
				return err
			}
		}
	}

	// 4. Transfer ISBP
	if isbpOffered > 0 {
		_, err = tx.Exec(ctx, `UPDATE teams SET isbp_balance = isbp_balance - $1 WHERE id = $2`, isbpOffered, proposerID)
		_, err = tx.Exec(ctx, `UPDATE teams SET isbp_balance = isbp_balance + $1 WHERE id = $2`, isbpOffered, receiverID)
	}
	if isbpRequested > 0 {
		_, err = tx.Exec(ctx, `UPDATE teams SET isbp_balance = isbp_balance - $1 WHERE id = $2`, isbpRequested, receiverID)
		_, err = tx.Exec(ctx, `UPDATE teams SET isbp_balance = isbp_balance + $1 WHERE id = $2`, isbpRequested, proposerID)
	}

	// 5. Update Status & Log
	_, err = tx.Exec(ctx, `UPDATE trades SET status = 'ACCEPTED' WHERE id = $1`, tradeID)

	// 5b. Chain cleanup: mark any other PROPOSED trades in this counter chain as COUNTERED
	// Walk up the chain to find the root trade, then mark all PROPOSED descendants
	var rootID string
	err = tx.QueryRow(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, parent_trade_id FROM trades WHERE id = $1
			UNION ALL
			SELECT t.id, t.parent_trade_id FROM trades t JOIN chain c ON t.id = c.parent_trade_id
		)
		SELECT id FROM chain WHERE parent_trade_id IS NULL
	`, tradeID).Scan(&rootID)
	if err == nil {
		// Mark all PROPOSED trades in this chain (except the one being accepted) as COUNTERED
		tx.Exec(ctx, `
			WITH RECURSIVE chain AS (
				SELECT id FROM trades WHERE id = $1
				UNION ALL
				SELECT t.id FROM trades t JOIN chain c ON t.parent_trade_id = c.id
			)
			UPDATE trades SET status = 'COUNTERED'
			WHERE id IN (SELECT id FROM chain) AND id != $2 AND status = 'PROPOSED'
		`, rootID, tradeID)
	}

	// Build trade summary for transaction log
	var proposerName, receiverName string
	tx.QueryRow(ctx, `SELECT name FROM teams WHERE id = $1`, proposerID).Scan(&proposerName)
	tx.QueryRow(ctx, `SELECT name FROM teams WHERE id = $1`, receiverID).Scan(&receiverName)

	var proposerSends, receiverSends []string
	for _, m := range moves {
		if m.SenderID == proposerID {
			proposerSends = append(proposerSends, m.PlayerName)
		} else {
			receiverSends = append(receiverSends, m.PlayerName)
		}
	}
	summary := proposerName + " sends " + strings.Join(proposerSends, ", ")
	if len(receiverSends) > 0 {
		summary += " | " + receiverName + " sends " + strings.Join(receiverSends, ", ")
	}
	if isbpOffered > 0 {
		summary += fmt.Sprintf(" | %s sends $%d ISBP", proposerName, isbpOffered)
	}
	if isbpRequested > 0 {
		summary += fmt.Sprintf(" | %s sends $%d ISBP", receiverName, isbpRequested)
	}

	// Create transaction log
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (team_id, transaction_type, status, related_transaction_id, summary)
		VALUES ($1, 'TRADE', 'COMPLETED', $2, $3)
	`, proposerID, tradeID, summary)

	return tx.Commit(ctx)
}

func ReverseTrade(db *pgxpool.Pool, tradeID string) error {
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Verify trade is in ACCEPTED status
	var proposerID, receiverID, status string
	var isbpOffered, isbpRequested int
	err = tx.QueryRow(ctx, `
		SELECT proposing_team_id, receiving_team_id, status, isbp_offered, isbp_requested
		FROM trades WHERE id = $1
	`, tradeID).Scan(&proposerID, &receiverID, &status, &isbpOffered, &isbpRequested)
	if err != nil {
		return fmt.Errorf("trade not found: %w", err)
	}
	if status != "ACCEPTED" {
		return fmt.Errorf("trade is not in ACCEPTED status (current: %s)", status)
	}

	// 2. Swap all traded players back to their original teams
	rows, err := tx.Query(ctx, `SELECT player_id, sender_team_id FROM trade_items WHERE trade_id = $1`, tradeID)
	if err != nil {
		return err
	}

	type tradeMove struct {
		PlayerID     string
		SenderTeamID string
	}
	var moves []tradeMove
	for rows.Next() {
		var m tradeMove
		if err := rows.Scan(&m.PlayerID, &m.SenderTeamID); err != nil {
			continue
		}
		moves = append(moves, m)
	}
	rows.Close()

	for _, m := range moves {
		_, err = tx.Exec(ctx, `UPDATE players SET team_id = $1 WHERE id = $2`, m.SenderTeamID, m.PlayerID)
		if err != nil {
			return err
		}
	}

	// 3. Reverse ISBP transfers
	if isbpOffered > 0 {
		tx.Exec(ctx, `UPDATE teams SET isbp_balance = isbp_balance + $1 WHERE id = $2`, isbpOffered, proposerID)
		tx.Exec(ctx, `UPDATE teams SET isbp_balance = isbp_balance - $1 WHERE id = $2`, isbpOffered, receiverID)
	}
	if isbpRequested > 0 {
		tx.Exec(ctx, `UPDATE teams SET isbp_balance = isbp_balance + $1 WHERE id = $2`, isbpRequested, receiverID)
		tx.Exec(ctx, `UPDATE teams SET isbp_balance = isbp_balance - $1 WHERE id = $2`, isbpRequested, proposerID)
	}

	// 4. Remove trade-specific dead cap penalties
	var tradeCreatedAt time.Time
	tx.QueryRow(ctx, `SELECT created_at FROM trades WHERE id = $1`, tradeID).Scan(&tradeCreatedAt)
	tx.Exec(ctx, `
		DELETE FROM dead_cap_penalties
		WHERE (note ILIKE '%Trade Retention%' OR note ILIKE '%Pro-Rated%' OR note ILIKE '%Retained%')
		AND created_at >= $1
	`, tradeCreatedAt)

	// 5. Set trade status to REVERSED
	_, err = tx.Exec(ctx, `UPDATE trades SET status = 'REVERSED' WHERE id = $1`, tradeID)
	if err != nil {
		return err
	}

	// 6. Log as Admin Correction
	tx.Exec(ctx, `
		INSERT INTO transactions (team_id, transaction_type, status, summary)
		VALUES ($1, 'COMMISSIONER', 'COMPLETED', 'Trade reversed by Commissioner')
	`, proposerID)

	return tx.Commit(ctx)
}
