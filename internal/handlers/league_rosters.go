package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LeagueTeamSummary struct {
	ID          string
	Name        string
	Owner       string
	Count40Man  int
	Count26Man  int
	CountMinors int
}

func LeagueRostersHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagues, _ := store.GetLeaguesWithTeams(db)

		leagueID := c.Query("league_id")
		if leagueID == "" && len(leagues) > 0 {
			leagueID = leagues[0].ID
		}

		// Fetch teams with roster counts for selected league
		rows, err := db.Query(context.Background(), `
			SELECT t.id, t.name, COALESCE(t.owner_name, ''),
				COUNT(*) FILTER (WHERE p.status_40_man = true) as cnt_40,
				COUNT(*) FILTER (WHERE p.status_26_man = true) as cnt_26,
				COUNT(*) FILTER (WHERE p.status_40_man = false AND COALESCE(p.status_il, '') = '') as cnt_minors
			FROM teams t
			LEFT JOIN players p ON p.team_id = t.id
			WHERE t.league_id = $1
			GROUP BY t.id, t.name, t.owner_name
			ORDER BY t.name
		`, leagueID)

		if err != nil {
			fmt.Printf("ERROR [LeagueRosters]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}
		defer rows.Close()

		var teams []LeagueTeamSummary
		for rows.Next() {
			var t LeagueTeamSummary
			if err := rows.Scan(&t.ID, &t.Name, &t.Owner, &t.Count40Man, &t.Count26Man, &t.CountMinors); err != nil {
				continue
			}
			teams = append(teams, t)
		}

		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "league_rosters.html", gin.H{
			"User":      user,
			"Leagues":   leagues,
			"LeagueID":  leagueID,
			"Teams":     teams,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}

func BidCalculatorHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "bid_calculator.html", gin.H{
			"User":      user,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}

type WaiverAuditPlayer struct {
	ID              string
	FirstName       string
	LastName        string
	Position        string
	LeagueName      string
	WaivingTeamName string
	WaiverEndTime   time.Time
	IsExpired       bool
	TimeRemaining   string
	ClaimingTeams   []string
}

func AdminWaiverAuditHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Access denied")
			return
		}

		rows, err := db.Query(context.Background(), `
			SELECT p.id, p.first_name, p.last_name, p.position,
				COALESCE(l.name, 'Unknown') as league_name,
				COALESCE(wt.name, p.waiving_team_id::TEXT, 'Unknown') as waiving_team,
				COALESCE(p.waiver_end_time, NOW())
			FROM players p
			LEFT JOIN leagues l ON p.league_id = l.id
			LEFT JOIN teams wt ON p.waiving_team_id = wt.id
			WHERE p.fa_status = 'on waivers'
			ORDER BY p.waiver_end_time ASC
		`)
		if err != nil {
			fmt.Printf("ERROR [WaiverAudit]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}
		defer rows.Close()

		var players []WaiverAuditPlayer
		for rows.Next() {
			var p WaiverAuditPlayer
			if err := rows.Scan(&p.ID, &p.FirstName, &p.LastName, &p.Position, &p.LeagueName, &p.WaivingTeamName, &p.WaiverEndTime); err != nil {
				continue
			}
			if time.Now().After(p.WaiverEndTime) {
				p.IsExpired = true
			} else {
				remaining := time.Until(p.WaiverEndTime)
				hours := int(remaining.Hours())
				minutes := int(remaining.Minutes()) % 60
				if hours > 0 {
					p.TimeRemaining = fmt.Sprintf("%dh %dm left", hours, minutes)
				} else {
					p.TimeRemaining = fmt.Sprintf("%dm left", minutes)
				}
			}
			players = append(players, p)
		}

		// Fetch claiming teams for each player
		for i, p := range players {
			claimRows, err := db.Query(context.Background(), `
				SELECT COALESCE(t.name, 'Unknown')
				FROM waiver_claims wc
				JOIN teams t ON wc.team_id = t.id
				WHERE wc.player_id = $1
				ORDER BY wc.claim_priority ASC
			`, p.ID)
			if err != nil {
				continue
			}
			for claimRows.Next() {
				var teamName string
				claimRows.Scan(&teamName)
				players[i].ClaimingTeams = append(players[i].ClaimingTeams, teamName)
			}
			claimRows.Close()
		}

		RenderTemplate(c, "admin_waiver_audit.html", gin.H{
			"User":          user,
			"WaiverPlayers": players,
			"IsCommish":     true,
		})
	}
}
