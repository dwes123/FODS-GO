package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Activity struct {
	ID                string    `json:"id"`
	TeamName          string    `json:"team_name"`
	PlayerName        string    `json:"player_name"`
	TransactionType   string    `json:"transaction_type"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	LeagueName        string    `json:"league_name"`
	Summary           string    `json:"summary"`
	FantraxProcessed  bool      `json:"fantrax_processed"`
}

func LogActivity(db *pgxpool.Pool, leagueID, teamID, transType, summary string) error {
	_, err := db.Exec(context.Background(), `
		INSERT INTO transactions (league_id, team_id, transaction_type, summary, status)
		VALUES ($1, $2, $3, $4, 'COMPLETED')
	`, leagueID, teamID, transType, summary)
	return err
}

func GetFantraxQueue(db *pgxpool.Pool, leagueID string, showCompleted bool) ([]Activity, error) {
	query := `
		SELECT t.id, COALESCE(teams.name, 'League'), COALESCE(p.first_name || ' ' || p.last_name, ''),
		       t.transaction_type, t.status, t.created_at, COALESCE(l.name, 'All'),
		       COALESCE(t.summary, ''), COALESCE(t.fantrax_processed, FALSE)
		FROM transactions t
		LEFT JOIN teams ON t.team_id = teams.id
		LEFT JOIN players p ON t.player_id = p.id
		LEFT JOIN leagues l ON teams.league_id = l.id OR t.league_id = l.id
		WHERE t.transaction_type IN ('Roster Move', 'Added Player', 'Trade')
		  AND COALESCE(t.summary, '') NOT LIKE '%promoted%to 40-Man roster%'
		  AND COALESCE(t.summary, '') NOT LIKE '%promoted%to 26-Man roster%'
		  AND t.created_at >= CURRENT_DATE
	`

	args := []interface{}{}
	argIdx := 1

	if !showCompleted {
		query += fmt.Sprintf(" AND (t.fantrax_processed IS NOT TRUE) ")
	}

	if leagueID != "" {
		query += fmt.Sprintf(" AND (teams.league_id = $%d OR t.league_id = $%d) ", argIdx, argIdx)
		args = append(args, leagueID)
		argIdx++
	}

	query += " ORDER BY t.created_at DESC"

	rows, err := db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.TeamName, &a.PlayerName, &a.TransactionType, &a.Status, &a.CreatedAt, &a.LeagueName, &a.Summary, &a.FantraxProcessed); err != nil {
			continue
		}
		a.TransactionType = strings.TrimSpace(strings.ToUpper(a.TransactionType))
		activities = append(activities, a)
	}
	return activities, nil
}

func GetRecentActivity(db *pgxpool.Pool, limit int, leagueID string) ([]Activity, error) {
	return GetRecentActivityForLeagues(db, limit, leagueID, nil)
}

func GetTransactionLog(db *pgxpool.Pool, limit int, leagueID, transactionType string) ([]Activity, error) {
	query := `
		SELECT t.id, COALESCE(teams.name, 'League'), COALESCE(p.first_name || ' ' || p.last_name, ''),
		       t.transaction_type, t.status, t.created_at, COALESCE(l.name, 'All'),
		       COALESCE(t.summary, ''), COALESCE(t.fantrax_processed, FALSE)
		FROM transactions t
		LEFT JOIN teams ON t.team_id = teams.id
		LEFT JOIN players p ON t.player_id = p.id
		LEFT JOIN leagues l ON teams.league_id = l.id OR t.league_id = l.id
		WHERE t.status = 'COMPLETED'
	`
	args := []interface{}{limit}
	argIdx := 2
	if leagueID != "" {
		query += fmt.Sprintf(" AND (teams.league_id = $%d OR t.league_id = $%d)", argIdx, argIdx)
		args = append(args, leagueID)
		argIdx++
	}
	if transactionType != "" {
		query += fmt.Sprintf(" AND UPPER(TRIM(t.transaction_type)) = UPPER($%d)", argIdx)
		args = append(args, transactionType)
		argIdx++
	}
	query += " ORDER BY t.created_at DESC LIMIT $1"

	rows, err := db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.TeamName, &a.PlayerName, &a.TransactionType, &a.Status, &a.CreatedAt, &a.LeagueName, &a.Summary, &a.FantraxProcessed); err != nil {
			continue
		}
		a.TransactionType = strings.TrimSpace(strings.ToUpper(a.TransactionType))
		activities = append(activities, a)
	}
	return activities, nil
}

func GetDistinctTransactionTypes(db *pgxpool.Pool) ([]string, error) {
	rows, err := db.Query(context.Background(), `
		SELECT DISTINCT UPPER(TRIM(transaction_type))
		FROM transactions
		WHERE status = 'COMPLETED' AND transaction_type IS NOT NULL
		ORDER BY 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			types = append(types, t)
		}
	}
	return types, nil
}

func GetRecentActivityForLeagues(db *pgxpool.Pool, limit int, leagueID string, leagueIDs []string) ([]Activity, error) {
	query := `
		SELECT t.id, COALESCE(teams.name, 'League'), COALESCE(p.first_name || ' ' || p.last_name, ''),
		       t.transaction_type, t.status, t.created_at, COALESCE(l.name, 'All'),
		       COALESCE(t.summary, ''), COALESCE(t.fantrax_processed, FALSE)
		FROM transactions t
		LEFT JOIN teams ON t.team_id = teams.id
		LEFT JOIN players p ON t.player_id = p.id
		LEFT JOIN leagues l ON teams.league_id = l.id OR t.league_id = l.id
	`

	args := []interface{}{limit}
	if leagueID != "" {
		query += " WHERE (teams.league_id = $2 OR t.league_id = $2) "
		args = append(args, leagueID)
	} else if len(leagueIDs) > 0 {
		query += " WHERE (teams.league_id = ANY($2) OR t.league_id = ANY($2)) "
		args = append(args, leagueIDs)
	}

	query += " ORDER BY t.created_at DESC LIMIT $1"

	rows, err := db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.TeamName, &a.PlayerName, &a.TransactionType, &a.Status, &a.CreatedAt, &a.LeagueName, &a.Summary, &a.FantraxProcessed); err != nil {
			continue
		}

		a.TransactionType = strings.TrimSpace(strings.ToUpper(a.TransactionType))
		activities = append(activities, a)
	}
	return activities, nil
}
