package store

import (
	"context"
	"encoding/json"
	"fmt"
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
		       COALESCE(r.pitcher_1_id::TEXT, '') as p1_id,
		       COALESCE(p1.first_name || ' ' || p1.last_name, '') as p1_name, r.pitcher_1_date,
		       COALESCE(r.pitcher_2_id::TEXT, '') as p2_id,
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
		var p1id, p1n, p2id, p2n string
		var p1d, p2d *string
		var bankedRaw []byte
		var updated time.Time

		if err := rows.Scan(&teamID, &teamName, &dayNum, &p1id, &p1n, &p1d, &p2id, &p2n, &p2d, &bankedRaw, &updated); err != nil {
			continue
		}

		day := dayNumToName(dayNum)

		if data[teamName] == nil {
			data[teamName] = make(map[string]RotationEntry)
		}

		entry := RotationEntry{
			TeamID:       teamID,
			TeamName:     teamName,
			Day:          day,
			Pitcher1ID:   p1id,
			Pitcher1Name: p1n,
			Pitcher2ID:   p2id,
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

// --- Banked Starts ---

type BankedStartInput struct {
	PitcherID string
	Day       int
	Date      string
}

type BankedStartRecord struct {
	ID            string   `json:"id"`
	TeamID        string   `json:"team_id"`
	PitcherID     string   `json:"pitcher_id"`
	PitcherName   string   `json:"pitcher_name"`
	BankedWeek    string   `json:"banked_week"`
	BankedDay     int      `json:"banked_day"`
	BankedDate    string   `json:"banked_date"`
	UsedWeek      *string  `json:"used_week"`
	UsedDay       *int     `json:"used_day"`
	UsedDate      *string  `json:"used_date"`
	FantasyPoints *float64 `json:"fantasy_points"`
}

// SyncBankedStarts creates/removes banked_starts rows to match pitcher_2 entries in the submitted rotation.
// Does not touch rows that have already been used.
func SyncBankedStarts(db *pgxpool.Pool, teamID, leagueID, week string, entries []BankedStartInput) error {
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Delete unused banked starts for this team+week that are no longer in the submission
	// Build set of current pitcher+day combos
	keepSet := make(map[string]bool)
	for _, e := range entries {
		keepSet[fmt.Sprintf("%s-%d", e.PitcherID, e.Day)] = true
	}

	// Get existing unused rows for this team+week
	rows, err := tx.Query(ctx,
		`SELECT id, pitcher_id::TEXT, banked_day FROM banked_starts
		 WHERE team_id = $1 AND banked_week = $2 AND used_week IS NULL`,
		teamID, week)
	if err != nil {
		return err
	}
	var toDelete []string
	for rows.Next() {
		var id, pid string
		var day int
		rows.Scan(&id, &pid, &day)
		if !keepSet[fmt.Sprintf("%s-%d", pid, day)] {
			toDelete = append(toDelete, id)
		}
	}
	rows.Close()

	for _, id := range toDelete {
		tx.Exec(ctx, `DELETE FROM banked_starts WHERE id = $1`, id)
	}

	// Upsert current entries
	for _, e := range entries {
		_, err = tx.Exec(ctx, `
			INSERT INTO banked_starts (team_id, league_id, pitcher_id, banked_week, banked_day, banked_date)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (team_id, pitcher_id, banked_week, banked_day) DO NOTHING
		`, teamID, leagueID, e.PitcherID, week, e.Day, e.Date)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// GetAvailableBankedStarts returns unused banked starts for a team from the current or previous week.
// Lazily populates fantasy_points from daily_player_stats.
func GetAvailableBankedStarts(db *pgxpool.Pool, teamID, currentWeek, prevWeek string) ([]BankedStartRecord, error) {
	ctx := context.Background()

	rows, err := db.Query(ctx, `
		SELECT bs.id, bs.team_id::TEXT, bs.pitcher_id::TEXT,
		       p.first_name || ' ' || p.last_name AS pitcher_name,
		       bs.banked_week, bs.banked_day, bs.banked_date, bs.fantasy_points
		FROM banked_starts bs
		JOIN players p ON bs.pitcher_id = p.id
		WHERE bs.team_id = $1
		  AND bs.used_week IS NULL
		  AND bs.banked_week IN ($2, $3)
		ORDER BY bs.banked_week, bs.banked_day
	`, teamID, currentWeek, prevWeek)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BankedStartRecord
	var needPoints []string // IDs with NULL fantasy_points
	for rows.Next() {
		var r BankedStartRecord
		var bankedDate time.Time
		if err := rows.Scan(&r.ID, &r.TeamID, &r.PitcherID, &r.PitcherName,
			&r.BankedWeek, &r.BankedDay, &bankedDate, &r.FantasyPoints); err != nil {
			continue
		}
		r.BankedDate = bankedDate.Format("2006-01-02")
		if r.FantasyPoints == nil {
			needPoints = append(needPoints, r.ID)
		}
		results = append(results, r)
	}

	// Lazily populate points for any that are missing, then update our results in-place
	if len(needPoints) > 0 {
		populateBankedStartPoints(db, needPoints)
		// Re-read the points we just populated
		for i := range results {
			if results[i].FantasyPoints == nil {
				var pts *float64
				db.QueryRow(ctx, `SELECT fantasy_points FROM banked_starts WHERE id = $1`, results[i].ID).Scan(&pts)
				results[i].FantasyPoints = pts
			}
		}
	}

	return results, nil
}

// populateBankedStartPoints fills in fantasy_points from daily_player_stats for banked starts with NULL points.
func populateBankedStartPoints(db *pgxpool.Pool, bankedStartIDs []string) {
	ctx := context.Background()
	for _, id := range bankedStartIDs {
		var pitcherID string
		var bankedDate time.Time
		err := db.QueryRow(ctx,
			`SELECT pitcher_id::TEXT, banked_date FROM banked_starts WHERE id = $1`, id).Scan(&pitcherID, &bankedDate)
		if err != nil {
			continue
		}

		var pts float64
		err = db.QueryRow(ctx,
			`SELECT COALESCE(SUM(fantasy_points), 0) FROM daily_player_stats
			 WHERE player_id = $1 AND game_date = $2`,
			pitcherID, bankedDate.Format("2006-01-02")).Scan(&pts)
		if err != nil {
			continue
		}

		// Only set points if the date has been processed (stats exist)
		var count int
		db.QueryRow(ctx,
			`SELECT COUNT(*) FROM daily_player_stats WHERE player_id = $1 AND game_date = $2`,
			pitcherID, bankedDate.Format("2006-01-02")).Scan(&count)
		if count > 0 {
			db.Exec(ctx, `UPDATE banked_starts SET fantasy_points = $1 WHERE id = $2`, pts, id)
		}
	}
}

// UseBankedStart atomically claims a banked start. Returns error if already used.
func UseBankedStart(db *pgxpool.Pool, bankedStartID, usedWeek string, usedDay int, usedDate string) error {
	ctx := context.Background()
	tag, err := db.Exec(ctx, `
		UPDATE banked_starts SET used_week = $1, used_day = $2, used_date = $3
		WHERE id = $4 AND used_week IS NULL
	`, usedWeek, usedDay, usedDate, bankedStartID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("banked start already used or not found")
	}
	return nil
}

// GetBankedStartPitcherID returns the pitcher_id for a banked start.
func GetBankedStartPitcherID(db *pgxpool.Pool, bankedStartID string) (string, error) {
	ctx := context.Background()
	var pitcherID string
	err := db.QueryRow(ctx, `SELECT pitcher_id::TEXT FROM banked_starts WHERE id = $1`, bankedStartID).Scan(&pitcherID)
	return pitcherID, err
}

// UpsertRotationStarter sets the active starter (pitcher_1) on a rotation day, creating the entry if needed.
func UpsertRotationStarter(db *pgxpool.Pool, teamID, leagueID, week string, day int, pitcherID, date string) {
	ctx := context.Background()
	db.Exec(ctx, `
		INSERT INTO rotations (team_id, league_id, week_identifier, day_of_week, pitcher_1_id, pitcher_1_date, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (team_id, week_identifier, day_of_week) DO UPDATE SET
			pitcher_1_id = $5,
			pitcher_1_date = $6,
			updated_at = NOW()
	`, teamID, leagueID, week, day, pitcherID, date)
}

// UnuseBankedStart clears usage fields on a banked start.
func UnuseBankedStart(db *pgxpool.Pool, bankedStartID string) error {
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		UPDATE banked_starts SET used_week = NULL, used_day = NULL, used_date = NULL
		WHERE id = $1
	`, bankedStartID)
	return err
}

// UsedBankedDisplay is a template-friendly struct for showing used banked starts on the dashboard.
type UsedBankedDisplay struct {
	PitcherID   string
	PitcherName string
	Points      float64
	HasPoints   bool
}

// GetUsedBankedStartsForWeek returns used banked starts for a league+week, keyed by team_id then day index.
func GetUsedBankedStartsForWeek(db *pgxpool.Pool, leagueID, week string) (map[string]map[int][]UsedBankedDisplay, error) {
	ctx := context.Background()
	rows, err := db.Query(ctx, `
		SELECT bs.team_id::TEXT,
		       bs.pitcher_id::TEXT,
		       p.first_name || ' ' || p.last_name AS pitcher_name,
		       bs.used_day,
		       COALESCE(bs.fantasy_points, 0),
		       bs.fantasy_points IS NOT NULL AS has_points
		FROM banked_starts bs
		JOIN players p ON bs.pitcher_id = p.id
		WHERE bs.league_id = $1 AND bs.used_week = $2
		ORDER BY bs.used_day
	`, leagueID, week)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[int][]UsedBankedDisplay)
	for rows.Next() {
		var teamID string
		var d UsedBankedDisplay
		var usedDay int
		if err := rows.Scan(&teamID, &d.PitcherID, &d.PitcherName, &usedDay, &d.Points, &d.HasPoints); err != nil {
			continue
		}
		if result[teamID] == nil {
			result[teamID] = make(map[int][]UsedBankedDisplay)
		}
		result[teamID][usedDay] = append(result[teamID][usedDay], d)
	}
	return result, nil
}
