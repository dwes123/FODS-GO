// Auto-fill pitching rotation for the Colorado Rockies (MLB league) from MLB probable pitchers.
// Runs locally only — not part of the web server.
//
// Usage:
//   go run ./cmd/auto_rotation --mode=weekly    # Sunday 4 PM: fill entire week
//   go run ./cmd/auto_rotation --mode=daily     # Daily 9 AM: update today only
//
// Requires SSH tunnel: ssh -N -L 15433:localhost:5433 root@178.128.178.100
// Set DATABASE_URL=postgres://admin:password123@localhost:15433/fantasy_db

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	mlbLeagueID = "11111111-1111-1111-1111-111111111111"
	teamName    = "Colorado Rockies"
	myUserID    = "" // Filled at runtime from DB
)

type pitcher struct {
	ID       string
	Name     string
	Position string
	MlbID    int
	Rank     int // Lower = better. 0 = unranked (goes last)
}

// Starter rankings — lower number = higher priority. Best available always starts.
var spRankings = map[string]int{
	"Paul Skenes":     1,
	"Ryan Pepiot":     2,
	"Shota Imanaga":   3,
	"Nathan Eovaldi":  4,
	"Drew Rasmussen":  5,
	"Casey Mize":      6,
}

func pitcherRank(name string) int {
	if r, ok := spRankings[name]; ok {
		return r
	}
	return 99 // Unranked pitchers go last
}

func main() {
	mode := flag.String("mode", "weekly", "weekly (full week) or daily (today only)")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://admin:password123@localhost:15433/fantasy_db"
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("DB connection failed: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("DB ping failed (is SSH tunnel running?): %v", err)
	}
	fmt.Println("Connected to production DB")

	// Get team ID
	ctx := context.Background()
	var teamID string
	err = pool.QueryRow(ctx,
		`SELECT id FROM teams WHERE name = $1 AND league_id = $2`, teamName, mlbLeagueID).Scan(&teamID)
	if err != nil {
		log.Fatalf("Team not found: %v", err)
	}
	fmt.Printf("Team: %s (%s)\n", teamName, teamID)

	// Get owner user ID (for audit log)
	var userID string
	pool.QueryRow(ctx,
		`SELECT user_id FROM team_owners WHERE team_id = $1 LIMIT 1`, teamID).Scan(&userID)

	// Get team's pitchers on 26-man with mlb_id
	rows, err := pool.Query(ctx, `
		SELECT id, first_name || ' ' || last_name, position, COALESCE(mlb_id, 0)
		FROM players
		WHERE team_id = $1 AND status_26_man = TRUE
		  AND position IN ('SP', 'RP', 'P', 'SP,RP', 'RP,SP')
		  AND COALESCE(mlb_id, 0) > 0
		ORDER BY position, last_name
	`, teamID)
	if err != nil {
		log.Fatalf("Failed to load roster: %v", err)
	}
	defer rows.Close()

	var pitchers []pitcher
	mlbIDMap := make(map[int]pitcher)
	for rows.Next() {
		var p pitcher
		rows.Scan(&p.ID, &p.Name, &p.Position, &p.MlbID)
		p.Rank = pitcherRank(p.Name)
		pitchers = append(pitchers, p)
		mlbIDMap[p.MlbID] = p
	}
	fmt.Printf("Found %d pitchers with MLB IDs\n", len(pitchers))
	for _, p := range pitchers {
		rankStr := "unranked"
		if p.Rank < 99 {
			rankStr = fmt.Sprintf("#%d", p.Rank)
		}
		fmt.Printf("  %s (%s) - MLB ID: %d - Rank: %s\n", p.Name, p.Position, p.MlbID, rankStr)
	}

	loc, _ := time.LoadLocation("America/Los_Angeles")
	now := time.Now().In(loc)

	switch *mode {
	case "weekly":
		fillWeek(ctx, pool, teamID, mlbLeagueID, userID, mlbIDMap, now)
	case "daily":
		checkDay(ctx, pool, teamID, mlbLeagueID, userID, mlbIDMap, now)
	default:
		log.Fatalf("Unknown mode: %s (use weekly or daily)", *mode)
	}
}

func getWeekIdentifier(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-%02d", y, w)
}

func getWeekDates(t time.Time) (monday time.Time, dates [7]time.Time) {
	y, w := t.ISOWeek()
	jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, time.UTC)
	offset := int(time.Monday - jan4.Weekday())
	if jan4.Weekday() == time.Sunday {
		offset = -6
	}
	week1Monday := jan4.AddDate(0, 0, offset)
	monday = week1Monday.AddDate(0, 0, (w-1)*7)
	for i := 0; i < 7; i++ {
		dates[i] = monday.AddDate(0, 0, i)
	}
	return
}

func fetchProbables(startDate, endDate string) map[string][]int {
	url := fmt.Sprintf("https://statsapi.mlb.com/api/v1/schedule?sportId=1&startDate=%s&endDate=%s&hydrate=probablePitcher", startDate, endDate)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("MLB API error: %v", err)
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var schedule struct {
		Dates []struct {
			Date  string `json:"date"`
			Games []struct {
				Teams struct {
					Away struct {
						ProbablePitcher struct {
							ID int `json:"id"`
						} `json:"probablePitcher"`
					} `json:"away"`
					Home struct {
						ProbablePitcher struct {
							ID int `json:"id"`
						} `json:"probablePitcher"`
					} `json:"home"`
				} `json:"teams"`
			} `json:"games"`
		} `json:"dates"`
	}
	json.Unmarshal(body, &schedule)

	// date -> []mlbIDs pitching that day
	result := make(map[string][]int)
	for _, d := range schedule.Dates {
		for _, g := range d.Games {
			if id := g.Teams.Away.ProbablePitcher.ID; id > 0 {
				result[d.Date] = append(result[d.Date], id)
			}
			if id := g.Teams.Home.ProbablePitcher.ID; id > 0 {
				result[d.Date] = append(result[d.Date], id)
			}
		}
	}
	return result
}

func fillWeek(ctx context.Context, pool *pgxpool.Pool, teamID, leagueID, userID string, mlbIDMap map[int]pitcher, now time.Time) {
	week := getWeekIdentifier(now)
	monday, dates := getWeekDates(now)
	_ = monday
	startDate := dates[0].Format("2006-01-02")
	endDate := dates[6].Format("2006-01-02")

	fmt.Printf("\n=== WEEKLY FILL: %s (%s to %s) ===\n", week, startDate, endDate)

	probables := fetchProbables(startDate, endDate)
	if probables == nil {
		log.Fatal("Failed to fetch probables")
	}

	dayNames := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

	// Match probables to our roster
	type dayInfo struct {
		date     string
		dayIdx   int
		starters []pitcher
	}
	var days [7]dayInfo
	for i := 0; i < 7; i++ {
		ds := dates[i].Format("2006-01-02")
		days[i] = dayInfo{date: ds, dayIdx: i}
		if mlbIDs, ok := probables[ds]; ok {
			for _, mlbID := range mlbIDs {
				if p, ok := mlbIDMap[mlbID]; ok {
					days[i].starters = append(days[i].starters, p)
				}
			}
		}
		// Sort by rank (best ranked pitcher starts)
		sort.Slice(days[i].starters, func(a, b int) bool {
			return days[i].starters[a].Rank < days[i].starters[b].Rank
		})
	}

	// Clear existing rotation (with audit)
	clearRotation(ctx, pool, teamID, leagueID, userID, week)

	// Assign starters and bank extras
	type bankedEntry struct {
		pitcherID string
		name      string
		fromDay   int
		rank      int
	}
	var bankedPool []bankedEntry

	for i := 0; i < 7; i++ {
		d := days[i]
		if len(d.starters) == 0 {
			fmt.Printf("  %s (%s): -- empty --\n", dayNames[i], d.date)
			continue
		}

		// First pitcher = active starter
		starter := d.starters[0]
		upsertStarter(ctx, pool, teamID, leagueID, week, i, starter.ID, d.date)
		fmt.Printf("  %s (%s): %s (active)\n", dayNames[i], d.date, starter.Name)

		// Rest = banked
		for _, p := range d.starters[1:] {
			insertBankedStart(ctx, pool, teamID, leagueID, week, i, p.ID, d.date)
			bankedPool = append(bankedPool, bankedEntry{p.ID, p.Name, i, p.Rank})
			fmt.Printf("  %s (%s): %s (banked, rank #%d)\n", dayNames[i], d.date, p.Name, p.Rank)
		}
	}

	// Auto-use banked starts on empty days (best ranked first)
	for i := 0; i < 7; i++ {
		if days[i].starters != nil && len(days[i].starters) > 0 {
			continue
		}
		// Find best-ranked available banked start from an earlier day
		bestIdx := -1
		for j := 0; j < len(bankedPool); j++ {
			if bankedPool[j].fromDay < i {
				if bestIdx == -1 || bankedPool[j].rank < bankedPool[bestIdx].rank {
					bestIdx = j
				}
			}
		}
		if bestIdx >= 0 {
			p := bankedPool[bestIdx]
			upsertStarter(ctx, pool, teamID, leagueID, week, i, p.pitcherID, days[i].date)
			// Mark as used
			var bsID string
			pool.QueryRow(ctx,
				`SELECT id FROM banked_starts WHERE team_id = $1 AND pitcher_id = $2 AND banked_week = $3 AND banked_day = $4 AND used_week IS NULL LIMIT 1`,
				teamID, p.pitcherID, week, p.fromDay).Scan(&bsID)
			if bsID != "" {
				pool.Exec(ctx, `UPDATE banked_starts SET used_week = $1, used_day = $2, used_date = $3 WHERE id = $4`,
					week, i, days[i].date, bsID)
			}
			fmt.Printf("  %s (%s): %s (banked start used, rank #%d)\n", dayNames[i], days[i].date, p.name, p.rank)
			bankedPool = append(bankedPool[:bestIdx], bankedPool[bestIdx+1:]...)
		}
	}

	fmt.Println("\nDone!")
}

func checkDay(ctx context.Context, pool *pgxpool.Pool, teamID, leagueID, userID string, mlbIDMap map[int]pitcher, now time.Time) {
	week := getWeekIdentifier(now)
	todayDate := now.Format("2006-01-02")
	dayIdx := int(now.Weekday()+6) % 7 // Convert Sun=0 to Mon=0 format
	dayNames := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

	fmt.Printf("\n=== DAILY CHECK: %s %s (week %s, day %d) ===\n", dayNames[dayIdx], todayDate, week, dayIdx)

	// Fetch today's probables
	probables := fetchProbables(todayDate, todayDate)
	if probables == nil {
		log.Fatal("Failed to fetch probables")
	}

	// Find our pitchers pitching today
	var todayPitchers []pitcher
	if mlbIDs, ok := probables[todayDate]; ok {
		for _, mlbID := range mlbIDs {
			if p, ok := mlbIDMap[mlbID]; ok {
				todayPitchers = append(todayPitchers, p)
			}
		}
	}

	// Sort by rank (best ranked starts)
	sort.Slice(todayPitchers, func(a, b int) bool {
		return todayPitchers[a].Rank < todayPitchers[b].Rank
	})

	// Check current rotation for today
	var currentP1 *string
	pool.QueryRow(ctx,
		`SELECT pitcher_1_id::TEXT FROM rotations WHERE team_id = $1 AND week_identifier = $2 AND day_of_week = $3`,
		teamID, week, dayIdx).Scan(&currentP1)

	if len(todayPitchers) == 0 {
		fmt.Println("  No pitchers scheduled today")
		if currentP1 != nil {
			fmt.Printf("  Current starter in rotation: %s (keeping as-is)\n", *currentP1)
		}
		return
	}

	newStarter := todayPitchers[0]
	if currentP1 != nil && *currentP1 == newStarter.ID {
		fmt.Printf("  %s already set as today's starter — no changes needed\n", newStarter.Name)
	} else {
		if currentP1 != nil {
			fmt.Printf("  Updating starter: was %s, now %s\n", *currentP1, newStarter.Name)
		} else {
			fmt.Printf("  Setting starter: %s\n", newStarter.Name)
		}
		upsertStarter(ctx, pool, teamID, leagueID, week, dayIdx, newStarter.ID, todayDate)
	}

	// Bank any extras
	for _, p := range todayPitchers[1:] {
		fmt.Printf("  Banking: %s\n", p.Name)
		insertBankedStart(ctx, pool, teamID, leagueID, week, dayIdx, p.ID, todayDate)
	}

	fmt.Println("Done!")
}

func clearRotation(ctx context.Context, pool *pgxpool.Pool, teamID, leagueID, userID, week string) {
	// Snapshot and delete (simplified audit)
	var rotCount, bsCount int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM rotations WHERE team_id = $1 AND week_identifier = $2`, teamID, week).Scan(&rotCount)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM banked_starts WHERE team_id = $1 AND banked_week = $2`, teamID, week).Scan(&bsCount)

	pool.Exec(ctx, `DELETE FROM rotations WHERE team_id = $1 AND week_identifier = $2`, teamID, week)
	pool.Exec(ctx, `DELETE FROM banked_starts WHERE team_id = $1 AND banked_week = $2 AND used_week IS NULL`, teamID, week)
	pool.Exec(ctx, `UPDATE banked_starts SET used_week = NULL, used_day = NULL, used_date = NULL WHERE team_id = $1 AND used_week = $2`, teamID, week)

	// Audit log
	if userID != "" {
		pool.Exec(ctx, `
			INSERT INTO rotation_clear_log (team_id, league_id, user_id, week_identifier, cleared_rotations, cleared_banked_starts)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, teamID, leagueID, userID, week,
			fmt.Sprintf(`[{"note":"auto-rotation clear","rotations":%d}]`, rotCount),
			fmt.Sprintf(`[{"note":"auto-rotation clear","banked_starts":%d}]`, bsCount))
	}

	fmt.Printf("Cleared %d rotation entries, %d banked starts\n", rotCount, bsCount)
}

func upsertStarter(ctx context.Context, pool *pgxpool.Pool, teamID, leagueID, week string, day int, pitcherID, date string) {
	pool.Exec(ctx, `
		INSERT INTO rotations (team_id, league_id, week_identifier, day_of_week, pitcher_1_id, pitcher_1_date, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (team_id, week_identifier, day_of_week) DO UPDATE SET
			pitcher_1_id = $5, pitcher_1_date = $6, updated_at = NOW()
	`, teamID, leagueID, week, day, pitcherID, date)
}

func insertBankedStart(ctx context.Context, pool *pgxpool.Pool, teamID, leagueID, week string, day int, pitcherID, date string) {
	pool.Exec(ctx, `
		INSERT INTO banked_starts (team_id, league_id, pitcher_id, banked_week, banked_day, banked_date)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (team_id, pitcher_id, banked_week, banked_day) DO NOTHING
	`, teamID, leagueID, pitcherID, week, day, date)
}
