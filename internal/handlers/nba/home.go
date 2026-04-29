package nba

import (
	"github.com/dwes123/fantasy-baseball-go/internal/handlers"
	"github.com/dwes123/fantasy-baseball-go/internal/sport"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	nbastore "github.com/dwes123/fantasy-baseball-go/internal/store/nba"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HomeHandler renders the NBA dashboard. Personalized to the logged-in user:
// shows their team snapshot, action items (pending offers / match windows),
// recent league activity, and quick-access tiles.
func HomeHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userVal, _ := c.Get("user")
		user, _ := userVal.(*store.User)

		var (
			isCommish        bool
			ownedTeams       []nbastore.Team
			primaryTeam      *nbastore.Team
			primaryRoster    []nbastore.Player
			pendingAgent     []*nbastore.Offer
			openMatch        []*nbastore.Offer
			openOutgoing     []*nbastore.Offer
			recentActivity   []*nbastore.Offer
			isAgent          bool
			poolFAs          int
			poolPending      int
		)

		if user != nil {
			adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)
			isCommish = len(adminLeagues) > 0 || user.Role == "admin"

			// User's owned teams + primary
			ownedTeams, _ = nbastore.GetManagedNBATeams(nbaDB, user.ID)
			if len(ownedTeams) > 0 {
				full, err := nbastore.GetTeamWithRoster(nbaDB, ownedTeams[0].ID)
				if err == nil {
					primaryTeam = full
					primaryRoster = full.Roster
				}
			}

			// Agent dashboard summary across all agencies the user belongs to
			agencyIDs, _ := nbastore.AgenciesForUser(nbaDB, user.ID)
			isAgent = len(agencyIDs) > 0
			for _, aid := range agencyIDs {
				p, _ := nbastore.ListPendingForAgency(nbaDB, aid)
				pendingAgent = append(pendingAgent, p...)
			}

			// Match windows + outgoing offers across all owned teams
			for _, t := range ownedTeams {
				m, _ := nbastore.ListAwaitingMatchForTeam(nbaDB, t.ID)
				openMatch = append(openMatch, m...)
				out, _ := nbastore.ListOutgoingForTeam(nbaDB, t.ID, 10)
				for _, o := range out {
					switch o.Status {
					case nbastore.OfferPendingAgent, nbastore.OfferAgentCountered, nbastore.OfferAwaitingMatch:
						openOutgoing = append(openOutgoing, o)
					}
				}
			}
		}

		// League-wide recent signings
		recentActivity, _ = nbastore.ListRecentSignings(nbaDB, 8)

		// Pool sizes for the hero stats
		poolFAs, _ = nbastore.CountFreeAgents(nbaDB, nbastore.FreeAgentFilter{})
		// Pending count: rostered players whose fa_class is Pending
		nbaDB.QueryRow(c, `SELECT COUNT(*) FROM players WHERE fa_class = 'Pending'`).Scan(&poolPending)

		// Action item count for the badge
		actionCount := len(pendingAgent) + len(openMatch)

		handlers.RenderTemplate(c, "nba/home.html", gin.H{
			"Sport":          sport.SportNBA,
			"User":           userVal,
			"IsCommish":      isCommish,
			"OwnedTeams":     ownedTeams,
			"PrimaryTeam":    primaryTeam,
			"PrimaryRoster":  primaryRoster,
			"IsAgent":        isAgent,
			"PendingAgent":   pendingAgent,
			"OpenMatch":      openMatch,
			"OpenOutgoing":   openOutgoing,
			"RecentActivity": recentActivity,
			"PoolFAs":        poolFAs,
			"PoolPending":    poolPending,
			"ActionCount":    actionCount,
		})
	}
}
