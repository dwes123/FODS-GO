package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

var standardPositions = map[string]bool{
	"C": true, "1B": true, "2B": true, "SS": true, "3B": true, "OF": true, "SP": true, "RP": true,
}

var positionAliases = map[string]string{
	"CF": "OF", "LF": "OF", "RF": "OF", "DH": "OF", "UT": "OF",
	"P": "SP", "RHP": "SP", "LHP": "SP",
}

func normalizePosition(pos string) string {
	pos = strings.ToUpper(strings.TrimSpace(pos))
	// Take first position from multi-position strings like "SP,RP" or "1B,OF"
	if idx := strings.Index(pos, ","); idx != -1 {
		pos = strings.TrimSpace(pos[:idx])
	}
	if standardPositions[pos] {
		return pos
	}
	if mapped, ok := positionAliases[pos]; ok {
		return mapped
	}
	return "OF" // fallback for anything truly unknown
}

func RosterHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID := c.Param("id")

		team, err := store.GetTeamWithRoster(db, teamID)
		if err != nil {
			c.String(http.StatusNotFound, "Team not found")
			return
		}

		posOrder := []string{"C", "1B", "2B", "SS", "3B", "OF", "SP", "RP"}

		// Categorize players — normalize position before bucketing
		roster26 := make(map[string][]store.RosterPlayer)
		roster40 := make(map[string][]store.RosterPlayer)
		minors := make(map[string][]store.RosterPlayer)

		for _, p := range team.Players {
			bucket := normalizePosition(p.Position)
			if p.Status26Man {
				roster26[bucket] = append(roster26[bucket], p)
			} else if p.Status40Man {
				roster40[bucket] = append(roster40[bucket], p)
			} else {
				minors[bucket] = append(minors[bucket], p)
			}
		}

		// Compute roster counts
		count26 := 0
		for _, players := range roster26 {
			count26 += len(players)
		}
		count40only := 0
		for _, players := range roster40 {
			count40only += len(players)
		}
		countMinors := 0
		for _, players := range minors {
			countMinors += len(players)
		}
		count40 := count26 + count40only
		spCount := len(roster26["SP"])

		settings := store.GetLeagueSettings(db, team.LeagueID, time.Now().Year())

		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		data := gin.H{
			"Team":           team,
			"Roster26":       roster26,
			"Roster40":       roster40,
			"Minors":         minors,
			"PosOrder":       posOrder,
			"User":           user,
			"IsCommish":      len(adminLeagues) > 0,
			"Count26":        count26,
			"Limit26":        settings.Roster26ManLimit,
			"Count40":        count40,
			"Limit40":        settings.Roster40ManLimit,
			"CountMinors":    countMinors,
			"SPCount":        spCount,
			"SPLimit":        settings.SP26ManLimit,
		}

		RenderTemplate(c, "roster.html", data)
	}
}

func SaveDepthOrderHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			TeamID string   `json:"team_id"`
			Order  []string `json:"order"`
		}
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		user := c.MustGet("user").(*store.User)
		isOwner, _ := store.IsTeamOwner(db, req.TeamID, user.ID)
		if !isOwner {
			c.JSON(http.StatusForbidden, gin.H{"error": "You do not own this team"})
			return
		}

		ctx := context.Background()
		for i, playerID := range req.Order {
			db.Exec(ctx,
				`UPDATE players SET depth_rank = $1 WHERE id = $2 AND team_id = $3`,
				i+1, playerID, req.TeamID)
		}

		c.JSON(http.StatusOK, gin.H{"message": "Depth order saved"})
	}
}
