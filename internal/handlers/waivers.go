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

// WaiverWireHandler displays players currently on waivers
func WaiverWireHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagueID := c.Query("league_id")

		// Build league list from user's managed teams only
		myTeams, _ := store.GetManagedTeams(db, user.ID)
		type UserLeague struct {
			ID   string
			Name string
		}
		seen := map[string]bool{}
		var userLeagues []UserLeague
		for _, t := range myTeams {
			if !seen[t.LeagueID] {
				seen[t.LeagueID] = true
				userLeagues = append(userLeagues, UserLeague{ID: t.LeagueID, Name: t.LeagueName})
			}
		}

		// Default to first of user's leagues
		if leagueID == "" && len(userLeagues) > 0 {
			leagueID = userLeagues[0].ID
		}
		if leagueID == "" {
			leagueID = "11111111-1111-1111-1111-111111111111"
		}

		query := `
			SELECT id, first_name, last_name, position, COALESCE(mlb_team, ''),
				COALESCE(waiver_end_time, NOW()), COALESCE(waiving_team_id::TEXT, '')
			FROM players
			WHERE fa_status = 'on waivers' AND league_id = $1
			ORDER BY waiver_end_time ASC
		`

		rows, err := db.Query(context.Background(), query, leagueID)
		if err != nil {
			fmt.Printf("ERROR [WaiverWire]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}
		defer rows.Close()

		type WaiverPlayer struct {
			ID            string
			FirstName     string
			LastName      string
			Position      string
			MLBTeam       string
			WaiverEndTime time.Time
			WaivingTeamID string
		}

		var players []WaiverPlayer
		for rows.Next() {
			var p WaiverPlayer
			if err := rows.Scan(&p.ID, &p.FirstName, &p.LastName, &p.Position, &p.MLBTeam, &p.WaiverEndTime, &p.WaivingTeamID); err == nil {
				players = append(players, p)
			}
		}

		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		
		RenderTemplate(c, "waiver_wire.html", gin.H{
			"User":      user,
			"Players":   players,
			"Leagues":   userLeagues,
			"LeagueID":  leagueID,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}

// ClaimWaiverHandler processes a user's claim on a player
func ClaimWaiverHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		playerID := c.PostForm("player_id")
		user := c.MustGet("user").(*store.User)

		var teamID, leagueID, teamName string
		err := db.QueryRow(context.Background(),
			`SELECT t.id, t.league_id, t.name FROM teams t
			 JOIN team_owners town ON t.id = town.team_id
			 WHERE town.user_id = $1 LIMIT 1`, user.ID).Scan(&teamID, &leagueID, &teamName)
		
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "You do not manage a team."})
			return
		}

		// Check if player is actually on waivers
		var faStatus string
		db.QueryRow(context.Background(), "SELECT fa_status FROM players WHERE id = $1", playerID).Scan(&faStatus)
		if faStatus != "on waivers" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Player is not on waivers."})
			return
		}

		// Check if already claimed by this team
		var exists int
		db.QueryRow(context.Background(), 
			"SELECT COUNT(*) FROM waiver_claims WHERE team_id = $1 AND player_id = $2", teamID, playerID).Scan(&exists)
		
		if exists > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "You have already claimed this player."})
			return
		}

		// Insert Claim
		// Note: Priority is 0 for now, real implementation would fetch current priority from standings/settings
		_, err = db.Exec(context.Background(),
			"INSERT INTO waiver_claims (league_id, team_id, player_id, claim_priority) VALUES ($1, $2, $3, 0)",
			leagueID, teamID, playerID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		// Notify
		// var pName string
		// db.QueryRow(context.Background(), "SELECT first_name || ' ' || last_name FROM players WHERE id = $1", playerID).Scan(&pName)
		
		// msg := fmt.Sprintf("ðŸ‘€ *Waiver Claim:* *%s* has put a claim on *%s*.", teamName, pName)
		// notification.SendSlackNotification(db, leagueID, "transaction", msg)

		c.JSON(http.StatusOK, gin.H{"message": "Claim submitted successfully! Processing will occur when waivers expire."})
	}
}
