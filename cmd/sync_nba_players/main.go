// Command sync_nba_players imports NBA player rosters into fantasy_basketball_db.
//
// Skeleton: data source is intentionally pluggable. The default implementation hits
// stats.nba.com unofficial endpoints (commonallplayers, commonplayerinfo, leaguedashteamroster)
// with browser-like headers. If those start being blocked, swap the impl for balldontlie
// All-Star tier ($9.99/mo) — only the fetcher needs to change, the upsert path is the same.
//
// Usage:
//
//	$env:DATABASE_URL_NBA = "postgres://admin:password123@localhost:5433/fantasy_basketball_db?sslmode=disable"
//	go run ./cmd/sync_nba_players
//
// Build for the server:
//
//	$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o sync_nba_players_linux ./cmd/sync_nba_players
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PlayerSource is the abstraction over whichever NBA data API we end up using.
// Implementing a second source (e.g., balldontlie) is just satisfying this interface.
type PlayerSource interface {
	// FetchActivePlayers returns every active NBA player with rostered/team info populated.
	FetchActivePlayers(ctx context.Context) ([]ImportPlayer, error)
}

// ImportPlayer is the source-agnostic shape we upsert into fantasy_basketball_db.players.
type ImportPlayer struct {
	NBAID         int    // stats.nba.com PERSON_ID (or balldontlie equivalent)
	FirstName     string
	LastName      string
	Position      string // "PG", "SG", "PG,SG", etc.
	JerseyNumber  string
	HeightInches  *int
	WeightLbs     *int
	Age           *int
	College       string
	DraftYear     *int
	DraftPick     *int
	RealLifeTeam  string // NBA team abbrev, e.g. "LAL"
}

// statsNBASource is the default implementation hitting stats.nba.com.
//
// IMPLEMENTATION NOTE (deferred until user provides data source):
//   - Endpoint reference: https://github.com/swar/nba_api/blob/master/docs/nba_api/stats/endpoints/
//   - Required headers (mimic browser): User-Agent, Referer=https://www.nba.com/, Origin=https://www.nba.com,
//     x-nba-stats-origin: stats, x-nba-stats-token: true
//   - Rate limit politely: ~1 req/sec, exponential backoff on 429.
//   - Endpoints to compose:
//       commonallplayers?Season=2025-26&IsOnlyCurrentSeason=1 → roster of active players
//       commonplayerinfo?PlayerID={id}                       → bio + draft info per player
//       leaguedashteamroster?Season=...&TeamID=...           → real-life team mapping
//   - Aggregate into ImportPlayer slice and return.
type statsNBASource struct{}

func (statsNBASource) FetchActivePlayers(ctx context.Context) ([]ImportPlayer, error) {
	return nil, fmt.Errorf("statsNBASource: not yet implemented — fill in once data source is ready")
}

func main() {
	if os.Getenv("DATABASE_URL_NBA") == "" {
		log.Fatal("DATABASE_URL_NBA must be set (basketball DB connection string)")
	}

	nbaDB := db.InitFromEnv("DATABASE_URL_NBA")
	defer nbaDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	src := statsNBASource{}
	players, err := src.FetchActivePlayers(ctx)
	if err != nil {
		log.Fatalf("fetch failed: %v", err)
	}

	fmt.Printf("Fetched %d players, upserting...\n", len(players))
	if err := upsertPlayers(ctx, nbaDB, players); err != nil {
		log.Fatalf("upsert failed: %v", err)
	}
	fmt.Printf("Done.\n")
}

// upsertPlayers inserts new players and updates existing ones (matched on nba_id).
// Idempotent: rerunnable safely.
func upsertPlayers(ctx context.Context, nbaDB *pgxpool.Pool, players []ImportPlayer) error {
	const leagueID = "55555555-5555-5555-5555-555555555555"

	tx, err := nbaDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	stmt := `
		INSERT INTO players (
			nba_id, league_id, first_name, last_name, position, jersey_number,
			height_inches, weight_lbs, age, college, draft_year, draft_pick, real_life_team
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (nba_id) DO UPDATE SET
			first_name    = EXCLUDED.first_name,
			last_name     = EXCLUDED.last_name,
			position      = EXCLUDED.position,
			jersey_number = EXCLUDED.jersey_number,
			height_inches = EXCLUDED.height_inches,
			weight_lbs    = EXCLUDED.weight_lbs,
			age           = EXCLUDED.age,
			college       = EXCLUDED.college,
			draft_year    = EXCLUDED.draft_year,
			draft_pick    = EXCLUDED.draft_pick,
			real_life_team = EXCLUDED.real_life_team,
			updated_at    = NOW()
	`
	for _, p := range players {
		if _, err := tx.Exec(ctx, stmt,
			p.NBAID, leagueID, p.FirstName, p.LastName, p.Position, p.JerseyNumber,
			p.HeightInches, p.WeightLbs, p.Age, p.College, p.DraftYear, p.DraftPick, p.RealLifeTeam,
		); err != nil {
			return fmt.Errorf("upsert %s %s (nba_id=%d): %w", p.FirstName, p.LastName, p.NBAID, err)
		}
	}
	return tx.Commit(ctx)
}
