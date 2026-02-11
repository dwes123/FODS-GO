package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func AdminDashboardHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}
		var actions []store.PendingAction
		if user.Role == "admin" {
			actions, _ = store.GetPendingActionsForLeagues(db, nil)
		} else {
			actions, _ = store.GetPendingActionsForLeagues(db, adminLeagues)
		}
		RenderTemplate(c, "admin_dashboard.html", gin.H{
			"User":      user,
			"Actions":   actions,
			"IsCommish": true,
		})
	}
}

func ProcessActionHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		actionID := c.PostForm("action_id")
		status := c.PostForm("status")
		var actionLeagueID string
		db.QueryRow(c, "SELECT league_id FROM pending_actions WHERE id = $1", actionID).Scan(&actionLeagueID)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		isLeagueAdmin := false
		for _, l := range adminLeagues { if l == actionLeagueID { isLeagueAdmin = true; break } }
		if !isLeagueAdmin && user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized for this league"})
			return
		}
		err := store.ProcessAction(db, actionID, status)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Redirect(http.StatusFound, "/admin/")
	}
}

func AdminPlayerEditorHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if user.Role != "admin" {
			c.String(http.StatusForbidden, "Global Admin Only")
			return
		}
		playerID := c.Query("player_id")
		searchQuery := c.Query("q")
		var player *store.RosterPlayer
		searchResults := []store.RosterPlayer{}
		if playerID != "" {
			player, _ = store.GetPlayerByID(db, playerID)
		} else if searchQuery != "" {
			searchResults, _ = store.SearchAllPlayers(db, searchQuery)
		}
		leagues, _ := store.GetLeaguesWithTeams(db)
		RenderTemplate(c, "admin_player_editor.html", gin.H{
			"User":          user,
			"Player":        player,
			"SearchResults": searchResults,
			"SearchQuery":   searchQuery,
			"Leagues":       leagues,
			"IsCommish":     true,
		})
	}
}

func AdminSavePlayerHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		contracts := make(map[string]string)
		for y := 2026; y <= 2040; y++ {
			ys := fmt.Sprintf("%d", y)
			contracts[ys] = c.PostForm("contract_" + ys)
		}
		optYears, _ := strconv.Atoi(c.PostForm("option_years"))
		update := store.PlayerAdminUpdate{
			ID:          c.PostForm("player_id"),
			FirstName:   c.PostForm("first_name"),
			LastName:    c.PostForm("last_name"),
			Position:    c.PostForm("position"),
			MLBTeam:     c.PostForm("mlb_team"),
			TeamID:      c.PostForm("team_id"),
			LeagueID:    c.PostForm("league_id"),
			Status40Man: c.PostForm("status_40_man") == "on",
			Status26Man: c.PostForm("status_26_man") == "on",
			StatusIL:    c.PostForm("status_il"),
			OptionYears: optYears,
			Contracts:   contracts,
		}
		var err error
		if update.ID == "" {
			newID, errCreate := store.AdminCreatePlayer(db, update)
			update.ID = newID
			err = errCreate
		} else {
			err = store.AdminUpdatePlayer(db, update)
		}
		if err != nil {
			c.String(http.StatusInternalServerError, "Error saving player: %v", err)
			return
		}
		c.Redirect(http.StatusFound, "/admin/player-editor?player_id=" + update.ID)
	}
}

// --- DEAD CAP ADMIN ---

func AdminDeadCapHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if user.Role != "admin" {
			c.String(http.StatusForbidden, "Global Admin Only")
			return
		}
		teamID := c.Query("team_id")
		entries, _ := store.GetDeadCapForAdmin(db, teamID)
		leagues, _ := store.GetLeaguesWithTeams(db)

		RenderTemplate(c, "admin_dead_cap.html", gin.H{
			"User":      user,
			"Entries":   entries,
			"Leagues":   leagues,
			"IsCommish": true,
		})
	}
}

func AdminSaveDeadCapHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID := c.PostForm("team_id")
		playerID := c.PostForm("player_id")
		amount, _ := strconv.ParseFloat(c.PostForm("amount"), 64)
		year, _ := strconv.Atoi(c.PostForm("year"))
		note := c.PostForm("note")

		err := store.AddDeadCapPenalty(db, teamID, playerID, amount, year, note)
		if err != nil {
			c.String(500, "Error: %v", err)
			return
		}
		c.Redirect(http.StatusFound, "/admin/dead-cap")
	}
}

func AdminDeleteDeadCapHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.PostForm("id")
		store.DeleteDeadCapPenalty(db, id)
		c.Redirect(http.StatusFound, "/admin/dead-cap")
	}
}
