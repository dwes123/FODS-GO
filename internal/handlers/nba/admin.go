package nba

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/dwes123/fantasy-baseball-go/internal/handlers"
	"github.com/dwes123/fantasy-baseball-go/internal/sport"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	nbastore "github.com/dwes123/fantasy-baseball-go/internal/store/nba"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// adminGate returns true when the user is an admin OR has an NBA commissioner role.
func adminGate(userDB *pgxpool.Pool, user *store.User) bool {
	if user.Role == "admin" {
		return true
	}
	leagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)
	return len(leagues) > 0
}

// userDirEntry hydrates a user_id with display fields. Cross-DB lookup via fantasy_db.
type userDirEntry struct {
	ID       string
	Username string
	Email    string
}

// hydrateUsers takes a list of user_ids and returns a map[id]userDirEntry by querying
// fantasy_db.users in a single SELECT. Missing IDs (deleted users) are simply absent.
func hydrateUsers(userDB *pgxpool.Pool, ids []string) map[string]userDirEntry {
	out := map[string]userDirEntry{}
	if len(ids) == 0 {
		return out
	}
	// Build a Postgres array literal for the IN ANY clause
	rows, err := userDB.Query(context.Background(),
		`SELECT id::TEXT, username, email FROM users WHERE id::TEXT = ANY($1)`,
		ids)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var e userDirEntry
		if err := rows.Scan(&e.ID, &e.Username, &e.Email); err == nil {
			out[e.ID] = e
		}
	}
	return out
}

// AdminNBARolesHandler renders the admin page listing all NBA-side role assignments
// (team owners, agency members) and forms to add/remove.
func AdminNBARolesHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if !adminGate(userDB, user) {
			c.String(http.StatusForbidden, "admin or NBA commissioner only")
			return
		}

		teams, _ := nbastore.ListTeams(nbaDB)
		agencies, _ := nbastore.ListAgencies(nbaDB)
		teamRows, _ := nbastore.ListNBATeamOwnerships(nbaDB)
		agencyRows, _ := nbastore.ListAgencyMemberships(nbaDB)

		// Hydrate user details
		idSet := map[string]bool{}
		for _, r := range teamRows {
			idSet[r.UserID] = true
		}
		for _, r := range agencyRows {
			idSet[r.UserID] = true
		}
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		userMap := hydrateUsers(userDB, ids)

		// Build a per-team and per-agency view: { ScopeName, ScopeID, [ {UserID, Username, Email, IsPrimary} ] }
		type member struct {
			UserID    string
			Username  string
			Email     string
			IsPrimary bool
		}
		type group struct {
			ID      string
			Name    string
			Members []member
		}

		teamGroups := make([]group, 0, len(teams))
		teamIdx := map[string]int{}
		for _, t := range teams {
			teamIdx[t.ID] = len(teamGroups)
			teamGroups = append(teamGroups, group{ID: t.ID, Name: t.Name})
		}
		for _, r := range teamRows {
			i, ok := teamIdx[r.ScopeID]
			if !ok {
				continue
			}
			u := userMap[r.UserID]
			teamGroups[i].Members = append(teamGroups[i].Members, member{
				UserID: r.UserID, Username: u.Username, Email: u.Email, IsPrimary: r.IsPrimary,
			})
		}

		agencyGroups := make([]group, 0, len(agencies))
		agencyIdx := map[string]int{}
		for _, a := range agencies {
			agencyIdx[a.ID] = len(agencyGroups)
			agencyGroups = append(agencyGroups, group{ID: a.ID, Name: a.Name})
		}
		for _, r := range agencyRows {
			i, ok := agencyIdx[r.ScopeID]
			if !ok {
				continue
			}
			u := userMap[r.UserID]
			agencyGroups[i].Members = append(agencyGroups[i].Members, member{
				UserID: r.UserID, Username: u.Username, Email: u.Email, IsPrimary: r.IsPrimary,
			})
		}

		handlers.RenderTemplate(c, "nba/admin_roles.html", gin.H{
			"Sport":        sport.SportNBA,
			"User":         user,
			"IsCommish":    true,
			"Teams":        teams,
			"Agencies":     agencies,
			"TeamGroups":   teamGroups,
			"AgencyGroups": agencyGroups,
		})
	}
}

// resolveUserIdentifier finds a user_id given an email-or-username string.
func resolveUserIdentifier(userDB *pgxpool.Pool, identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", http.ErrAbortHandler
	}
	u, err := store.GetUserByEmailOrUsername(userDB, identifier)
	if err != nil {
		return "", err
	}
	if u == nil {
		return "", nil
	}
	return u.ID, nil
}

// AdminAddNBATeamOwnerHandler — POST /admin/nba-roles/team-owner/add
func AdminAddNBATeamOwnerHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if !adminGate(userDB, user) {
			c.String(http.StatusForbidden, "admin or NBA commissioner only")
			return
		}
		teamID := strings.TrimSpace(c.PostForm("team_id"))
		identifier := strings.TrimSpace(c.PostForm("identifier"))
		if teamID == "" || identifier == "" {
			c.String(http.StatusBadRequest, "team_id and identifier required")
			return
		}
		uid, err := resolveUserIdentifier(userDB, identifier)
		if err != nil || uid == "" {
			c.String(http.StatusBadRequest, "user not found")
			return
		}
		if err := nbastore.AddTeamOwner(nbaDB, teamID, uid, false); err != nil {
			c.String(http.StatusInternalServerError, "add failed: "+err.Error())
			return
		}
		c.Redirect(http.StatusSeeOther, "/admin/nba-roles")
	}
}

// AdminRemoveNBATeamOwnerHandler — POST /admin/nba-roles/team-owner/remove
func AdminRemoveNBATeamOwnerHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if !adminGate(userDB, user) {
			c.String(http.StatusForbidden, "admin or NBA commissioner only")
			return
		}
		if err := nbastore.RemoveTeamOwner(nbaDB,
			c.PostForm("team_id"), c.PostForm("user_id")); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Redirect(http.StatusSeeOther, "/admin/nba-roles")
	}
}

// AdminAddAgencyMemberHandler — POST /admin/nba-roles/agency/add
func AdminAddAgencyMemberHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if !adminGate(userDB, user) {
			c.String(http.StatusForbidden, "admin or NBA commissioner only")
			return
		}
		agencyID := strings.TrimSpace(c.PostForm("agency_id"))
		identifier := strings.TrimSpace(c.PostForm("identifier"))
		if agencyID == "" || identifier == "" {
			c.String(http.StatusBadRequest, "agency_id and identifier required")
			return
		}
		uid, err := resolveUserIdentifier(userDB, identifier)
		if err != nil || uid == "" {
			c.String(http.StatusBadRequest, "user not found")
			return
		}
		if err := nbastore.AddAgencyMember(nbaDB, agencyID, uid, false); err != nil {
			c.String(http.StatusInternalServerError, "add failed: "+err.Error())
			return
		}
		c.Redirect(http.StatusSeeOther, "/admin/nba-roles")
	}
}

// AdminRemoveAgencyMemberHandler — POST /admin/nba-roles/agency/remove
func AdminRemoveAgencyMemberHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if !adminGate(userDB, user) {
			c.String(http.StatusForbidden, "admin or NBA commissioner only")
			return
		}
		if err := nbastore.RemoveAgencyMember(nbaDB,
			c.PostForm("agency_id"), c.PostForm("user_id")); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Redirect(http.StatusSeeOther, "/admin/nba-roles")
	}
}
