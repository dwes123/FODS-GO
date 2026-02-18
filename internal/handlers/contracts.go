package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TeamOptionsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		
		players2026, _ := store.GetPlayersWithOptions(db, "", 2026)
		players2027, _ := store.GetPlayersWithOptions(db, "", 2027)
		allPlayers := append(players2026, players2027...)

		var myOptions []store.OptionPlayer
		myTeams, _ := store.GetManagedTeams(db, user.ID)
		isAdmin := user.Role == "admin"
		
		for _, p := range allPlayers {
			isOwner := false
			for _, mt := range myTeams {
				if mt.ID == p.TeamID { isOwner = true; break }
			}
			if isOwner || isAdmin {
				myOptions = append(myOptions, p)
			}
		}

		RenderTemplate(c, "team_options.html", gin.H{
			"User":    user,
			"Options": myOptions,
		})
	}
}

func ProcessOptionDecisionHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		playerID := c.PostForm("player_id")
		year, _ := strconv.Atoi(c.PostForm("year"))
		action := c.PostForm("action")

		player, _ := store.GetPlayerByID(db, playerID)
		isOwner, _ := store.IsTeamOwner(db, player.TeamID, user.ID)

		if !isOwner && user.Role != "admin" {
			c.String(http.StatusForbidden, "Unauthorized")
			return
		}

		// Team option deadline enforcement
		now := time.Now()
		deadline, deadlineErr := store.GetLeagueDateValue(db, player.LeagueID, now.Year(), "option_deadline")
		if deadlineErr == nil && now.After(deadline) {
			c.String(http.StatusForbidden, "The team option deadline has passed (%s). Option decisions are no longer accepted.", deadline.Format("January 2, 2006"))
			return
		}

		err := store.ProcessOptionDecision(db, playerID, year, action)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error: %v", err)
			return
		}

		c.Redirect(http.StatusFound, "/team-options")
	}
}

func SubmitExtensionHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		playerID := c.PostForm("player_id")
		details := c.PostForm("details")

		player, _ := store.GetPlayerByID(db, playerID)

		// Extension deadline enforcement
		now := time.Now()
		deadline, err := store.GetLeagueDateValue(db, player.LeagueID, now.Year(), "extension_deadline")
		if err == nil && now.After(deadline) {
			c.String(http.StatusForbidden, "The extension deadline has passed (%s). Extensions are no longer accepted.", deadline.Format("January 2, 2006"))
			return
		}

		err = store.CreatePendingAction(db, player.LeagueID, player.TeamID, "EXTENSION", 
			fmt.Sprintf("Extension request for %s %s: %s", player.FirstName, player.LastName, details))
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		store.LogActivity(db, player.LeagueID, player.TeamID, "Extension Request", 
			fmt.Sprintf("%s submitted an extension request for %s %s.", user.Username, player.FirstName, player.LastName))

		c.Redirect(http.StatusFound, "/player/"+playerID)
	}
}

func ProcessRestructureHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		playerID := c.PostForm("player_id")
		details := c.PostForm("details")

		player, _ := store.GetPlayerByID(db, playerID)
		
		err := store.CreatePendingAction(db, player.LeagueID, player.TeamID, "RESTRUCTURE", 
			fmt.Sprintf("Restructure request for %s %s: %s", player.FirstName, player.LastName, details))
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		store.LogActivity(db, player.LeagueID, player.TeamID, "Restructure Request", 
			fmt.Sprintf("%s submitted a restructure request for %s %s.", user.Username, player.FirstName, player.LastName))

		c.Redirect(http.StatusFound, "/player/"+playerID)
	}
}