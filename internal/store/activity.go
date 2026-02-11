package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Activity struct {
	ID              string    `json:"id"`
	TeamName        string    `json:"team_name"`
	PlayerName      string    `json:"player_name"`
	TransactionType string    `json:"transaction_type"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	LeagueName      string    `json:"league_name"`
	Summary         string    `json:"summary"`
}

func LogActivity(db *pgxpool.Pool, leagueID, teamID, transType, summary string) error {
	_, err := db.Exec(context.Background(), `
		INSERT INTO transactions (league_id, team_id, transaction_type, summary, status)
		VALUES ($1, $2, $3, $4, 'COMPLETED')
	`, leagueID, teamID, transType, summary)
	return err
}

func GetRecentActivity(db *pgxpool.Pool, limit int, leagueID string) ([]Activity, error) {
	query := `
		SELECT t.id, COALESCE(teams.name, 'League'), COALESCE(p.first_name || ' ' || p.last_name, ''), 
		       t.transaction_type, t.status, t.created_at, COALESCE(l.name, 'All'),
		       COALESCE(t.summary, '')
		FROM transactions t
		LEFT JOIN teams ON t.team_id = teams.id
		LEFT JOIN players p ON t.player_id = p.id
		LEFT JOIN leagues l ON teams.league_id = l.id OR t.league_id = l.id
	`

	args := []interface{}{limit}
	if leagueID != "" {
		query += " WHERE (teams.league_id = $2 OR t.league_id = $2) "
		args = append(args, leagueID)
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
		if err := rows.Scan(&a.ID, &a.TeamName, &a.PlayerName, &a.TransactionType, &a.Status, &a.CreatedAt, &a.LeagueName, &a.Summary); err != nil {
			continue
		}

		a.TransactionType = strings.TrimSpace(strings.ToUpper(a.TransactionType))
		activities = append(activities, a)
	}
	return activities, nil
}
