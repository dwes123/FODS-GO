package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ScoringCategory struct {
	ID          string  `json:"id"`
	StatType    string  `json:"stat_type"`
	StatKey     string  `json:"stat_key"`
	DisplayName string  `json:"display_name"`
	Points      float64 `json:"points"`
	IsActive    bool    `json:"is_active"`
}

type DailyPlayerStats struct {
	ID            string            `json:"id"`
	PlayerID      string            `json:"player_id"`
	MlbID         string            `json:"mlb_id"`
	GamePk        int               `json:"game_pk"`
	GameDate      string            `json:"game_date"`
	StatType      string            `json:"stat_type"`
	RawStats      map[string]float64 `json:"raw_stats"`
	FantasyPoints float64           `json:"fantasy_points"`
	TeamID        string            `json:"team_id"`
	LeagueID      string            `json:"league_id"`
	Opponent      string            `json:"opponent"`
	PlayerName    string            `json:"player_name"`
	Position      string            `json:"position"`
	TeamName      string            `json:"team_name"`
	LeagueName    string            `json:"league_name"`
}

type StatsLeaderEntry struct {
	PlayerID      string  `json:"player_id"`
	PlayerName    string  `json:"player_name"`
	Position      string  `json:"position"`
	TeamName      string  `json:"team_name"`
	LeagueName    string  `json:"league_name"`
	GamesPlayed   int     `json:"games_played"`
	TotalPoints   float64 `json:"total_points"`
	AvgPoints     float64 `json:"avg_points"`
	TotalIP       float64 `json:"total_ip"`
	TotalK        int     `json:"total_k"`
	TotalER       int     `json:"total_er"`
	TotalQS       int     `json:"total_qs"`
	TotalSV       int     `json:"total_sv"`
	TotalHLD      int     `json:"total_hld"`
}

type HittingLeaderEntry struct {
	PlayerID    string  `json:"player_id"`
	PlayerName  string  `json:"player_name"`
	Position    string  `json:"position"`
	TeamName    string  `json:"team_name"`
	LeagueName  string  `json:"league_name"`
	GamesPlayed int     `json:"games_played"`
	TotalPoints float64 `json:"total_points"`
	AvgPoints   float64 `json:"avg_points"`
	TotalH      int     `json:"total_h"`
	TotalHR     int     `json:"total_hr"`
	TotalRBI    int     `json:"total_rbi"`
	TotalR      int     `json:"total_r"`
	TotalBB     int     `json:"total_bb"`
	TotalSB     int     `json:"total_sb"`
	TotalK      int     `json:"total_k"`
	TotalCS     int     `json:"total_cs"`
}

// GetScoringCategories returns all scoring categories for a stat type.
func GetScoringCategories(db *pgxpool.Pool, statType string) ([]ScoringCategory, error) {
	ctx := context.Background()
	rows, err := db.Query(ctx,
		`SELECT id, stat_type, stat_key, display_name, points, is_active
		 FROM scoring_categories WHERE stat_type = $1 ORDER BY points DESC, display_name`,
		statType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []ScoringCategory
	for rows.Next() {
		var c ScoringCategory
		if err := rows.Scan(&c.ID, &c.StatType, &c.StatKey, &c.DisplayName, &c.Points, &c.IsActive); err != nil {
			continue
		}
		cats = append(cats, c)
	}
	return cats, nil
}

// GetScoringCategoryMap returns a stat_key → points lookup for calculation.
func GetScoringCategoryMap(db *pgxpool.Pool, statType string) (map[string]float64, error) {
	ctx := context.Background()
	rows, err := db.Query(ctx,
		`SELECT stat_key, points FROM scoring_categories WHERE stat_type = $1 AND is_active = TRUE`,
		statType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]float64)
	for rows.Next() {
		var key string
		var pts float64
		if err := rows.Scan(&key, &pts); err != nil {
			continue
		}
		m[key] = pts
	}
	return m, nil
}

// UpsertDailyPlayerStats inserts or updates a daily stat line.
func UpsertDailyPlayerStats(db *pgxpool.Pool, s *DailyPlayerStats) error {
	ctx := context.Background()
	rawJSON, err := json.Marshal(s.RawStats)
	if err != nil {
		return err
	}

	// Convert empty IDs to nil for nullable UUID columns
	var teamID, leagueID interface{}
	if s.TeamID != "" && s.TeamID != "00000000-0000-0000-0000-000000000000" {
		teamID = s.TeamID
	}
	if s.LeagueID != "" {
		leagueID = s.LeagueID
	}

	_, err = db.Exec(ctx, `
		INSERT INTO daily_player_stats (player_id, mlb_id, game_pk, game_date, stat_type, raw_stats, fantasy_points, team_id, league_id, opponent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (player_id, game_pk, stat_type) DO UPDATE SET
			raw_stats = EXCLUDED.raw_stats,
			fantasy_points = EXCLUDED.fantasy_points,
			team_id = EXCLUDED.team_id,
			league_id = EXCLUDED.league_id,
			opponent = EXCLUDED.opponent
	`, s.PlayerID, s.MlbID, s.GamePk, s.GameDate, s.StatType, rawJSON, s.FantasyPoints, teamID, leagueID, s.Opponent)
	return err
}

// GetPitchingLeaderboard returns aggregated fantasy points leaders for pitchers.
func GetPitchingLeaderboard(db *pgxpool.Pool, leagueID, startDate, endDate string, limit int) ([]StatsLeaderEntry, error) {
	ctx := context.Background()

	query := `
		SELECT
			dps.player_id,
			p.first_name || ' ' || p.last_name AS player_name,
			p.position,
			COALESCE(t.name, 'Free Agent') AS team_name,
			l.name AS league_name,
			COUNT(*) AS games_played,
			SUM(dps.fantasy_points) AS total_points,
			ROUND(AVG(dps.fantasy_points), 2) AS avg_points,
			COALESCE(SUM((dps.raw_stats->>'ip')::numeric), 0) AS total_ip,
			COALESCE(SUM((dps.raw_stats->>'k')::numeric), 0)::int AS total_k,
			COALESCE(SUM((dps.raw_stats->>'er')::numeric), 0)::int AS total_er,
			COALESCE(SUM(CASE WHEN (dps.raw_stats->>'qs')::numeric > 0 THEN 1 ELSE 0 END), 0)::int AS total_qs,
			COALESCE(SUM((dps.raw_stats->>'sv')::numeric), 0)::int AS total_sv,
			COALESCE(SUM((dps.raw_stats->>'hld')::numeric), 0)::int AS total_hld
		FROM daily_player_stats dps
		JOIN players p ON dps.player_id = p.id
		LEFT JOIN teams t ON p.team_id = t.id
		JOIN leagues l ON p.league_id = l.id
		WHERE dps.stat_type = 'pitching'
		  AND dps.game_date >= $1
		  AND dps.game_date <= $2
	`
	args := []interface{}{startDate, endDate}
	argCount := 3

	if leagueID != "" {
		query += fmt.Sprintf(" AND p.league_id = $%d", argCount)
		args = append(args, leagueID)
		argCount++
	}

	_ = argCount
	query += `
		GROUP BY dps.player_id, p.first_name, p.last_name, p.position, t.name, l.name
		ORDER BY total_points DESC
		LIMIT $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leaders []StatsLeaderEntry
	for rows.Next() {
		var e StatsLeaderEntry
		if err := rows.Scan(
			&e.PlayerID, &e.PlayerName, &e.Position, &e.TeamName, &e.LeagueName,
			&e.GamesPlayed, &e.TotalPoints, &e.AvgPoints,
			&e.TotalIP, &e.TotalK, &e.TotalER, &e.TotalQS, &e.TotalSV, &e.TotalHLD,
		); err != nil {
			continue
		}
		leaders = append(leaders, e)
	}
	return leaders, nil
}

// GetHittingLeaderboard returns aggregated fantasy points leaders for hitters.
func GetHittingLeaderboard(db *pgxpool.Pool, leagueID, startDate, endDate string, limit int) ([]HittingLeaderEntry, error) {
	ctx := context.Background()

	query := `
		SELECT
			dps.player_id,
			p.first_name || ' ' || p.last_name AS player_name,
			p.position,
			COALESCE(t.name, 'Free Agent') AS team_name,
			l.name AS league_name,
			COUNT(*) AS games_played,
			SUM(dps.fantasy_points) AS total_points,
			ROUND(AVG(dps.fantasy_points), 2) AS avg_points,
			COALESCE(SUM((dps.raw_stats->>'h')::numeric), 0)::int AS total_h,
			COALESCE(SUM((dps.raw_stats->>'hr')::numeric), 0)::int AS total_hr,
			COALESCE(SUM((dps.raw_stats->>'rbi')::numeric), 0)::int AS total_rbi,
			COALESCE(SUM((dps.raw_stats->>'r')::numeric), 0)::int AS total_r,
			COALESCE(SUM((dps.raw_stats->>'bb')::numeric), 0)::int AS total_bb,
			COALESCE(SUM((dps.raw_stats->>'sb')::numeric), 0)::int AS total_sb,
			COALESCE(SUM((dps.raw_stats->>'k')::numeric), 0)::int AS total_k,
			COALESCE(SUM((dps.raw_stats->>'cs')::numeric), 0)::int AS total_cs
		FROM daily_player_stats dps
		JOIN players p ON dps.player_id = p.id
		LEFT JOIN teams t ON p.team_id = t.id
		JOIN leagues l ON p.league_id = l.id
		WHERE dps.stat_type = 'hitting'
		  AND dps.game_date >= $1
		  AND dps.game_date <= $2
	`
	args := []interface{}{startDate, endDate}
	argCount := 3

	if leagueID != "" {
		query += fmt.Sprintf(" AND p.league_id = $%d", argCount)
		args = append(args, leagueID)
		argCount++
	}

	_ = argCount
	query += `
		GROUP BY dps.player_id, p.first_name, p.last_name, p.position, t.name, l.name
		ORDER BY total_points DESC
		LIMIT $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leaders []HittingLeaderEntry
	for rows.Next() {
		var e HittingLeaderEntry
		if err := rows.Scan(
			&e.PlayerID, &e.PlayerName, &e.Position, &e.TeamName, &e.LeagueName,
			&e.GamesPlayed, &e.TotalPoints, &e.AvgPoints,
			&e.TotalH, &e.TotalHR, &e.TotalRBI, &e.TotalR, &e.TotalBB, &e.TotalSB, &e.TotalK, &e.TotalCS,
		); err != nil {
			continue
		}
		leaders = append(leaders, e)
	}
	return leaders, nil
}

// GetPlayerGameLog returns a player's recent game-by-game stats.
func GetPlayerGameLog(db *pgxpool.Pool, playerID, statType string, limit int) ([]DailyPlayerStats, error) {
	ctx := context.Background()

	rows, err := db.Query(ctx, `
		SELECT dps.id, dps.player_id, COALESCE(dps.mlb_id, ''), dps.game_pk, dps.game_date,
		       dps.stat_type, dps.raw_stats, dps.fantasy_points, COALESCE(dps.opponent, '')
		FROM daily_player_stats dps
		WHERE dps.player_id = $1 AND dps.stat_type = $2
		ORDER BY dps.game_date DESC
		LIMIT $3
	`, playerID, statType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []DailyPlayerStats
	for rows.Next() {
		var s DailyPlayerStats
		var rawJSON []byte
		var gameDate time.Time
		if err := rows.Scan(&s.ID, &s.PlayerID, &s.MlbID, &s.GamePk, &gameDate,
			&s.StatType, &rawJSON, &s.FantasyPoints, &s.Opponent); err != nil {
			continue
		}
		s.GameDate = gameDate.Format("2006-01-02")
		if len(rawJSON) > 0 {
			json.Unmarshal(rawJSON, &s.RawStats)
		}
		if s.RawStats == nil {
			s.RawStats = make(map[string]float64)
		}
		logs = append(logs, s)
	}
	return logs, nil
}

// GetPlayerPointsSummary returns total fantasy points for a batch of players in a date range.
func GetPlayerPointsSummary(db *pgxpool.Pool, playerIDs []string, startDate, endDate string) (map[string]float64, error) {
	ctx := context.Background()
	result := make(map[string]float64)
	if len(playerIDs) == 0 {
		return result, nil
	}

	// Build IN clause
	args := []interface{}{startDate, endDate}
	inClause := ""
	for i, id := range playerIDs {
		if i > 0 {
			inClause += ","
		}
		inClause += fmt.Sprintf("$%d", i+3)
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT player_id, SUM(fantasy_points) AS total_points
		FROM daily_player_stats
		WHERE game_date >= $1 AND game_date <= $2
		  AND player_id IN (%s)
		GROUP BY player_id
	`, inClause)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var pid string
		var pts float64
		if err := rows.Scan(&pid, &pts); err != nil {
			continue
		}
		result[pid] = pts
	}
	return result, nil
}

// IsDateProcessed checks if a date has already been processed for a stat type.
func IsDateProcessed(db *pgxpool.Pool, date, statType string) bool {
	ctx := context.Background()
	var count int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM stats_processing_log WHERE game_date = $1 AND stat_type = $2 AND status = 'completed'`,
		date, statType).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// LogStatsProcessing records a processing run in the audit log.
func LogStatsProcessing(db *pgxpool.Pool, date, statType string, games, players int, status, errMsg string) {
	ctx := context.Background()
	db.Exec(ctx, `
		INSERT INTO stats_processing_log (game_date, stat_type, games_processed, players_processed, status, error_message)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (game_date, stat_type) DO UPDATE SET
			games_processed = EXCLUDED.games_processed,
			players_processed = EXCLUDED.players_processed,
			status = EXCLUDED.status,
			error_message = EXCLUDED.error_message,
			processed_at = NOW()
	`, date, statType, games, players, status, errMsg)
}
