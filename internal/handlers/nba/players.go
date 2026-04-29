package nba

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/dwes123/fantasy-baseball-go/internal/handlers"
	"github.com/dwes123/fantasy-baseball-go/internal/sport"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	nbastore "github.com/dwes123/fantasy-baseball-go/internal/store/nba"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FreeAgentHandler renders the NBA free-agent search page.
func FreeAgentHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)

		search := c.Query("q")
		pos := c.Query("pos")
		page, _ := strconv.Atoi(c.Query("page"))
		if page < 1 {
			page = 1
		}
		const perPage = 50

		filter := nbastore.FreeAgentFilter{
			Search:   search,
			Position: pos,
			Limit:    perPage,
			Offset:   (page - 1) * perPage,
		}

		players, err := nbastore.GetFreeAgents(nbaDB, filter)
		if err != nil {
			fmt.Printf("ERROR [NBA FreeAgents]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		totalCount, err := nbastore.CountFreeAgents(nbaDB, filter)
		if err != nil {
			fmt.Printf("ERROR [NBA FreeAgents count]: %v\n", err)
		}
		totalPages := (totalCount + perPage - 1) / perPage
		if totalPages < 1 {
			totalPages = 1
		}

		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)

		handlers.RenderTemplate(c, "nba/free_agents.html", gin.H{
			"Sport":       sport.SportNBA,
			"User":        user,
			"IsCommish":   len(adminLeagues) > 0 || user.Role == "admin",
			"Players":     players,
			"Search":      search,
			"Pos":         pos,
			"Positions":   sport.NBAPositions,
			"CurrentPage": page,
			"TotalPages":  totalPages,
			"TotalCount":  totalCount,
		})
	}
}

// PlayerProfileHandler renders an individual NBA player's profile.
func PlayerProfileHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		player, err := nbastore.GetPlayerByID(nbaDB, id)
		if err != nil {
			c.String(http.StatusNotFound, "Player not found")
			return
		}

		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)

		var isOwner bool
		var teamIDStr string
		if player.TeamID != nil && *player.TeamID != "" {
			teamIDStr = *player.TeamID
			isOwner, _ = nbastore.IsTeamOwner(nbaDB, teamIDStr, user.ID)
		}

		// Build the contract year list — only include years that have a contract value
		// OR an annotation. Empty years are dropped from the strip entirely so the visual
		// matches the player's actual deal length.
		years := make([]int, 0, 15)
		for y := 2026; y <= 2040; y++ {
			yKey := fmt.Sprintf("%d", y)
			hasContract := false
			if v, ok := player.Contracts[y]; ok && v != "" {
				hasContract = true
			}
			hasAnnotation := false
			if anns, ok := player.Annotations[yKey]; ok && len(anns) > 0 {
				hasAnnotation = true
			}
			if hasContract || hasAnnotation {
				years = append(years, y)
			}
		}
		hasContracts := len(years) > 0

		handlers.RenderTemplate(c, "nba/player_profile.html", gin.H{
			"Sport":         sport.SportNBA,
			"User":          user,
			"IsCommish":     len(adminLeagues) > 0 || user.Role == "admin",
			"Player":        player,
			"TeamIDStr":     teamIDStr,
			"IsOwner":       isOwner,
			"ContractYears": years,
			"HasContracts":  hasContracts,
		})
	}
}

// TradeBlockHandler renders all NBA players currently flagged on the trade block.
func TradeBlockHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)

		players, err := nbastore.GetTradeBlockPlayers(nbaDB)
		if err != nil {
			fmt.Printf("ERROR [NBA TradeBlock]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		// Group by team for display
		type teamGroup struct {
			TeamName string
			Players  []nbastore.Player
		}
		groupsByTeam := map[string]*teamGroup{}
		var teamOrder []string
		for _, p := range players {
			tn := p.TeamName
			if tn == "" {
				tn = "Unassigned"
			}
			if _, ok := groupsByTeam[tn]; !ok {
				groupsByTeam[tn] = &teamGroup{TeamName: tn}
				teamOrder = append(teamOrder, tn)
			}
			groupsByTeam[tn].Players = append(groupsByTeam[tn].Players, p)
		}
		var groups []*teamGroup
		for _, tn := range teamOrder {
			groups = append(groups, groupsByTeam[tn])
		}

		handlers.RenderTemplate(c, "nba/trade_block.html", gin.H{
			"Sport":     sport.SportNBA,
			"User":      user,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
			"Groups":    groups,
			"Total":     len(players),
		})
	}
}
