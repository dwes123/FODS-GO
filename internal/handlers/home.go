package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WaiverSpotlightPlayer struct {
	ID            string
	FirstName     string
	LastName      string
	Position      string
	LeagueName    string
	TimeRemaining string
	IsExpired     bool
}

func getWaiverSpotlight(db *pgxpool.Pool) []WaiverSpotlightPlayer {
	rows, err := db.Query(context.Background(), `
		SELECT p.id, p.first_name, p.last_name, p.position, COALESCE(p.waiver_end_time, NOW()),
		       COALESCE(l.name, '')
		FROM players p
		LEFT JOIN leagues l ON p.league_id = l.id
		WHERE p.fa_status = 'on waivers'
		ORDER BY p.waiver_end_time ASC
		LIMIT 5
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var players []WaiverSpotlightPlayer
	for rows.Next() {
		var p WaiverSpotlightPlayer
		var endTime time.Time
		if err := rows.Scan(&p.ID, &p.FirstName, &p.LastName, &p.Position, &endTime, &p.LeagueName); err != nil {
			continue
		}
		if time.Now().After(endTime) {
			p.IsExpired = true
		} else {
			remaining := time.Until(endTime)
			hours := int(remaining.Hours())
			minutes := int(remaining.Minutes()) % 60
			if hours > 0 {
				p.TimeRemaining = fmt.Sprintf("%dh %dm", hours, minutes)
			} else {
				p.TimeRemaining = fmt.Sprintf("%dm", minutes)
			}
		}
		players = append(players, p)
	}
	return players
}

func HomeHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Get All Leagues
		leagues, _ := store.GetLeaguesWithTeams(db)

		// 2. Get My Managed Teams
		var myTeams []store.TeamDetail
		var isCommish bool
		userVal, exists := c.Get("user")
		if exists {
			user := userVal.(*store.User)
			myTeams, _ = store.GetManagedTeams(db, user.ID)
			adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
			isCommish = len(adminLeagues) > 0 || user.Role == "admin"
		}

		// 3. Get Recent Activity
		activities, _ := store.GetRecentActivity(db, 5, "")

		// 4. Get Upcoming Dates (Filtered by league query param)
		selectedLeague := c.Query("league")
		keyDates, _ := store.GetKeyDates(db, selectedLeague)

		// 5. Get Waiver Wire Spotlight
		waiverSpotlight := getWaiverSpotlight(db)

		data := gin.H{
			"Leagues":         leagues,
			"MyTeams":         myTeams,
			"User":            userVal,
			"IsCommish":       isCommish,
			"Activities":      activities,
			"KeyDates":        keyDates,
			"SelectedLeague":  selectedLeague,
			"WaiverSpotlight": waiverSpotlight,
		}

		RenderTemplate(c, "home.html", data)
	}
}
