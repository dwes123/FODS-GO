package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
			fmt.Printf("ERROR [ProcessOptionDecision]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		c.Redirect(http.StatusFound, "/team-options")
	}
}

func SubmitExtensionHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		playerID := c.PostForm("player_id")
		years, _ := strconv.Atoi(c.PostForm("years"))
		aav, _ := strconv.ParseFloat(c.PostForm("aav"), 64)

		if years < 1 || years > 8 || aav <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid extension parameters"})
			return
		}

		player, _ := store.GetPlayerByID(db, playerID)

		isOwner, _ := store.IsTeamOwner(db, player.TeamID, user.ID)
		if !isOwner && user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
			return
		}

		// Extension deadline enforcement
		now := time.Now()
		deadline, err := store.GetLeagueDateValue(db, player.LeagueID, now.Year(), "extension_deadline")
		if err == nil && now.After(deadline) {
			c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("The extension deadline has passed (%s).", deadline.Format("January 2, 2006"))})
			return
		}

		// Check ARB eligibility — can't extend if more than 1 ARB year remaining
		arbCount := 0
		for yr := now.Year(); yr <= 2040; yr++ {
			upper := strings.ToUpper(strings.TrimSpace(player.Contracts[yr]))
			if strings.HasPrefix(upper, "ARB") {
				arbCount++
			}
		}
		if arbCount > 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Player has %d arbitration years remaining. Players with more than 1 ARB year cannot be extended.", arbCount)})
			return
		}

		// Find the first contract year without a dollar amount
		// Non-dollar values like UFA, TC, ARB, ARB 1, ARB 2, ARB 3 don't count
		startYear := 0
		for yr := now.Year(); yr <= 2040; yr++ {
			val := player.Contracts[yr]
			if !isDollarContract(val) {
				startYear = yr
				break
			}
		}
		if startYear == 0 || startYear+years-1 > 2040 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Not enough contract years available for this extension length"})
			return
		}

		// Build year-by-year contract map
		salaries := make(map[string]float64)
		for i := 0; i < years; i++ {
			salaries[fmt.Sprintf("%d", startYear+i)] = aav
		}

		contractData, _ := json.Marshal(salaries)
		summary := fmt.Sprintf("Extension request for %s %s: %d years at $%s AAV (years %d-%d)",
			player.FirstName, player.LastName, years, formatDollar(aav), startYear, startYear+years-1)

		err = store.CreatePendingAction(db, playerID, player.LeagueID, player.TeamID, "EXTENSION", summary, contractData)

		if err != nil {
			fmt.Printf("ERROR [SubmitExtension]: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		store.LogActivity(db, player.LeagueID, player.TeamID, "Roster Move",
			fmt.Sprintf("%s submitted an extension request for %s %s.", user.Username, player.FirstName, player.LastName))

		c.JSON(http.StatusOK, gin.H{"message": "Extension request submitted for commissioner approval."})
	}
}

func formatDollar(amount float64) string {
	s := fmt.Sprintf("%.0f", amount)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// isDollarContract returns true if the contract value is a real dollar amount
// (not empty, UFA, TC, ARB, ARB 1, ARB 2, ARB 3, etc.)
func isDollarContract(val string) bool {
	if val == "" {
		return false
	}
	upper := strings.ToUpper(strings.TrimSpace(val))
	if upper == "UFA" || upper == "TC" || strings.HasPrefix(upper, "ARB") {
		return false
	}
	return true
}

func ProcessRestructureHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		playerID := c.PostForm("player_id")
		fromYear := c.PostForm("from_year")
		toYear := c.PostForm("to_year")
		amount := c.PostForm("amount")

		player, _ := store.GetPlayerByID(db, playerID)

		isOwner, _ := store.IsTeamOwner(db, player.TeamID, user.ID)
		if !isOwner && user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
			return
		}

		details := fmt.Sprintf("Move $%s from %s to %s", amount, fromYear, toYear)

		restructureData, _ := json.Marshal(map[string]string{
			"from_year": fromYear,
			"to_year":   toYear,
			"amount":    amount,
		})

		err := store.CreatePendingAction(db, playerID, player.LeagueID, player.TeamID, "RESTRUCTURE",
			fmt.Sprintf("Restructure request for %s %s: %s", player.FirstName, player.LastName, details), restructureData)

		if err != nil {
			fmt.Printf("ERROR [ProcessRestructure]: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		store.LogActivity(db, player.LeagueID, player.TeamID, "Roster Move",
			fmt.Sprintf("%s submitted a restructure request for %s %s.", user.Username, player.FirstName, player.LastName))

		c.JSON(http.StatusOK, gin.H{"message": "Restructure request submitted for commissioner approval."})
	}
}