package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StartStatsWorker polls for completed MLB games and processes pitching + hitting stats.
// Ticks every 30 minutes; runs 5-6 AM ET during season (Mar 25-Oct).
// Catches up any unprocessed dates in the last 7 days on each tick.
func StartStatsWorker(ctx context.Context, db *pgxpool.Pool) {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("Stats worker stopped")
				return
			case <-ticker.C:
				loc, err := time.LoadLocation("America/New_York")
				if err != nil {
					loc = time.FixedZone("EST", -5*60*60)
				}
				et := time.Now().In(loc)

				// Only run late March-October, 5-6 AM ET
				month := et.Month()
				day := et.Day()
				hour := et.Hour()
				if month < 3 || month > 10 || (month == 3 && day < 25) || hour < 5 || hour > 6 {
					continue
				}

				// Process yesterday + catch up last 7 days (pitching + hitting)
				for i := 1; i <= 7; i++ {
					date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
					pitchingDone := store.IsDateProcessed(db, date, "pitching")
					hittingDone := store.IsDateProcessed(db, date, "hitting")
					if pitchingDone && hittingDone {
						continue
					}
					fmt.Printf("Stats worker: processing stats for %s\n", date)
					ProcessDateStats(ctx, db, date)
				}
			}
		}
	}()
}

// ProcessDateStats fetches all completed MLB games for a date and processes both pitching and hitting stats.
// Exported so it can be called from the admin backfill handler.
func ProcessDateStats(ctx context.Context, db *pgxpool.Pool, date string) {
	pitchingDone := store.IsDateProcessed(db, date, "pitching")
	hittingDone := store.IsDateProcessed(db, date, "hitting")

	if pitchingDone && hittingDone {
		return
	}

	var pitchingScoringMap, hittingScoringMap map[string]float64
	var err error

	if !pitchingDone {
		pitchingScoringMap, err = store.GetScoringCategoryMap(db, "pitching")
		if err != nil {
			fmt.Printf("ERROR [StatsWorker]: failed to load pitching scoring: %v\n", err)
			store.LogStatsProcessing(db, date, "pitching", 0, 0, "error", err.Error())
			return
		}
	}

	if !hittingDone {
		hittingScoringMap, err = store.GetScoringCategoryMap(db, "hitting")
		if err != nil {
			fmt.Printf("ERROR [StatsWorker]: failed to load hitting scoring: %v\n", err)
			store.LogStatsProcessing(db, date, "hitting", 0, 0, "error", err.Error())
			return
		}
	}

	games, err := fetchSchedule(date)
	if err != nil {
		fmt.Printf("ERROR [StatsWorker]: failed to fetch schedule for %s: %v\n", date, err)
		if !pitchingDone {
			store.LogStatsProcessing(db, date, "pitching", 0, 0, "error", err.Error())
		}
		if !hittingDone {
			store.LogStatsProcessing(db, date, "hitting", 0, 0, "error", err.Error())
		}
		return
	}

	pitchingGames, pitchingPlayers := 0, 0
	hittingGames, hittingPlayers := 0, 0

	for _, game := range games {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Fetch box score once per game
		boxscore, err := fetchBoxScore(game.GamePk)
		if err != nil {
			fmt.Printf("ERROR [StatsWorker]: game %d boxscore: %v\n", game.GamePk, err)
			continue
		}

		if !pitchingDone {
			count, err := processGamePitching(db, boxscore, game.GamePk, date, pitchingScoringMap)
			if err != nil {
				fmt.Printf("ERROR [StatsWorker]: game %d pitching: %v\n", game.GamePk, err)
			} else {
				pitchingGames++
				pitchingPlayers += count
			}
		}

		if !hittingDone {
			count, err := processGameHitting(db, boxscore, game.GamePk, date, hittingScoringMap)
			if err != nil {
				fmt.Printf("ERROR [StatsWorker]: game %d hitting: %v\n", game.GamePk, err)
			} else {
				hittingGames++
				hittingPlayers += count
			}
		}

		// Rate limit: 1 second between box score fetches
		time.Sleep(1 * time.Second)
	}

	if !pitchingDone {
		store.LogStatsProcessing(db, date, "pitching", pitchingGames, pitchingPlayers, "completed", "")
		fmt.Printf("Stats worker: %s pitching — %d games, %d pitchers\n", date, pitchingGames, pitchingPlayers)
	}
	if !hittingDone {
		store.LogStatsProcessing(db, date, "hitting", hittingGames, hittingPlayers, "completed", "")
		fmt.Printf("Stats worker: %s hitting — %d games, %d hitters\n", date, hittingGames, hittingPlayers)
	}
}

type scheduleGame struct {
	GamePk int
	State  string
}

func fetchSchedule(date string) ([]scheduleGame, error) {
	url := fmt.Sprintf("https://statsapi.mlb.com/api/v1/schedule?sportId=1&date=%s", date)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var schedule struct {
		Dates []struct {
			Games []struct {
				GamePk int `json:"gamePk"`
				Status struct {
					AbstractGameState string `json:"abstractGameState"`
				} `json:"status"`
			} `json:"games"`
		} `json:"dates"`
	}
	if err := json.Unmarshal(body, &schedule); err != nil {
		return nil, err
	}

	var games []scheduleGame
	for _, d := range schedule.Dates {
		for _, g := range d.Games {
			if g.Status.AbstractGameState == "Final" {
				games = append(games, scheduleGame{GamePk: g.GamePk, State: g.Status.AbstractGameState})
			}
		}
	}
	return games, nil
}

// fetchBoxScore retrieves the box score for a single game from the MLB API.
func fetchBoxScore(gamePk int) (*boxScoreResp, error) {
	url := fmt.Sprintf("https://statsapi.mlb.com/api/v1/game/%d/boxscore", gamePk)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var boxscore boxScoreResp
	if err := json.Unmarshal(body, &boxscore); err != nil {
		return nil, err
	}
	return &boxscore, nil
}

func processGamePitching(db *pgxpool.Pool, boxscore *boxScoreResp, gamePk int, date string, scoringMap map[string]float64) (int, error) {
	count := 0

	for _, side := range []boxScoreSide{boxscore.Teams.Away, boxscore.Teams.Home} {
		var opponent string
		if side.Team.Abbreviation == boxscore.Teams.Away.Team.Abbreviation {
			opponent = boxscore.Teams.Home.Team.Abbreviation
		} else {
			opponent = boxscore.Teams.Away.Team.Abbreviation
		}

		for _, playerEntry := range side.Players {
			if playerEntry.Stats.Pitching.InningsPitched == "" {
				continue // Not a pitcher in this game
			}

			mlbID := playerEntry.Person.ID
			raw := extractPitchingRawStats(playerEntry.Stats.Pitching)
			pts := calculateFantasyPoints(raw, scoringMap)

			dbCtx := context.Background()
			var playerID, teamID, leagueID string
			err := db.QueryRow(dbCtx, `
				SELECT p.id, COALESCE(p.team_id::TEXT, ''), COALESCE(p.league_id::TEXT, '')
				FROM players p
				WHERE p.mlb_id = $1
				LIMIT 1
			`, mlbID).Scan(&playerID, &teamID, &leagueID)
			if err != nil {
				continue // Player not in our DB
			}

			stat := &store.DailyPlayerStats{
				PlayerID:      playerID,
				MlbID:         strconv.Itoa(mlbID),
				GamePk:        gamePk,
				GameDate:      date,
				StatType:      "pitching",
				RawStats:      raw,
				FantasyPoints: pts,
				TeamID:        teamID,
				LeagueID:      leagueID,
				Opponent:      opponent,
			}

			if err := store.UpsertDailyPlayerStats(db, stat); err != nil {
				fmt.Printf("ERROR [StatsWorker]: upsert pitcher %s game %d: %v\n", playerID, gamePk, err)
				continue
			}
			count++
		}
	}

	return count, nil
}

func processGameHitting(db *pgxpool.Pool, boxscore *boxScoreResp, gamePk int, date string, scoringMap map[string]float64) (int, error) {
	count := 0

	for _, side := range []boxScoreSide{boxscore.Teams.Away, boxscore.Teams.Home} {
		var opponent string
		if side.Team.Abbreviation == boxscore.Teams.Away.Team.Abbreviation {
			opponent = boxscore.Teams.Home.Team.Abbreviation
		} else {
			opponent = boxscore.Teams.Away.Team.Abbreviation
		}

		for _, playerEntry := range side.Players {
			batting := playerEntry.Stats.Batting
			ab := toFloat(batting.AtBats)
			pa := toFloat(batting.PlateAppearances)
			if ab == 0 && pa == 0 {
				continue // Did not bat in this game
			}

			mlbID := playerEntry.Person.ID
			raw := extractHittingRawStats(batting)
			pts := calculateFantasyPoints(raw, scoringMap)

			dbCtx := context.Background()
			var playerID, teamID, leagueID string
			err := db.QueryRow(dbCtx, `
				SELECT p.id, COALESCE(p.team_id::TEXT, ''), COALESCE(p.league_id::TEXT, '')
				FROM players p
				WHERE p.mlb_id = $1
				LIMIT 1
			`, mlbID).Scan(&playerID, &teamID, &leagueID)
			if err != nil {
				continue // Player not in our DB
			}

			stat := &store.DailyPlayerStats{
				PlayerID:      playerID,
				MlbID:         strconv.Itoa(mlbID),
				GamePk:        gamePk,
				GameDate:      date,
				StatType:      "hitting",
				RawStats:      raw,
				FantasyPoints: pts,
				TeamID:        teamID,
				LeagueID:      leagueID,
				Opponent:      opponent,
			}

			if err := store.UpsertDailyPlayerStats(db, stat); err != nil {
				fmt.Printf("ERROR [StatsWorker]: upsert hitter %s game %d: %v\n", playerID, gamePk, err)
				continue
			}
			count++
		}
	}

	return count, nil
}

// extractPitchingRawStats converts MLB API pitching stats to our flat map.
func extractPitchingRawStats(p pitchingStats) map[string]float64 {
	raw := make(map[string]float64)

	ip := parseInningsPitched(p.InningsPitched)
	raw["ip"] = ip
	raw["k"] = toFloat(p.StrikeOuts)
	raw["er"] = toFloat(p.EarnedRuns)
	raw["bb"] = toFloat(p.BaseOnBalls)
	raw["hra"] = toFloat(p.HomeRuns)
	raw["hb"] = toFloat(p.HitByPitch)
	raw["wp"] = toFloat(p.WildPitches)
	raw["bk"] = toFloat(p.Balks)
	raw["gs"] = toFloat(p.GamesStarted)
	raw["sv"] = toFloat(p.Saves)
	raw["hld"] = toFloat(p.Holds)
	raw["bs"] = toFloat(p.BlownSaves)
	raw["pko"] = toFloat(p.Pickoffs)

	hits := toFloat(p.Hits)
	cg := toFloat(p.CompleteGames)
	sho := toFloat(p.Shutouts)

	raw["cg"] = cg
	raw["sho"] = sho

	// Derived: Quality Start (6+ IP, <=3 ER)
	if ip >= 6.0 && raw["er"] <= 3 {
		raw["qs"] = 1
	}

	// Derived: No-Hitter (complete game + 0 hits)
	if cg > 0 && hits == 0 {
		raw["nh"] = 1
		// Derived: Perfect Game (no-hitter + 0 BB + 0 HBP)
		if raw["bb"] == 0 && raw["hb"] == 0 {
			raw["pg"] = 1
		}
	}

	// Derived: Inherited Runners Stranded
	ir := toFloat(p.InheritedRunners)
	irs := toFloat(p.InheritedRunnersScored)
	if ir > 0 {
		raw["irs"] = ir - irs
	}

	return raw
}

// extractHittingRawStats converts MLB API batting stats to our flat map.
func extractHittingRawStats(b battingStats) map[string]float64 {
	raw := make(map[string]float64)
	raw["h"] = toFloat(b.Hits)
	raw["hr"] = toFloat(b.HomeRuns)
	raw["rbi"] = toFloat(b.RBI)
	raw["r"] = toFloat(b.Runs)
	raw["bb"] = toFloat(b.BaseOnBalls)
	raw["sb"] = toFloat(b.StolenBases)
	raw["k"] = toFloat(b.StrikeOuts)
	raw["cs"] = toFloat(b.CaughtStealing)
	return raw
}

// calculateFantasyPoints computes total points from raw stats and scoring weights.
func calculateFantasyPoints(raw map[string]float64, scoringMap map[string]float64) float64 {
	total := 0.0
	for key, value := range raw {
		if weight, ok := scoringMap[key]; ok {
			total += value * weight
		}
	}
	return math.Round(total*100) / 100
}

// parseInningsPitched converts MLB format (e.g. "7.1" = 7 1/3) to decimal innings.
func parseInningsPitched(s string) float64 {
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ".")
	whole := 0.0
	if len(parts) > 0 {
		whole, _ = strconv.ParseFloat(parts[0], 64)
	}
	if len(parts) > 1 {
		fraction, _ := strconv.Atoi(parts[1])
		whole += float64(fraction) / 3.0
	}
	return math.Round(whole*1000) / 1000
}

func toFloat(v json.Number) float64 {
	f, _ := v.Float64()
	return f
}

// --- MLB API box score response types ---

type boxScoreResp struct {
	Teams struct {
		Away boxScoreSide `json:"away"`
		Home boxScoreSide `json:"home"`
	} `json:"teams"`
}

type boxScoreSide struct {
	Team struct {
		Abbreviation string `json:"abbreviation"`
	} `json:"team"`
	Players map[string]boxScorePlayer `json:"players"`
}

type boxScorePlayer struct {
	Person struct {
		ID       int    `json:"id"`
		FullName string `json:"fullName"`
	} `json:"person"`
	Stats struct {
		Pitching pitchingStats `json:"pitching"`
		Batting  battingStats  `json:"batting"`
	} `json:"stats"`
}

type pitchingStats struct {
	InningsPitched         string      `json:"inningsPitched"`
	Hits                   json.Number `json:"hits"`
	EarnedRuns             json.Number `json:"earnedRuns"`
	BaseOnBalls            json.Number `json:"baseOnBalls"`
	StrikeOuts             json.Number `json:"strikeOuts"`
	HomeRuns               json.Number `json:"homeRuns"`
	HitByPitch             json.Number `json:"hitByPitch"`
	Balks                  json.Number `json:"balks"`
	WildPitches            json.Number `json:"wildPitches"`
	Pickoffs               json.Number `json:"pickoffs"`
	GamesStarted           json.Number `json:"gamesStarted"`
	CompleteGames          json.Number `json:"completeGames"`
	Shutouts               json.Number `json:"shutouts"`
	Saves                  json.Number `json:"saves"`
	Holds                  json.Number `json:"holds"`
	BlownSaves             json.Number `json:"blownSaves"`
	InheritedRunners       json.Number `json:"inheritedRunners"`
	InheritedRunnersScored json.Number `json:"inheritedRunnersScored"`
	NumberOfPitches        json.Number `json:"numberOfPitches"`
}

type battingStats struct {
	AtBats           json.Number `json:"atBats"`
	Runs             json.Number `json:"runs"`
	Hits             json.Number `json:"hits"`
	Doubles          json.Number `json:"doubles"`
	Triples          json.Number `json:"triples"`
	HomeRuns         json.Number `json:"homeRuns"`
	RBI              json.Number `json:"rbi"`
	BaseOnBalls      json.Number `json:"baseOnBalls"`
	StrikeOuts       json.Number `json:"strikeOuts"`
	StolenBases      json.Number `json:"stolenBases"`
	CaughtStealing   json.Number `json:"caughtStealing"`
	HitByPitch       json.Number `json:"hitByPitch"`
	PlateAppearances json.Number `json:"plateAppearances"`
}
