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

type PendingBidPlayer struct {
	ID              string
	FirstName       string
	LastName        string
	Position        string
	LeagueName      string
	LeagueID        string
	BiddingTeamName string
	BidAmount       float64
	BidYears        int
	BidAAV          float64
	BidEndTime      time.Time
	BidEndTimeStr   string
	TimeRemaining   string
	IsExpired       bool
}

func GetPendingBids(db *pgxpool.Pool, leagueID string) ([]PendingBidPlayer, error) {
	ctx := context.Background()

	query := `
		SELECT p.id, p.first_name, p.last_name, COALESCE(p.position, ''),
			COALESCE(l.name, 'Unknown'), COALESCE(l.id::TEXT, ''),
			COALESCE(t.name, 'Unknown'),
			COALESCE(p.pending_bid_amount, 0), COALESCE(p.pending_bid_years, 0),
			COALESCE(p.pending_bid_aav, 0), COALESCE(p.bid_end_time, NOW())
		FROM players p
		LEFT JOIN leagues l ON p.league_id = l.id
		LEFT JOIN teams t ON p.pending_bid_team_id = t.id
		WHERE p.fa_status = 'pending_bid'
	`
	args := []interface{}{}
	if leagueID != "" {
		query += " AND p.league_id = $1"
		args = append(args, leagueID)
	}
	query += " ORDER BY p.bid_end_time ASC"

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []PendingBidPlayer
	for rows.Next() {
		var p PendingBidPlayer
		if err := rows.Scan(&p.ID, &p.FirstName, &p.LastName, &p.Position,
			&p.LeagueName, &p.LeagueID, &p.BiddingTeamName,
			&p.BidAmount, &p.BidYears, &p.BidAAV, &p.BidEndTime); err != nil {
			continue
		}
		et := p.BidEndTime.In(time.FixedZone("EST", -5*3600))
		p.BidEndTimeStr = et.Format("Jan 2 3:04 PM")
		if time.Now().After(p.BidEndTime) {
			p.IsExpired = true
			p.TimeRemaining = "Expired"
		} else {
			remaining := time.Until(p.BidEndTime)
			hours := int(remaining.Hours())
			minutes := int(remaining.Minutes()) % 60
			if hours > 0 {
				p.TimeRemaining = fmt.Sprintf("%dh %dm left", hours, minutes)
			} else {
				p.TimeRemaining = fmt.Sprintf("%dm left", minutes)
			}
		}
		players = append(players, p)
	}

	return players, nil
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
