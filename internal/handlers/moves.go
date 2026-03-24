package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/notification"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MoveRequest struct {
	PlayerID string `json:"player_id"`
	TeamID   string `json:"team_id"`
}

type ILMoveRequest struct {
	MoveRequest
	Duration string `json:"duration"` // "10", "15", "60"
}

type TradeBlockRequest struct {
	MoveRequest
	OnBlock bool   `json:"on_block"`
	Notes   string `json:"notes"`
}

// assignRookieContractIfEmpty checks if a player has no contract for the current year
// and assigns the default rookie contract: $760,000 current year, TC, TC, ARB 1, ARB 2, ARB 3
func assignRookieContractIfEmpty(db *pgxpool.Pool, playerID string) {
	ctx := context.Background()
	year := time.Now().Year()
	contractCol := fmt.Sprintf("contract_%d", year)

	var currentContract *string
	db.QueryRow(ctx, fmt.Sprintf("SELECT %s FROM players WHERE id = $1", contractCol), playerID).Scan(&currentContract)

	if currentContract != nil && *currentContract != "" {
		return
	}

	// Assign rookie contract: current year $760K, then TC, TC, ARB 1, ARB 2, ARB 3
	values := []struct {
		offset int
		value  string
	}{
		{0, "760000"},
		{1, "TC"},
		{2, "TC"},
		{3, "ARB 1"},
		{4, "ARB 2"},
		{5, "ARB 3"},
	}

	for _, v := range values {
		col := fmt.Sprintf("contract_%d", year+v.offset)
		if year+v.offset > 2040 {
			break
		}
		db.Exec(ctx, fmt.Sprintf("UPDATE players SET %s = $1 WHERE id = $2", col), v.value, playerID)
	}
}

func getPlayerAndTeamName(db *pgxpool.Pool, playerID, teamID string) (playerName, teamName, leagueID string) {
	db.QueryRow(context.Background(),
		"SELECT first_name || ' ' || last_name FROM players WHERE id = $1", playerID).Scan(&playerName)
	db.QueryRow(context.Background(),
		"SELECT name, league_id FROM teams WHERE id = $1", teamID).Scan(&teamName, &leagueID)
	return
}

func PromoteTo40ManHandler(db *pgxpool.Pool) gin.HandlerFunc {
        return func(c *gin.Context) {
                var req MoveRequest
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

		leagueID, _ := store.GetTeamLeagueID(db, req.TeamID)
		settings := store.GetLeagueSettings(db, leagueID, time.Now().Year())
		limit40 := settings.Roster40ManLimit

		_, count40, err := store.GetTeamRosterCounts(db, req.TeamID)
		if err == nil && count40 >= limit40 {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("40-Man Roster is full (%d/%d). You must DFA a player first.", count40, limit40)})
			return
		}

                _, err = db.Exec(context.Background(),
                        `UPDATE players SET status_40_man = TRUE WHERE id = $1 AND team_id = $2`,
                        req.PlayerID, req.TeamID)

                if err != nil {
                        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
                        return
                }

                // Auto-assign rookie contract if player has no contract data
                assignRookieContractIfEmpty(db, req.PlayerID)

                store.AppendRosterMove(db, req.PlayerID, req.TeamID, "Promoted to 40-Man")
                pName, tName, lID := getPlayerAndTeamName(db, req.PlayerID, req.TeamID)
                store.LogActivity(db, lID, req.TeamID, "Roster Move", fmt.Sprintf("%s promoted %s to 40-Man roster", tName, pName))
                c.JSON(http.StatusOK, gin.H{"message": "Promoted to 40-man"})
        }
}

func PromoteTo26ManHandler(db *pgxpool.Pool) gin.HandlerFunc {
        return func(c *gin.Context) {
                var req MoveRequest
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

		leagueID, _ := store.GetTeamLeagueID(db, req.TeamID)
		settings := store.GetLeagueSettings(db, leagueID, time.Now().Year())
		limit26 := settings.Roster26ManLimit
		limit40 := settings.Roster40ManLimit

		count26, count40, err := store.GetTeamRosterCounts(db, req.TeamID)
		if err == nil {
			if count26 >= limit26 {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("26-Man Roster is full (%d/%d). You must option a player to the minors first.", count26, limit26)})
				return
			}
			// Promotion to 26-man also implies promotion to 40-man if not already there
			var isAlreadyOn40 bool
			db.QueryRow(context.Background(), "SELECT status_40_man FROM players WHERE id = $1", req.PlayerID).Scan(&isAlreadyOn40)
			if !isAlreadyOn40 && count40 >= limit40 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "40-Man Roster is full. Cannot promote to active roster."})
				return
			}
		}

		// SP limit check
		var playerPos string
		db.QueryRow(context.Background(), "SELECT position FROM players WHERE id = $1", req.PlayerID).Scan(&playerPos)
		if playerPos == "SP" {
			spCount, _ := store.GetTeam26ManSPCount(db, req.TeamID)
			if spCount >= settings.SP26ManLimit {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("SP limit reached (%d/%d on 26-man). You must option an SP first.", spCount, settings.SP26ManLimit)})
				return
			}
		}

                _, err = db.Exec(context.Background(),
                        `UPDATE players SET status_26_man = TRUE, status_40_man = TRUE WHERE id = $1 AND team_id = $2`,
                        req.PlayerID, req.TeamID)

                if err != nil {
                        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
                        return
                }

                // Auto-assign rookie contract if player has no contract data
                assignRookieContractIfEmpty(db, req.PlayerID)

                store.AppendRosterMove(db, req.PlayerID, req.TeamID, "Promoted to 26-Man")
                pName, tName, lID := getPlayerAndTeamName(db, req.PlayerID, req.TeamID)
                store.LogActivity(db, lID, req.TeamID, "Roster Move", fmt.Sprintf("%s promoted %s to 26-Man roster", tName, pName))
                c.JSON(http.StatusOK, gin.H{"message": "Promoted to 26-man"})
        }
}
func OptionToMinorsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req MoveRequest
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

		// Check if player is DFA-only (cannot be optioned)
		var dfaOnly bool
		var optionYearsUsed, optionsThisSeason, optionYearLogged int
		err := db.QueryRow(context.Background(),
			`SELECT COALESCE(dfa_only, FALSE), option_years_used, options_this_season, option_year_logged FROM players WHERE id = $1 AND team_id = $2`,
			req.PlayerID, req.TeamID).Scan(&dfaOnly, &optionYearsUsed, &optionsThisSeason, &optionYearLogged)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		if dfaOnly {
			c.JSON(http.StatusBadRequest, gin.H{"error": "DFA-only players cannot be optioned. Use the Waive button instead."})
			return
		}

		currentYear := time.Now().Year()
		isNewOptionYear := optionYearLogged != currentYear

		if isNewOptionYear && optionYearsUsed >= 3 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Player is out of options (3 option years used)"})
			return
		}

		if !isNewOptionYear && optionsThisSeason >= 5 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Player has been optioned 5 times this season (maximum reached)"})
			return
		}

		// Build update: always increment options_this_season, conditionally increment option_years_used
		if isNewOptionYear {
			_, err = db.Exec(context.Background(),
				`UPDATE players SET
					status_26_man = FALSE,
					option_years_used = option_years_used + 1,
					options_this_season = 1,
					option_year_logged = $3
				WHERE id = $1 AND team_id = $2`,
				req.PlayerID, req.TeamID, currentYear)
		} else {
			_, err = db.Exec(context.Background(),
				`UPDATE players SET
					status_26_man = FALSE,
					options_this_season = options_this_season + 1
				WHERE id = $1 AND team_id = $2`,
				req.PlayerID, req.TeamID)
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		store.AppendRosterMove(db, req.PlayerID, req.TeamID, "Optioned to Minors")
		pName, tName, lID := getPlayerAndTeamName(db, req.PlayerID, req.TeamID)
		store.LogActivity(db, lID, req.TeamID, "Roster Move", fmt.Sprintf("%s optioned %s to minors", tName, pName))
		c.JSON(http.StatusOK, gin.H{"message": "Optioned to minors"})
	}
}

func MoveToILHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ILMoveRequest
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

		statusIL := req.Duration + "-Day IL"

		// Validate IL duration based on position
		var position string
		var on26 bool
		err := db.QueryRow(context.Background(),
			`SELECT position, status_26_man FROM players WHERE id = $1 AND team_id = $2`,
			req.PlayerID, req.TeamID).Scan(&position, &on26)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		isPitcher := position == "SP" || position == "RP" || position == "P"
		if req.Duration == "10" && isPitcher {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Pitchers must use the 15-Day or 60-Day IL"})
			return
		}
		if req.Duration == "15" && !isPitcher {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Position players must use the 10-Day or 60-Day IL"})
			return
		}

		// Record pre-IL roster status so we can restore on activation
		preILStatus := "40"
		if on26 {
			preILStatus = "26"
		}

		_, err = db.Exec(context.Background(),
			`UPDATE players SET
				status_il = $1,
				il_start_date = NOW(),
				status_26_man = FALSE,
				pre_il_status = $3
			WHERE id = $2 AND team_id = $4`,
			statusIL, req.PlayerID, preILStatus, req.TeamID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		store.AppendRosterMove(db, req.PlayerID, req.TeamID, "Placed on "+statusIL)
		pName, tName, lID := getPlayerAndTeamName(db, req.PlayerID, req.TeamID)
		store.LogActivity(db, lID, req.TeamID, "Roster Move", fmt.Sprintf("%s placed %s on %s", tName, pName, statusIL))
		c.JSON(http.StatusOK, gin.H{"message": "Player moved to IL"})
	}
}

func ActivateFromILHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req MoveRequest
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

		// Restore player to their pre-IL roster position
		var preILStatus *string
		err := db.QueryRow(context.Background(),
			`SELECT pre_il_status FROM players WHERE id = $1 AND team_id = $2`,
			req.PlayerID, req.TeamID).Scan(&preILStatus)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		restore26 := false
		restore40 := true
		if preILStatus != nil && *preILStatus == "26" {
			restore26 = true
		}

		_, err = db.Exec(context.Background(),
			`UPDATE players SET
				status_il = NULL,
				il_start_date = NULL,
				pre_il_status = NULL,
				status_40_man = $3,
				status_26_man = $4
			WHERE id = $1 AND team_id = $2`,
			req.PlayerID, req.TeamID, restore40, restore26)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		store.AppendRosterMove(db, req.PlayerID, req.TeamID, "Activated from IL")
		pName, tName, lID := getPlayerAndTeamName(db, req.PlayerID, req.TeamID)
		store.LogActivity(db, lID, req.TeamID, "Roster Move", fmt.Sprintf("%s activated %s from IL", tName, pName))
		c.JSON(http.StatusOK, gin.H{"message": "Player activated from IL"})
	}
}

type DFARequest struct {
	MoveRequest
	ClearAction string `json:"dfa_clear_action"` // "release" or "minors"
}

func DFAPlayerHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req DFARequest
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

		if req.ClearAction == "" {
			req.ClearAction = "release"
		}

		waiverEnd := time.Now().Add(48 * time.Hour)

		_, err := db.Exec(context.Background(),
			`UPDATE players SET
				fa_status = 'on waivers',
				waiver_end_time = $1,
				waiving_team_id = $2,
				dfa_clear_action = $3,
				status_26_man = FALSE,
				status_40_man = FALSE
			WHERE id = $4 AND team_id = $2`,
			waiverEnd, req.TeamID, req.ClearAction, req.PlayerID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		store.AppendRosterMove(db, req.PlayerID, req.TeamID, "Designated for Assignment")
		pName, tName, lID := getPlayerAndTeamName(db, req.PlayerID, req.TeamID)
		store.LogActivity(db, lID, req.TeamID, "Roster Move", fmt.Sprintf("%s designated %s for assignment", tName, pName))
		c.JSON(http.StatusOK, gin.H{"message": "Player designated for assignment (DFA)"})
	}
}

func WaivePlayerHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req MoveRequest
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

		waiverEnd := time.Now().Add(48 * time.Hour)

		_, err := db.Exec(context.Background(),
			`UPDATE players SET
				fa_status = 'on waivers',
				waiver_end_time = $1,
				waiving_team_id = $2,
				dfa_clear_action = 'minors',
				status_26_man = FALSE
			WHERE id = $3 AND team_id = $2`,
			waiverEnd, req.TeamID, req.PlayerID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		store.AppendRosterMove(db, req.PlayerID, req.TeamID, "Placed on Waivers")
		pName, tName, lID := getPlayerAndTeamName(db, req.PlayerID, req.TeamID)
		store.LogActivity(db, lID, req.TeamID, "Roster Move", fmt.Sprintf("%s placed %s on waivers", tName, pName))
		c.JSON(http.StatusOK, gin.H{"message": "Player placed on waivers"})
	}
}

type PositionSwapRequest struct {
	MoveRequest
	TargetPosition string `json:"target_position"` // Required for "P" players (must be "SP" or "RP")
}

func SwapPitcherPositionHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req PositionSwapRequest
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

		// Get current position
		var position string
		err := db.QueryRow(ctx, "SELECT position FROM players WHERE id = $1 AND team_id = $2", req.PlayerID, req.TeamID).Scan(&position)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Player not found on this team"})
			return
		}

		if position != "SP" && position != "RP" && position != "P" && position != "SP,RP" && position != "RP,SP" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Only pitchers (SP, RP, P, SP/RP) can use position swap"})
			return
		}

		isDualEligible := position == "P" || position == "SP,RP" || position == "RP,SP"

		var newPosition string
		if isDualEligible {
			// Dual-eligible players must specify target via target_position
			if req.TargetPosition != "SP" && req.TargetPosition != "RP" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Must specify SP or RP as target position"})
				return
			}
			newPosition = req.TargetPosition
		} else {
			// SP↔RP toggle; ignore target_position
			newPosition = "RP"
			if position == "RP" {
				newPosition = "SP"
			}
		}

		// Check 14-day cooldown via roster_moves_log JSONB (skip for dual-eligible — no cooldown)
		if !isDualEligible {
			var lastSwapDate *time.Time
			err = db.QueryRow(ctx, `
				SELECT MAX((elem->>'date')::date)
				FROM players, jsonb_array_elements(COALESCE(roster_moves_log, '[]'::jsonb)) AS elem
				WHERE players.id = $1 AND elem->>'type' LIKE 'Position Swap%'
			`, req.PlayerID).Scan(&lastSwapDate)
			if err == nil && lastSwapDate != nil {
				cooldownEnd := lastSwapDate.AddDate(0, 0, 14)
				if time.Now().Before(cooldownEnd) {
					c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Position swap on cooldown. Available after %s.", cooldownEnd.Format("January 2, 2006"))})
					return
				}
			}
		}

		// If targeting SP on 26-man, check SP limit
		if newPosition == "SP" {
			var is26Man bool
			db.QueryRow(ctx, "SELECT status_26_man FROM players WHERE id = $1", req.PlayerID).Scan(&is26Man)
			if is26Man {
				leagueID, _ := store.GetTeamLeagueID(db, req.TeamID)
				settings := store.GetLeagueSettings(db, leagueID, time.Now().Year())
				spCount, _ := store.GetTeam26ManSPCount(db, req.TeamID)
				if spCount >= settings.SP26ManLimit {
					c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("SP limit reached (%d/%d on 26-man). Cannot assign as SP.", spCount, settings.SP26ManLimit)})
					return
				}
			}
		}

		// Perform the update
		_, err = db.Exec(ctx, "UPDATE players SET position = $1 WHERE id = $2 AND team_id = $3", newPosition, req.PlayerID, req.TeamID)
		if err != nil {
			fmt.Printf("ERROR [SwapPitcherPositionHandler]: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		moveDesc := fmt.Sprintf("Position Swap: %s → %s", position, newPosition)
		store.AppendRosterMove(db, req.PlayerID, req.TeamID, moveDesc)

		pName, tName, lID := getPlayerAndTeamName(db, req.PlayerID, req.TeamID)
		store.LogActivity(db, lID, req.TeamID, "Roster Move", fmt.Sprintf("%s swapped %s from %s to %s", tName, pName, position, newPosition))

		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Position changed from %s to %s", position, newPosition)})
	}
}

func ToggleTradeBlockHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req TradeBlockRequest
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

		_, err := db.Exec(context.Background(),
			`UPDATE players SET on_trade_block = $1, trade_block_notes = $2 WHERE id = $3 AND team_id = $4`,
			req.OnBlock, req.Notes, req.PlayerID, req.TeamID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		if req.OnBlock {
			var pName, pPos, tName, lID string
			db.QueryRow(context.Background(), `
				SELECT p.first_name || ' ' || p.last_name, p.position, t.name, p.league_id 
				FROM players p JOIN teams t ON p.team_id = t.id WHERE p.id = $1`, req.PlayerID).Scan(&pName, &pPos, &tName, &lID)
			
			msg := fmt.Sprintf("📢 *Trade Block Alert!* _%s_ (%s) has been put on the block by *%s*.\n📝 *Notes:* %s", pName, pPos, tName, req.Notes)
			notification.SendSlackNotification(db, lID, "trade_block", msg)
		}

		msg := "Player removed from trade block"
		if req.OnBlock {
			msg = "Player added to trade block"
		}
		c.JSON(http.StatusOK, gin.H{"message": msg})
	}
}
