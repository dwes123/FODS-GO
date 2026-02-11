package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RotationEntry struct {
	ID             string    `json:"id"`
	TeamID         string    `json:"team_id"`
	TeamName       string    `json:"team_name"`
	Day            string    `json:"day_of_week"`
	Pitcher1Name   string    `json:"p1_name"`
	Pitcher1Date   string    `json:"p1_date"`
	Pitcher2Name   string    `json:"p2_name"`
	Pitcher2Date   string    `json:"p2_date"`
	BankedStarters []byte    `json:"banked_starters"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func GetPitchersForTeam(db *pgxpool.Pool, teamID string) ([]RosterPlayer, error) {
	rows, err := db.Query(context.Background(), `
		SELECT id, first_name, last_name, position 
		FROM players 
		WHERE team_id = $1 AND position ILIKE '%P%' AND status_26_man = TRUE
		ORDER BY last_name
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pitchers []RosterPlayer
	for rows.Next() {
		var p RosterPlayer
		if err := rows.Scan(&p.ID, &p.FirstName, &p.LastName, &p.Position); err != nil {
			continue
		}
		pitchers = append(pitchers, p)
	}
	return pitchers, nil
}

func SubmitRotation(db *pgxpool.Pool, teamID, leagueID, week, day string, p1ID, p1Date, p2ID, p2Date string) error {
	ctx := context.Background()
	
	// Handle empty strings for IDs (convert to nil for DB)
	var p1, p2 *string
	if p1ID != "" { p1 = &p1ID }
	if p2ID != "" { p2 = &p2ID }

	var d1, d2 *string
	if p1Date != "" { d1 = &p1Date }
	if p2Date != "" { d2 = &p2Date }

	_, err := db.Exec(ctx, `
		INSERT INTO rotations (team_id, league_id, week_identifier, day_of_week, pitcher_1_id, pitcher_1_date, pitcher_2_id, pitcher_2_date, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (team_id, week_identifier, day_of_week) DO UPDATE SET
			pitcher_1_id = EXCLUDED.pitcher_1_id,
			pitcher_1_date = EXCLUDED.pitcher_1_date,
			pitcher_2_id = EXCLUDED.pitcher_2_id,
			pitcher_2_date = EXCLUDED.pitcher_2_date,
			updated_at = NOW()
	`, teamID, leagueID, week, day, p1, d1, p2, d2)

	return err
}

func GetWeeklyRotations(db *pgxpool.Pool, leagueID, week string) (map[string]map[string]RotationEntry, error) {
	rows, err := db.Query(context.Background(), `
		SELECT r.team_id, t.name, r.day_of_week, 
		       p1.first_name || ' ' || p1.last_name as p1_name, r.pitcher_1_date,
		       p2.first_name || ' ' || p2.last_name as p2_name, r.pitcher_2_date,
		       r.updated_at
		FROM rotations r
		JOIN teams t ON r.team_id = t.id
		LEFT JOIN players p1 ON r.pitcher_1_id = p1.id
		LEFT JOIN players p2 ON r.pitcher_2_id = p2.id
		WHERE r.league_id = $1 AND r.week_identifier = $2
	`, leagueID, week)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Map: TeamID -> Day -> Entry
	data := make(map[string]map[string]RotationEntry)

	for rows.Next() {
		var teamID, teamName, day, p1n, p2n string
		var p1d, p2d *time.Time
		var updated time.Time
		
		if err := rows.Scan(&teamID, &teamName, &day, &p1n, &p1d, &p2n, &p2d, &updated); err != nil {
			continue
		}

		if data[teamName] == nil {
			data[teamName] = make(map[string]RotationEntry)
		}

		entry := RotationEntry{
			TeamName:     teamName,
			Day:          day,
			Pitcher1Name: p1n,
			Pitcher2Name: p2n,
			UpdatedAt:    updated,
		}
		if p1d != nil { entry.Pitcher1Date = p1d.Format("01/02") }
		if p2d != nil { entry.Pitcher2Date = p2d.Format("01/02") }

		data[teamName][day] = entry
	}
	return data, nil
}
