package nba

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dwes123/fantasy-baseball-go/internal/handlers"
	"github.com/dwes123/fantasy-baseball-go/internal/sport"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	nbastore "github.com/dwes123/fantasy-baseball-go/internal/store/nba"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// =====================
// Auth helpers
// =====================

// userIsAgentFor returns true if the user is a member of the player's agency.
func userIsAgentFor(nbaDB *pgxpool.Pool, userID, agencyID string) bool {
	if agencyID == "" {
		return false
	}
	agencies, _ := nbastore.AgenciesForUser(nbaDB, userID)
	for _, a := range agencies {
		if a == agencyID {
			return true
		}
	}
	return false
}

// userOwnsTeam returns true if the user is a row in team_owners for that team.
func userOwnsTeam(nbaDB *pgxpool.Pool, userID, teamID string) bool {
	if teamID == "" {
		return false
	}
	owns, _ := nbastore.IsTeamOwner(nbaDB, teamID, userID)
	return owns
}

// =====================
// New offer form
// =====================

// NewOfferFormHandler renders the offer-submission form, gated to GMs of any team
// (they pick which of their teams the offer comes from if they own multiple).
func NewOfferFormHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		playerID := c.Query("player")
		if playerID == "" {
			c.String(http.StatusBadRequest, "player query param required")
			return
		}
		player, err := nbastore.GetPlayerByID(nbaDB, playerID)
		if err != nil {
			c.String(http.StatusNotFound, "player not found")
			return
		}
		ownedTeams, _ := nbastore.GetManagedNBATeams(nbaDB, user.ID)
		if len(ownedTeams) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "must own a team to submit an offer")
			return
		}
		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)
		handlers.RenderTemplate(c, "nba/offer_form.html", gin.H{
			"Sport":        sport.SportNBA,
			"User":         user,
			"IsCommish":    len(adminLeagues) > 0 || user.Role == "admin",
			"Player":       player,
			"OwnedTeams":   ownedTeams,
		})
	}
}

// =====================
// Submit / state actions
// =====================

func parseFloat(s string) float64 { v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return v }
func parseInt(s string) int       { v, _ := strconv.Atoi(strings.TrimSpace(s)); return v }

// SubmitOfferHandler handles POST /nba/offers/submit
func SubmitOfferHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		playerID := strings.TrimSpace(c.PostForm("player_id"))
		teamID := strings.TrimSpace(c.PostForm("offering_team_id"))
		years := parseInt(c.PostForm("years"))
		startSal := parseFloat(c.PostForm("starting_salary"))
		raise := parseFloat(c.PostForm("raise_pct"))
		exception := strings.TrimSpace(c.PostForm("exception_used"))
		notes := strings.TrimSpace(c.PostForm("notes"))

		if user.Role != "admin" && !userOwnsTeam(nbaDB, user.ID, teamID) {
			c.String(http.StatusForbidden, "you do not own that team")
			return
		}
		if years < 1 || years > 5 || startSal <= 0 || playerID == "" || teamID == "" || exception == "" {
			c.String(http.StatusBadRequest, "invalid offer terms")
			return
		}

		offer, err := nbastore.SubmitOffer(nbaDB, nbastore.CreateOfferParams{
			PlayerID:       playerID,
			OfferingTeamID: teamID,
			SubmittedBy:    user.ID,
			Years:          years,
			StartingSalary: startSal,
			RaisePct:       raise,
			ExceptionUsed:  exception,
			Notes:          notes,
		})
		if err != nil {
			fmt.Printf("ERROR [SubmitOffer]: %v\n", err)
			c.String(http.StatusInternalServerError, "failed to submit offer")
			return
		}
		c.Redirect(http.StatusSeeOther, "/nba/offers/"+offer.ID)
	}
}

// genericOfferAction wraps the bookkeeping for the simpler state transitions
// (accept/reject/withdraw/match/walk). action is one of the AgentXxx / WalkOffer / etc.
type offerActionFn func(db *pgxpool.Pool, offerID, userID, notes string) error

func doOfferAction(c *gin.Context, nbaDB *pgxpool.Pool, fn offerActionFn, requireAgent, requireBirdTeam, requireOffering bool) {
	user := c.MustGet("user").(*store.User)
	offerID := c.Param("id")
	notes := strings.TrimSpace(c.PostForm("notes"))

	offer, err := nbastore.GetOffer(nbaDB, offerID)
	if err != nil {
		c.String(http.StatusNotFound, "offer not found")
		return
	}

	authorized := user.Role == "admin"
	if requireAgent && offer.AgencyID != nil && userIsAgentFor(nbaDB, user.ID, *offer.AgencyID) {
		authorized = true
	}
	if requireBirdTeam && offer.BirdRightsTeamID != nil && userOwnsTeam(nbaDB, user.ID, *offer.BirdRightsTeamID) {
		authorized = true
	}
	if requireOffering && userOwnsTeam(nbaDB, user.ID, offer.OfferingTeamID) {
		authorized = true
	}
	if !authorized {
		c.String(http.StatusForbidden, "not authorized for this action")
		return
	}

	if err := fn(nbaDB, offerID, user.ID, notes); err != nil {
		fmt.Printf("ERROR [offer action]: %v\n", err)
		c.String(http.StatusInternalServerError, "action failed: "+err.Error())
		return
	}
	c.Redirect(http.StatusSeeOther, "/nba/offers/"+offerID)
}

func AgentAcceptHandler(_, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) { doOfferAction(c, nbaDB, nbastore.AgentAccept, true, false, false) }
}
func AgentRejectHandler(_, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) { doOfferAction(c, nbaDB, nbastore.AgentReject, true, false, false) }
}
func MatchHandler(_, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) { doOfferAction(c, nbaDB, nbastore.MatchOffer, false, true, false) }
}
func WalkHandler(_, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) { doOfferAction(c, nbaDB, nbastore.WalkOffer, false, true, false) }
}
func WithdrawHandler(_, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) { doOfferAction(c, nbaDB, nbastore.WithdrawOffer, false, false, true) }
}
func TeamAcceptCounterHandler(_, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) { doOfferAction(c, nbaDB, nbastore.TeamAcceptCounter, false, false, true) }
}
func TeamRejectCounterHandler(_, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) { doOfferAction(c, nbaDB, nbastore.TeamRejectCounter, false, false, true) }
}

// AgentCounterHandler accepts new terms via form fields.
func AgentCounterHandler(_, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		parentID := c.Param("id")
		offer, err := nbastore.GetOffer(nbaDB, parentID)
		if err != nil {
			c.String(http.StatusNotFound, "offer not found")
			return
		}
		authorized := user.Role == "admin" ||
			(offer.AgencyID != nil && userIsAgentFor(nbaDB, user.ID, *offer.AgencyID))
		if !authorized {
			c.String(http.StatusForbidden, "only the player's agent can counter")
			return
		}
		years := parseInt(c.PostForm("years"))
		startSal := parseFloat(c.PostForm("starting_salary"))
		raise := parseFloat(c.PostForm("raise_pct"))
		exception := strings.TrimSpace(c.PostForm("exception_used"))
		notes := strings.TrimSpace(c.PostForm("notes"))
		if years < 1 || years > 5 || startSal <= 0 || exception == "" {
			c.String(http.StatusBadRequest, "invalid counter terms")
			return
		}
		child, err := nbastore.AgentCounter(nbaDB, parentID, user.ID, years, startSal, raise, exception, notes)
		if err != nil {
			c.String(http.StatusInternalServerError, "counter failed: "+err.Error())
			return
		}
		c.Redirect(http.StatusSeeOther, "/nba/offers/"+child.ID)
	}
}

// =====================
// Read pages
// =====================

func OfferDetailHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		offerID := c.Param("id")
		offer, err := nbastore.GetOffer(nbaDB, offerID)
		if err != nil {
			c.String(http.StatusNotFound, "offer not found")
			return
		}
		events, _ := nbastore.ListEventsForOffer(nbaDB, offerID)
		var parent *nbastore.Offer
		if offer.ParentOfferID != nil {
			parent, _ = nbastore.GetOffer(nbaDB, *offer.ParentOfferID)
		}

		canAct := user.Role == "admin"
		canAgent := offer.AgencyID != nil && userIsAgentFor(nbaDB, user.ID, *offer.AgencyID)
		canBirdTeam := offer.BirdRightsTeamID != nil && userOwnsTeam(nbaDB, user.ID, *offer.BirdRightsTeamID)
		canOffering := userOwnsTeam(nbaDB, user.ID, offer.OfferingTeamID)
		if canAgent || canBirdTeam || canOffering {
			canAct = true
		}

		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)
		handlers.RenderTemplate(c, "nba/offer_detail.html", gin.H{
			"Sport":       sport.SportNBA,
			"User":        user,
			"IsCommish":   len(adminLeagues) > 0 || user.Role == "admin",
			"Offer":       offer,
			"Parent":      parent,
			"Events":      events,
			"CanAct":      canAct,
			"CanAgent":    canAgent,
			"CanBirdTeam": canBirdTeam,
			"CanOffering": canOffering,
		})
	}
}

func AgentDashboardHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		agencyIDs, _ := nbastore.AgenciesForUser(nbaDB, user.ID)
		if len(agencyIDs) == 0 && user.Role != "admin" {
			handlers.RenderTemplate(c, "nba/agent_dashboard.html", gin.H{
				"Sport": sport.SportNBA,
				"User":  user,
				"NotAgent": true,
			})
			return
		}

		// Aggregate offers across all agencies the user is a member of (most users have 1)
		var allOffers []*nbastore.Offer
		for _, aid := range agencyIDs {
			off, _ := nbastore.ListPendingForAgency(nbaDB, aid)
			allOffers = append(allOffers, off...)
		}
		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)
		handlers.RenderTemplate(c, "nba/agent_dashboard.html", gin.H{
			"Sport":      sport.SportNBA,
			"User":       user,
			"IsCommish":  len(adminLeagues) > 0 || user.Role == "admin",
			"NotAgent":   false,
			"Offers":     allOffers,
			"AgencyIDs":  agencyIDs,
		})
	}
}

func MatchPendingHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		ownedTeams, _ := nbastore.GetManagedNBATeams(nbaDB, user.ID)

		var allOffers []*nbastore.Offer
		for _, t := range ownedTeams {
			off, _ := nbastore.ListAwaitingMatchForTeam(nbaDB, t.ID)
			allOffers = append(allOffers, off...)
		}
		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)
		handlers.RenderTemplate(c, "nba/match_pending.html", gin.H{
			"Sport":     sport.SportNBA,
			"User":      user,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
			"Offers":    allOffers,
		})
	}
}

func MyOffersHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		ownedTeams, _ := nbastore.GetManagedNBATeams(nbaDB, user.ID)

		var allOffers []*nbastore.Offer
		for _, t := range ownedTeams {
			off, _ := nbastore.ListOutgoingForTeam(nbaDB, t.ID, 50)
			allOffers = append(allOffers, off...)
		}
		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)
		handlers.RenderTemplate(c, "nba/my_offers.html", gin.H{
			"Sport":     sport.SportNBA,
			"User":      user,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
			"Offers":    allOffers,
		})
	}
}
