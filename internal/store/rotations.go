package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BankedStarter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Date string `json:"date"`
}

var dayNames = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

func dayNumToName(n int) string {
	if n >= 0 && n < len(dayNames) {
		return dayNames[n]
	}
	return "unknown"
}

type RotationEntry struct {
	ID                  string          `json:"id"`
	TeamID              string          `json:"team_id"`
	TeamName            string          `json:"team_name"`
	Day                 string          `json:"day_of_week"`
	Pitcher1ID          string          `json:"p1_id"`
	Pitcher1Name        string          `json:"p1_name"`
	Pitcher1Date        string          `json:"p1_date"`
	Pitcher2ID          string          `json:"p2_id"`
	Pitcher2Name        string          `json:"p2_name"`
	Pitcher2Date        string          `json:"p2_date"`
	BankedStarters      []byte          `json:"banked_starters"`
	BankedStartersParsed []BankedStarter `json:"banked_starters_parsed"`
	UpdatedAt           time.Time       `json:"updated_at"`
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

func SubmitRotation(db *pgxpool.Pool, teamID, leagueID, week, day string, p1ID, p1Date, p2ID, p2Date string, bankedJSON []byte) error {
	ctx := context.Background()

	// Handle empty strings for IDs (convert to nil for DB)
	var p1, p2 *string
	if p1ID != "" { p1 = &p1ID }
	if p2ID != "" { p2 = &p2ID }

	var d1, d2 *string
	if p1Date != "" { d1 = &p1Date }
	if p2Date != "" { d2 = &p2Date }

	// Handle banked starters JSONB — nil means no banked starters for this day
	var banked interface{}
	if len(bankedJSON) > 0 {
		banked = bankedJSON
	}

	_, err := db.Exec(ctx, `
		INSERT INTO rotations (team_id, league_id, week_identifier, day_of_week, pitcher_1_id, pitcher_1_date, pitcher_2_id, pitcher_2_date, banked_starters, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (team_id, week_identifier, day_of_week) DO UPDATE SET
			pitcher_1_id = EXCLUDED.pitcher_1_id,
			pitcher_1_date = EXCLUDED.pitcher_1_date,
			pitcher_2_id = EXCLUDED.pitcher_2_id,
			pitcher_2_date = EXCLUDED.pitcher_2_date,
			banked_starters = EXCLUDED.banked_starters,
			updated_at = NOW()
	`, teamID, leagueID, week, day, p1, d1, p2, d2, banked)

	return err
}

func GetWeeklyRotations(db *pgxpool.Pool, leagueID, week string) (map[string]map[string]RotationEntry, error) {
	rows, err := db.Query(context.Background(), `
		SELECT r.team_id, t.name, r.day_of_week,
		       COALESCE(p1.first_name || ' ' || p1.last_name, '') as p1_name, r.pitcher_1_date,
		       COALESCE(p2.first_name || ' ' || p2.last_name, '') as p2_name, r.pitcher_2_date,
		       COALESCE(r.banked_starters, '[]'::JSONB),
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

	// Map: TeamName -> Day -> Entry
	data := make(map[string]map[string]RotationEntry)

	for rows.Next() {
		var teamID, teamName string
		var dayNum int
		var p1n, p2n string
		var p1d, p2d *string
		var bankedRaw []byte
		var updated time.Time

		if err := rows.Scan(&teamID, &teamName, &dayNum, &p1n, &p1d, &p2n, &p2d, &bankedRaw, &updated); err != nil {
			continue
		}

		day := dayNumToName(dayNum)

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
		if p1d != nil { entry.Pitcher1Date = *p1d }
		if p2d != nil { entry.Pitcher2Date = *p2d }

		// Parse banked starters JSONB
		if len(bankedRaw) > 0 {
			var banked []BankedStarter
			if err := json.Unmarshal(bankedRaw, &banked); err == nil {
				entry.BankedStartersParsed = banked
			}
		}

		data[teamName][day] = entry
	}
	return data, nil
}

// GetTeamWeekRotation returns a team's rotation entries for a specific week, keyed by day
func GetTeamWeekRotation(db *pgxpool.Pool, teamID, week string) (map[string]RotationEntry, error) {
	rows, err := db.Query(context.Background(), `
		SELECT r.day_of_week,
		       COALESCE(r.pitcher_1_id::TEXT, '') as p1_id,
		       COALESCE(p1.first_name || ' ' || p1.last_name, '') as p1_name,
		       r.pitcher_1_date,
		       COALESCE(r.pitcher_2_id::TEXT, '') as p2_id,
		       COALESCE(p2.first_name || ' ' || p2.last_name, '') as p2_name,
		       r.pitcher_2_date,
		       COALESCE(r.banked_starters, '[]'::JSONB)
		FROM rotations r
		LEFT JOIN players p1 ON r.pitcher_1_id = p1.id
		LEFT JOIN players p2 ON r.pitcher_2_id = p2.id
		WHERE r.team_id = $1 AND r.week_identifier = $2
	`, teamID, week)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	data := make(map[string]RotationEntry)
	for rows.Next() {
		var dayNum int
		var p1id, p1name, p2id, p2name string
		var p1d, p2d *string
		var bankedRaw []byte

		if err := rows.Scan(&dayNum, &p1id, &p1name, &p1d, &p2id, &p2name, &p2d, &bankedRaw); err != nil {
			continue
		}

		day := dayNumToName(dayNum)
		entry := RotationEntry{
			Day:          day,
			Pitcher1ID:   p1id,
			Pitcher1Name: p1name,
			Pitcher2ID:   p2id,
			Pitcher2Name: p2name,
		}
		if p1d != nil { entry.Pitcher1Date = *p1d }
		if p2d != nil { entry.Pitcher2Date = *p2d }

		// Parse banked starters JSONB
		if len(bankedRaw) > 0 {
			var banked []BankedStarter
			if err := json.Unmarshal(bankedRaw, &banked); err == nil {
				entry.BankedStartersParsed = banked
			}
		}

		data[day] = entry
	}
	return data, nil
}

// DeleteRotation removes a rotation entry for a specific team/week/day
func DeleteRotation(db *pgxpool.Pool, teamID, week, day string) error {
	_, err := db.Exec(context.Background(),
		`DELETE FROM rotations WHERE team_id = $1 AND week_identifier = $2 AND day_of_week = $3`,
		teamID, week, day)
	return err
}

// GetLeagueTeamCount returns the total number of teams in a league
func GetLeagueTeamCount(db *pgxpool.Pool, leagueID string) (int, error) {
	var count int
	err := db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM teams WHERE league_id = $1`, leagueID).Scan(&count)
	return count, err
}

// GetSubmittedTeamCount returns how many distinct teams submitted rotations for a given league+week
func GetSubmittedTeamCount(db *pgxpool.Pool, leagueID, week string) (int, error) {
	var count int
	err := db.QueryRow(context.Background(),
		`SELECT COUNT(DISTINCT team_id) FROM rotations WHERE league_id = $1 AND week_identifier = $2`,
		leagueID, week).Scan(&count)
	return count, err
}

// IsPlayerOn26Man checks if a player is on a team's 26-man roster
func IsPlayerOn26Man(db *pgxpool.Pool, playerID, teamID string) (bool, error) {
	var exists bool
	err := db.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM players WHERE id = $1 AND team_id = $2 AND status_26_man = TRUE)`,
		playerID, teamID).Scan(&exists)
	return exists, err
}
