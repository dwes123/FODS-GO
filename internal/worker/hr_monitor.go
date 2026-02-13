package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/notification"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StartHRMonitor polls the MLB Stats API for home runs during game hours
// and posts Slack notifications when rostered players hit a HR.
// Runs during MLB season (April-October), 1 PM - midnight ET.
func StartHRMonitor(db *pgxpool.Pool) {
	go func() {
		seenPlays := make(map[string]bool)
		var mu sync.Mutex

		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			now := time.Now()
			loc, _ := time.LoadLocation("America/New_York")
			et := now.In(loc)

			// Only run April-October, 1 PM - midnight ET
			month := et.Month()
			hour := et.Hour()
			if month < 4 || month > 10 || hour < 13 {
				continue
			}

			// Reset seen plays at midnight
			if hour == 0 {
				mu.Lock()
				seenPlays = make(map[string]bool)
				mu.Unlock()
				continue
			}

			mu.Lock()
			checkHomeRuns(db, seenPlays)
			mu.Unlock()
		}
	}()
}

type mlbScheduleResp struct {
	Dates []struct {
		Games []struct {
			GamePk int    `json:"gamePk"`
			Status struct {
				AbstractGameState string `json:"abstractGameState"`
			} `json:"status"`
		} `json:"games"`
	} `json:"dates"`
}

type mlbLiveResp struct {
	LiveData struct {
		Plays struct {
			AllPlays []struct {
				Result struct {
					Event       string `json:"event"`
					Description string `json:"description"`
				} `json:"result"`
				About struct {
					AtBatIndex int  `json:"atBatIndex"`
					IsComplete bool `json:"isComplete"`
				} `json:"about"`
				MatchUp struct {
					Batter struct {
						ID       int    `json:"id"`
						FullName string `json:"fullName"`
					} `json:"batter"`
				} `json:"matchup"`
			} `json:"allPlays"`
		} `json:"plays"`
	} `json:"liveData"`
}

func checkHomeRuns(db *pgxpool.Pool, seenPlays map[string]bool) {
	today := time.Now().Format("2006-01-02")
	url := fmt.Sprintf("https://statsapi.mlb.com/api/v1/schedule?sportId=1&date=%s", today)

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var schedule mlbScheduleResp
	if err := json.Unmarshal(body, &schedule); err != nil {
		return
	}

	for _, date := range schedule.Dates {
		for _, game := range date.Games {
			if game.Status.AbstractGameState != "Live" {
				continue
			}
			checkGameForHRs(db, game.GamePk, seenPlays)
		}
	}
}

func checkGameForHRs(db *pgxpool.Pool, gamePk int, seenPlays map[string]bool) {
	url := fmt.Sprintf("https://statsapi.mlb.com/api/v1.1/game/%d/feed/live", gamePk)
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var live mlbLiveResp
	if err := json.Unmarshal(body, &live); err != nil {
		return
	}

	ctx := context.Background()
	for _, play := range live.LiveData.Plays.AllPlays {
		if !play.About.IsComplete || play.Result.Event != "Home Run" {
			continue
		}

		playKey := fmt.Sprintf("%d_%d", gamePk, play.About.AtBatIndex)
		if seenPlays[playKey] {
			continue
		}
		seenPlays[playKey] = true

		mlbID := play.MatchUp.Batter.ID
		playerName := play.MatchUp.Batter.FullName

		// Check if this player is rostered in any league
		rows, err := db.Query(ctx, `
			SELECT p.league_id, t.name
			FROM players p
			JOIN teams t ON p.team_id = t.id
			WHERE p.mlb_id = $1 AND p.team_id IS NOT NULL
		`, mlbID)
		if err != nil {
			continue
		}

		for rows.Next() {
			var leagueID, teamName string
			if err := rows.Scan(&leagueID, &teamName); err != nil {
				continue
			}

			msg := fmt.Sprintf("⚾ *HOME RUN!* %s (rostered by *%s*) just hit a home run!", playerName, teamName)
			notification.SendSlackNotification(db, leagueID, "transaction", msg)
		}
		rows.Close()
	}
}
