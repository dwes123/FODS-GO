package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BidRecord struct {
	PlayerID   string  `json:"player_id"`
	PlayerName string  `json:"player_name"`
	TeamID     string  `json:"team_id"`
	TeamName   string  `json:"team_name"`
	LeagueName string  `json:"league_name"`
	Amount     float64 `json:"amount"`
	Years      int     `json:"years"`
	AAV        float64 `json:"aav"`
	BidDate    string  `json:"bid_date"`
}

type bidHistoryEntry struct {
	TeamID    string  `json:"history_team_id"`
	Amount    float64 `json:"history_bid_amount"`
	Years     int     `json:"history_bid_years"`
	AAV       float64 `json:"history_bid_aav"`
	Timestamp string  `json:"history_timestamp"`
}

func GetBidHistory(db *pgxpool.Pool, leagueID, teamID string) ([]BidRecord, error) {
	ctx := context.Background()

	query := `
		SELECT p.id, p.first_name || ' ' || p.last_name, l.name, p.bid_history
		FROM players p
		JOIN leagues l ON p.league_id = l.id
		WHERE p.bid_history IS NOT NULL AND p.bid_history != '[]'::jsonb
	`
	args := []interface{}{}
	argCount := 1

	if leagueID != "" {
		query += fmt.Sprintf(" AND p.league_id = $%d", argCount)
		args = append(args, leagueID)
		argCount++
	}

	query += " ORDER BY p.last_name ASC"

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []BidRecord
	for rows.Next() {
		var playerID, playerName, leagueName string
		var historyRaw []byte
		if err := rows.Scan(&playerID, &playerName, &leagueName, &historyRaw); err != nil {
			continue
		}

		var entries []bidHistoryEntry
		if err := json.Unmarshal(historyRaw, &entries); err != nil {
			continue
		}

		for _, e := range entries {
			if teamID != "" && e.TeamID != teamID {
				continue
			}

			// Look up team name
			var tName string
			db.QueryRow(ctx, "SELECT name FROM teams WHERE id = $1", e.TeamID).Scan(&tName)

			records = append(records, BidRecord{
				PlayerID:   playerID,
				PlayerName: playerName,
				TeamID:     e.TeamID,
				TeamName:   tName,
				LeagueName: leagueName,
				Amount:     e.Amount,
				Years:      e.Years,
				AAV:        e.AAV,
				BidDate:    e.Timestamp,
			})
		}
	}

	return records, nil
}

// AppendBidHistory appends a bid entry to a player's bid_history JSONB column.
func AppendBidHistory(db *pgxpool.Pool, playerID, teamID string, bidPoints float64, years int, aav float64) {
	entry := bidHistoryEntry{
		TeamID:    teamID,
		Amount:    bidPoints,
		Years:     years,
		AAV:       aav,
		Timestamp: time.Now().Format("2006-01-02T15:04:05"),
	}
	entryJSON, err := json.Marshal([]bidHistoryEntry{entry})
	if err != nil {
		return
	}
	db.Exec(context.Background(),
		`UPDATE players SET bid_history = COALESCE(bid_history, '[]'::jsonb) || $1::jsonb WHERE id = $2`,
		entryJSON, playerID)
}

// GetPlayerBidHistory returns the bid history for a single player, with team names resolved.
func GetPlayerBidHistory(db *pgxpool.Pool, playerID string) []BidRecord {
	ctx := context.Background()
	var historyRaw []byte
	err := db.QueryRow(ctx, `SELECT COALESCE(bid_history, '[]'::jsonb) FROM players WHERE id = $1`, playerID).Scan(&historyRaw)
	if err != nil {
		return nil
	}

	var entries []bidHistoryEntry
	if err := json.Unmarshal(historyRaw, &entries); err != nil {
		return nil
	}

	var records []BidRecord
	for _, e := range entries {
		var tName string
		db.QueryRow(ctx, "SELECT name FROM teams WHERE id = $1", e.TeamID).Scan(&tName)
		records = append(records, BidRecord{
			PlayerID: playerID,
			TeamID:   e.TeamID,
			TeamName: tName,
			Amount:   e.Amount,
			Years:    e.Years,
			AAV:      e.AAV,
			BidDate:  e.Timestamp,
		})
	}
	return records
}
