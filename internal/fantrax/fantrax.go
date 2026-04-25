package fantrax

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Standing is a single team row from Fantrax's getStandings JSON.
type Standing struct {
	Rank           int     `json:"rank"`
	TeamID         string  `json:"teamId"`
	TeamName       string  `json:"teamName"`
	Record         string  `json:"points"`
	WinPercentage  float64 `json:"winPercentage"`
	GamesBack      float64 `json:"gamesBack"`
	TotalPointsFor float64 `json:"totalPointsFor"`
}

type cacheEntry struct {
	data       []Standing
	pacificDay string
}

var (
	cacheMu sync.RWMutex
	cache   = map[string]cacheEntry{}
)

// pacificDay returns the YYYY-MM-DD date in Pacific time. Cache entries are
// keyed by (url, pacificDay) so they refresh once daily at midnight PT.
func pacificDay(t time.Time) string {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PT", -8*3600)
	}
	return t.In(loc).Format("2006-01-02")
}

// Fetch returns Fantrax standings for the given URL, caching results per
// Pacific day. The first request after midnight PT triggers a fresh fetch.
func Fetch(url string) ([]Standing, error) {
	today := pacificDay(time.Now())

	cacheMu.RLock()
	entry, ok := cache[url]
	cacheMu.RUnlock()
	if ok && entry.pacificDay == today {
		return entry.data, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fantrax status %d", resp.StatusCode)
	}

	var standings []Standing
	if err := json.NewDecoder(resp.Body).Decode(&standings); err != nil {
		return nil, err
	}

	cacheMu.Lock()
	cache[url] = cacheEntry{data: standings, pacificDay: today}
	cacheMu.Unlock()

	return standings, nil
}

// Invalidate clears the cached standings for a URL, forcing a fresh fetch on
// the next call. Use when manual refresh is needed (e.g. admin action).
func Invalidate(url string) {
	cacheMu.Lock()
	delete(cache, url)
	cacheMu.Unlock()
}
