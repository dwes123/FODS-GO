package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
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
			fmt.Printf("ERROR [ProcessAction]: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
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
		var bidInfo store.PlayerBidInfo
		searchResults := []store.RosterPlayer{}
		if playerID != "" {
			player, _ = store.GetPlayerByID(db, playerID)
			if player != nil {
				// Load contract_option_years JSONB (not fetched by GetPlayerByID)
				var optRaw []byte
				err := db.QueryRow(context.Background(),
					`SELECT COALESCE(contract_option_years, '[]'::jsonb) FROM players WHERE id = $1`, playerID).Scan(&optRaw)
				if err == nil && len(optRaw) > 0 {
					player.ContractOptionYears = make(map[int]bool)
					var optYears []int
					json.Unmarshal(optRaw, &optYears)
					for _, y := range optYears {
						player.ContractOptionYears[y] = true
					}
				}
			}
			bidInfo = store.GetPlayerBidInfo(db, playerID)
		} else if searchQuery != "" {
			searchResults, _ = store.SearchAllPlayers(db, searchQuery)
		}
		leagues, _ := store.GetLeaguesWithTeams(db)
		RenderTemplate(c, "admin_player_editor.html", gin.H{
			"User":          user,
			"Player":        player,
			"BidInfo":       bidInfo,
			"SearchResults": searchResults,
			"SearchQuery":   searchQuery,
			"Leagues":       leagues,
			"SaveSuccess":   c.Query("saved") == "1",
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
		// Collect team option year checkboxes
		var contractOptionYears []int
		for y := 2026; y <= 2040; y++ {
			if c.PostForm(fmt.Sprintf("option_year_%d", y)) == "on" {
				contractOptionYears = append(contractOptionYears, y)
			}
		}

		bidAmt, _ := strconv.ParseFloat(c.PostForm("pending_bid_amount"), 64)
		bidYrs, _ := strconv.Atoi(c.PostForm("pending_bid_years"))
		bidAAV, _ := strconv.ParseFloat(c.PostForm("pending_bid_aav"), 64)
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
			IsIFA:       c.PostForm("is_ifa") == "on",
			DFAOnly:     c.PostForm("dfa_only") == "on",
			Contracts:   contracts,
			FaStatus:       c.PostForm("fa_status"),
			PendingBidAmt:  bidAmt,
			PendingBidYrs:  bidYrs,
			PendingBidAAV:  bidAAV,
			PendingBidTeam: c.PostForm("pending_bid_team_id"),
			BidType:            c.PostForm("bid_type"),
			ContractOptionYears: contractOptionYears,
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
			fmt.Printf("ERROR [AdminSavePlayer]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}
		c.Redirect(http.StatusFound, "/admin/player-editor?player_id=" + update.ID + "&saved=1")
	}
}

// --- Account Approval Queue (Feature 9) ---

func AdminApprovalsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}
		reqs, _ := store.GetPendingRegistrations(db)
		RenderTemplate(c, "admin_approvals.html", gin.H{
			"User":      user,
			"Requests":  reqs,
			"IsCommish": true,
		})
	}
}

func AdminProcessRegistrationHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		requestID := c.PostForm("request_id")
		action := c.PostForm("action")

		var err error
		if action == "approve" {
			err = store.ApproveRegistration(db, requestID, user.ID)
		} else {
			err = store.DenyRegistration(db, requestID, user.ID)
		}

		if err != nil {
			fmt.Printf("ERROR [ProcessRegistration]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		c.Redirect(http.StatusFound, "/admin/approvals")
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
			fmt.Printf("ERROR [AdminSaveDeadCap]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
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

// --- Commissioner Role Management ---

type leagueRole struct {
	ID         string
	UserID     string
	Username   string
	Email      string
	LeagueID   string
	LeagueName string
}

func AdminRolesHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if user.Role != "admin" {
			c.String(http.StatusForbidden, "Global Admin Only")
			return
		}

		ctx := context.Background()

		// Get current commissioner roles with user/league names
		rows, err := db.Query(ctx, `
			SELECT lr.id, lr.user_id, u.username, u.email, lr.league_id, l.name
			FROM league_roles lr
			JOIN users u ON u.id = lr.user_id
			JOIN leagues l ON l.id = lr.league_id
			ORDER BY l.name, u.username
		`)
		var roles []leagueRole
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var r leagueRole
				if err := rows.Scan(&r.ID, &r.UserID, &r.Username, &r.Email, &r.LeagueID, &r.LeagueName); err == nil {
					roles = append(roles, r)
				}
			}
		} else {
			fmt.Printf("ERROR [AdminRolesHandler]: %v\n", err)
		}

		// Get all users for dropdown
		userRows, err := db.Query(ctx, `SELECT id, username, email, role FROM users ORDER BY username`)
		var allUsers []store.User
		if err == nil {
			defer userRows.Close()
			for userRows.Next() {
				var u store.User
				if err := userRows.Scan(&u.ID, &u.Username, &u.Email, &u.Role); err == nil {
					allUsers = append(allUsers, u)
				}
			}
		}

		leagues, _ := store.GetLeaguesWithTeams(db)

		RenderTemplate(c, "admin_roles.html", gin.H{
			"User":        user,
			"Roles":       roles,
			"AllUsers":    allUsers,
			"Leagues":     leagues,
			"SaveSuccess": c.Query("saved") == "1",
			"IsCommish":   true,
		})
	}
}

func AdminAddRoleHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if user.Role != "admin" {
			c.String(http.StatusForbidden, "Global Admin Only")
			return
		}

		ctx := context.Background()
		userID := c.PostForm("user_id")
		leagueID := c.PostForm("league_id")

		if userID != "" && leagueID != "" {
			_, err := db.Exec(ctx, `
				INSERT INTO league_roles (user_id, league_id, role)
				VALUES ($1, $2, 'commissioner')
				ON CONFLICT (user_id, league_id) DO NOTHING
			`, userID, leagueID)
			if err != nil {
				fmt.Printf("ERROR [AdminAddRole]: %v\n", err)
			}
		}

		// Optionally update global user role
		roleUserID := c.PostForm("user_role_id")
		roleValue := c.PostForm("user_role_value")
		if roleUserID != "" && (roleValue == "admin" || roleValue == "user") {
			_, err := db.Exec(ctx, `UPDATE users SET role = $1 WHERE id = $2`, roleValue, roleUserID)
			if err != nil {
				fmt.Printf("ERROR [AdminAddRole-UpdateRole]: %v\n", err)
			}
		}

		c.Redirect(http.StatusFound, "/admin/roles?saved=1")
	}
}

func AdminDeleteRoleHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if user.Role != "admin" {
			c.String(http.StatusForbidden, "Global Admin Only")
			return
		}

		roleID := c.PostForm("role_id")
		if roleID != "" {
			_, err := db.Exec(context.Background(), `DELETE FROM league_roles WHERE id = $1`, roleID)
			if err != nil {
				fmt.Printf("ERROR [AdminDeleteRole]: %v\n", err)
			}
		}

		c.Redirect(http.StatusFound, "/admin/roles")
	}
}

// --- ISBP / MiLB Balance Editor ---

func AdminBalanceEditorHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		leagueID := c.Query("league_id")
		teams, err := store.GetTeamsWithBalances(db, leagueID)
		if err != nil {
			fmt.Printf("ERROR [AdminBalanceEditor]: %v\n", err)
		}

		leagues, _ := store.GetLeaguesWithTeams(db)

		RenderTemplate(c, "admin_balance_editor.html", gin.H{
			"User":        user,
			"Teams":       teams,
			"Leagues":     leagues,
			"LeagueID":    leagueID,
			"SaveSuccess": c.Query("saved") == "1",
			"IsCommish":   true,
		})
	}
}

func AdminSaveBalanceHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		teamID := c.PostForm("team_id")
		isbpBalance, _ := strconv.ParseFloat(c.PostForm("isbp_balance"), 64)
		milbBalance, _ := strconv.ParseFloat(c.PostForm("milb_balance"), 64)

		err := store.SetTeamBalance(db, teamID, isbpBalance, milbBalance)
		if err != nil {
			fmt.Printf("ERROR [AdminSaveBalance]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		leagueID := c.PostForm("league_id")
		redirect := "/admin/balance-editor?saved=1"
		if leagueID != "" {
			redirect += "&league_id=" + leagueID
		}
		c.Redirect(http.StatusFound, redirect)
	}
}

// --- League Settings (Feature 16) ---

func AdminSettingsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		yearStr := c.Query("year")
		year, err := strconv.Atoi(yearStr)
		if err != nil {
			year = time.Now().Year()
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		dates, _ := store.GetLeagueDates(db, year)

		// Build a lookup map: "leagueID_dateType" -> "event_date"
		dateMap := make(map[string]string)
		for _, d := range dates {
			key := fmt.Sprintf("%s_%s", d.LeagueID, d.DateType)
			dateMap[key] = d.EventDate
		}

		// Build league settings map: "leagueID_field" -> value
		settingsMap := make(map[string]int)
		for _, l := range leagues {
			s := store.GetLeagueSettings(db, l.ID, year)
			settingsMap[l.ID+"_roster_26_man_limit"] = s.Roster26ManLimit
			settingsMap[l.ID+"_roster_40_man_limit"] = s.Roster40ManLimit
			settingsMap[l.ID+"_sp_26_man_limit"] = s.SP26ManLimit
		}

		// Load Slack integration settings
		slackMap := make(map[string]string)
		for _, l := range leagues {
			var token, chTx, chTrades, chAlerts, chTB string
			db.QueryRow(context.Background(), `
				SELECT COALESCE(slack_bot_token, ''),
					COALESCE(slack_channel_transactions, ''),
					COALESCE(slack_channel_completed_trades, ''),
					COALESCE(slack_channel_stat_alerts, ''),
					COALESCE(slack_channel_trade_block, '')
				FROM league_integrations WHERE league_id = $1
			`, l.ID).Scan(&token, &chTx, &chTrades, &chAlerts, &chTB)
			slackMap[l.ID+"_bot_token"] = token
			slackMap[l.ID+"_channel_transactions"] = chTx
			slackMap[l.ID+"_channel_completed_trades"] = chTrades
			slackMap[l.ID+"_channel_stat_alerts"] = chAlerts
			slackMap[l.ID+"_channel_trade_block"] = chTB
		}

		RenderTemplate(c, "admin_settings.html", gin.H{
			"User":        user,
			"Leagues":     leagues,
			"Year":        year,
			"DateMap":     dateMap,
			"SettingsMap": settingsMap,
			"SlackMap":    slackMap,
			"SaveSuccess": c.Query("saved") == "1",
			"IsCommish":   true,
		})
	}
}

func AdminSaveSettingsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		year, _ := strconv.Atoi(c.PostForm("year"))
		leagues, _ := store.GetLeaguesWithTeams(db)

		dateTypes := []string{
			"trade_deadline", "opening_day", "extension_deadline",
			"ifa_window_open", "ifa_window_close",
			"milb_fa_window_open", "milb_fa_window_close",
			"option_deadline",
			"roster_expansion_start", "roster_expansion_end",
		}

		for _, l := range leagues {
			for _, dt := range dateTypes {
				val := c.PostForm(dt + "_" + l.ID)
				if val != "" {
					store.UpsertLeagueDate(db, l.ID, year, dt, val)
				}
			}

			// Save numeric roster settings
			limit26, _ := strconv.Atoi(c.PostForm("roster_26_man_limit_" + l.ID))
			limit40, _ := strconv.Atoi(c.PostForm("roster_40_man_limit_" + l.ID))
			spLimit, _ := strconv.Atoi(c.PostForm("sp_26_man_limit_" + l.ID))
			if limit26 == 0 { limit26 = 26 }
			if limit40 == 0 { limit40 = 40 }
			if spLimit == 0 { spLimit = 6 }
			store.UpsertLeagueSettings(db, l.ID, year, limit26, limit40, spLimit)
		}

		// Save Slack integration settings
		for _, l := range leagues {
			token := c.PostForm("slack_bot_token_" + l.ID)
			chTx := c.PostForm("slack_channel_transactions_" + l.ID)
			chTrades := c.PostForm("slack_channel_completed_trades_" + l.ID)
			chAlerts := c.PostForm("slack_channel_stat_alerts_" + l.ID)
			chTB := c.PostForm("slack_channel_trade_block_" + l.ID)

			db.Exec(context.Background(), `
				INSERT INTO league_integrations (league_id, slack_bot_token, slack_channel_transactions, slack_channel_completed_trades, slack_channel_stat_alerts, slack_channel_trade_block)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (league_id) DO UPDATE SET
					slack_bot_token = $2,
					slack_channel_transactions = $3,
					slack_channel_completed_trades = $4,
					slack_channel_stat_alerts = $5,
					slack_channel_trade_block = $6
			`, l.ID, token, chTx, chTrades, chAlerts, chTB)
		}

		c.Redirect(http.StatusFound, fmt.Sprintf("/admin/settings?year=%d&saved=1", year))
	}
}

// --- Team & User Management ---

var allLeagueIDs = []string{
	"11111111-1111-1111-1111-111111111111",
	"22222222-2222-2222-2222-222222222222",
	"33333333-3333-3333-3333-333333333333",
	"44444444-4444-4444-4444-444444444444",
}

func AdminTeamOwnersHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		leagueIDs := adminLeagues
		if user.Role == "admin" {
			leagueIDs = allLeagueIDs
		}

		owners, err := store.GetAllTeamOwners(db, leagueIDs)
		if err != nil {
			fmt.Printf("ERROR [AdminTeamOwners]: %v\n", err)
		}

		allUsers, err := store.GetAllUsers(db)
		if err != nil {
			fmt.Printf("ERROR [AdminTeamOwners-Users]: %v\n", err)
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		// Filter leagues for commissioners
		if user.Role != "admin" {
			var filtered []store.League
			leagueSet := make(map[string]bool)
			for _, id := range adminLeagues {
				leagueSet[id] = true
			}
			for _, l := range leagues {
				if leagueSet[l.ID] {
					filtered = append(filtered, l)
				}
			}
			leagues = filtered
		}

		RenderTemplate(c, "admin_team_owners.html", gin.H{
			"User":        user,
			"Owners":      owners,
			"AllUsers":    allUsers,
			"Leagues":     leagues,
			"SaveSuccess": c.Query("saved") == "1",
			"IsCommish":   true,
		})
	}
}

func AdminAddTeamOwnerHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		teamID := c.PostForm("team_id")
		userID := c.PostForm("user_id")

		// Verify team is in commissioner's leagues
		if user.Role != "admin" {
			teamLeague, err := store.GetTeamLeagueID(db, teamID)
			if err != nil {
				c.String(http.StatusBadRequest, "Invalid team")
				return
			}
			allowed := false
			for _, l := range adminLeagues {
				if l == teamLeague {
					allowed = true
					break
				}
			}
			if !allowed {
				c.String(http.StatusForbidden, "Not authorized for this league")
				return
			}
		}

		err := store.AddTeamOwner(db, teamID, userID)
		if err != nil {
			fmt.Printf("ERROR [AdminAddTeamOwner]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		c.Redirect(http.StatusFound, "/admin/team-owners?saved=1")
	}
}

func AdminRemoveTeamOwnerHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		teamID := c.PostForm("team_id")
		userID := c.PostForm("user_id")

		// Verify team is in commissioner's leagues
		if user.Role != "admin" {
			teamLeague, err := store.GetTeamLeagueID(db, teamID)
			if err != nil {
				c.String(http.StatusBadRequest, "Invalid team")
				return
			}
			allowed := false
			for _, l := range adminLeagues {
				if l == teamLeague {
					allowed = true
					break
				}
			}
			if !allowed {
				c.String(http.StatusForbidden, "Not authorized for this league")
				return
			}
		}

		err := store.RemoveTeamOwner(db, teamID, userID)
		if err != nil {
			fmt.Printf("ERROR [AdminRemoveTeamOwner]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		c.Redirect(http.StatusFound, "/admin/team-owners?saved=1")
	}
}

func AdminCreateUserHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		username := c.PostForm("username")
		email := c.PostForm("email")
		password := c.PostForm("password")

		if username == "" || email == "" || password == "" {
			c.String(http.StatusBadRequest, "All fields are required")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			fmt.Printf("ERROR [AdminCreateUser]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		_, err = store.CreateUser(db, username, email, string(hash))
		if err != nil {
			fmt.Printf("ERROR [AdminCreateUser]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		c.Redirect(http.StatusFound, "/admin/team-owners?saved=1")
	}
}

func AdminDeleteUserHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		userID := c.PostForm("user_id")

		// Prevent deleting yourself
		if userID == user.ID {
			c.String(http.StatusBadRequest, "Cannot delete your own account")
			return
		}

		err := store.DeleteUser(db, userID)
		if err != nil {
			fmt.Printf("ERROR [AdminDeleteUser]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		c.Redirect(http.StatusFound, "/admin/team-owners?saved=1")
	}
}
