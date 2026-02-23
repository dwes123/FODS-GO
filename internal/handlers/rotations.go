package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// capitalize returns s with its first letter uppercased
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// weekFromString parses "YYYY-WW" into year and week number
func weekFromString(s string) (int, int) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		y, w := time.Now().ISOWeek()
		return y, w
	}
	y, _ := strconv.Atoi(parts[0])
	w, _ := strconv.Atoi(parts[1])
	if y == 0 || w == 0 {
		y, w = time.Now().ISOWeek()
	}
	return y, w
}

// adjacentWeeks returns the previous and next week identifiers
func adjacentWeeks(weekStr string) (string, string) {
	y, w := weekFromString(weekStr)

	// Previous week
	py, pw := y, w-1
	if pw < 1 {
		py--
		// ISO weeks: last week of previous year
		t := time.Date(py, 12, 28, 0, 0, 0, 0, time.UTC) // Dec 28 is always in the last ISO week
		_, pw = t.ISOWeek()
	}

	// Next week
	ny, nw := y, w+1
	// Check if current year's last week is exceeded
	t := time.Date(y, 12, 28, 0, 0, 0, 0, time.UTC)
	_, maxWeek := t.ISOWeek()
	if nw > maxWeek {
		ny++
		nw = 1
	}

	return fmt.Sprintf("%d-%02d", py, pw), fmt.Sprintf("%d-%02d", ny, nw)
}

// weekDateRange returns the Monday and Sunday dates for a given ISO week
func weekDateRange(weekStr string) (string, string) {
	y, w := weekFromString(weekStr)
	// Find Jan 4 of the year (always in ISO week 1), then calculate Monday of week 1
	jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, time.UTC)
	// Monday of ISO week 1
	offset := int(time.Monday - jan4.Weekday())
	if jan4.Weekday() == time.Sunday {
		offset = -6
	}
	week1Monday := jan4.AddDate(0, 0, offset)
	// Monday of target week
	monday := week1Monday.AddDate(0, 0, (w-1)*7)
	sunday := monday.AddDate(0, 0, 6)
	return monday.Format("Jan 2"), sunday.Format("Jan 2")
}

func RotationsDashboardHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)

		leagueID := c.Query("league_id")
		if leagueID == "" {
			leagueID = "11111111-1111-1111-1111-111111111111" // Default MLB
		}

		year, week := time.Now().ISOWeek()
		currentWeek := fmt.Sprintf("%d-%02d", year, week)

		selectedWeek := c.Query("week")
		if selectedWeek == "" {
			selectedWeek = currentWeek
		}

		submissions, _ := store.GetWeeklyRotations(db, leagueID, selectedWeek)
		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		totalTeams, _ := store.GetLeagueTeamCount(db, leagueID)
		submittedTeams, _ := store.GetSubmittedTeamCount(db, leagueID, selectedWeek)

		prevWeek, nextWeek := adjacentWeeks(selectedWeek)
		monDate, sunDate := weekDateRange(selectedWeek)

		RenderTemplate(c, "rotations.html", gin.H{
			"User":           user,
			"Submissions":    submissions,
			"Leagues":        leagues,
			"SelectedLID":    leagueID,
			"SelectedWeek":   selectedWeek,
			"CurrentWeek":    currentWeek,
			"PrevWeek":       prevWeek,
			"NextWeek":       nextWeek,
			"WeekStart":      monDate,
			"WeekEnd":        sunDate,
			"TotalTeams":     totalTeams,
			"SubmittedTeams": submittedTeams,
			"Days":           []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"},
			"IsCommish":      len(adminLeagues) > 0,
		})
	}
}

func RotationsSubmitPageHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)

		myTeams, _ := store.GetManagedTeams(db, user.ID)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		year, week := time.Now().ISOWeek()
		currentWeek := fmt.Sprintf("%d-%02d", year, week)

		selectedWeek := c.Query("week")
		if selectedWeek == "" {
			selectedWeek = currentWeek
		}

		prevWeek, nextWeek := adjacentWeeks(selectedWeek)
		monDate, sunDate := weekDateRange(selectedWeek)

		RenderTemplate(c, "rotations_submit.html", gin.H{
			"User":         user,
			"MyTeams":      myTeams,
			"Days":         []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"},
			"SelectedWeek": selectedWeek,
			"PrevWeek":     prevWeek,
			"NextWeek":     nextWeek,
			"WeekStart":    monDate,
			"WeekEnd":      sunDate,
			"IsCommish":    len(adminLeagues) > 0,
		})
	}
}

func SubmitRotationHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)

		teamID := c.PostForm("team_id")
		week := c.PostForm("week")

		if teamID == "" || week == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Team and week are required"})
			return
		}

		// Verify ownership
		isOwner, _ := store.IsTeamOwner(db, teamID, user.ID)
		if !isOwner {
			c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
			return
		}

		// Get league ID for the team
		var leagueID string
		err := db.QueryRow(c, "SELECT league_id FROM teams WHERE id = $1", teamID).Scan(&leagueID)
		if err != nil {
			fmt.Printf("ERROR [SubmitRotation]: could not find team: %v\n", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team"})
			return
		}

		days := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
		dayIndex := map[string]int{
			"monday": 0, "tuesday": 1, "wednesday": 2, "thursday": 3,
			"friday": 4, "saturday": 5, "sunday": 6,
		}
		var errors []string

		// Parse banked starters usage from hidden field
		type bankedUsageEntry struct {
			Day  string `json:"day"`
			ID   string `json:"id"`
			Name string `json:"name"`
			Date string `json:"date"`
		}
		var bankedUsage []bankedUsageEntry
		bankedJSON := c.PostForm("banked_starters")
		if bankedJSON != "" {
			if err := json.Unmarshal([]byte(bankedJSON), &bankedUsage); err != nil {
				errors = append(errors, "Invalid banked starters data")
			}
		}

		// Validate all days first
		type dayData struct {
			day    string
			p1ID   string
			p1Date string
			p2ID   string
			p2Date string
			empty  bool
		}
		var entries []dayData

		for _, day := range days {
			p1ID := c.PostForm(day + "_p1_id")
			p1Date := c.PostForm(day + "_p1_date")
			p2ID := c.PostForm(day + "_p2_id")
			p2Date := c.PostForm(day + "_p2_date")

			isEmpty := p1ID == "" && p2ID == ""

			// Validate: can't bank (set p2) without an active starter (p1)
			if p2ID != "" && p1ID == "" {
				errors = append(errors, fmt.Sprintf("%s: Cannot bank a pitcher without an active starter", capitalize(day)))
				continue
			}

			// Validate: same pitcher on both slots
			if p1ID != "" && p2ID != "" && p1ID == p2ID {
				errors = append(errors, fmt.Sprintf("%s: Active starter and banked pitcher must be different", capitalize(day)))
				continue
			}

			// Validate: pitchers belong to team's 26-man roster
			if p1ID != "" {
				onRoster, err := store.IsPlayerOn26Man(db, p1ID, teamID)
				if err != nil || !onRoster {
					errors = append(errors, fmt.Sprintf("%s Active Starter: Pitcher is not on this team's 26-man roster", capitalize(day)))
				}
			}
			if p2ID != "" {
				onRoster, err := store.IsPlayerOn26Man(db, p2ID, teamID)
				if err != nil || !onRoster {
					errors = append(errors, fmt.Sprintf("%s Banked Pitcher: Pitcher is not on this team's 26-man roster", capitalize(day)))
				}
			}

			entries = append(entries, dayData{day: day, p1ID: p1ID, p1Date: p1Date, p2ID: p2ID, p2Date: p2Date, empty: isEmpty})
		}

		// Count total banked (days with pitcher_2 set)
		totalBanked := 0
		// Track which pitcher is banked on which day
		bankedPitchersByDay := make(map[string]string) // day -> pitcher_id
		for _, e := range entries {
			if e.p2ID != "" {
				totalBanked++
				bankedPitchersByDay[e.day] = e.p2ID
			}
		}

		// Build map of active starters by day for invalidation checking
		activeStartersByDay := make(map[string]string) // day -> pitcher_id
		for _, e := range entries {
			if e.p1ID != "" {
				activeStartersByDay[e.day] = e.p1ID
			}
		}

		// Validate banked usage
		if len(bankedUsage) > totalBanked {
			errors = append(errors, fmt.Sprintf("Cannot use %d banked starts — only %d available", len(bankedUsage), totalBanked))
		}

		// Validate each banked usage entry
		for _, bu := range bankedUsage {
			usageDayIdx, validDay := dayIndex[bu.Day]
			if !validDay {
				errors = append(errors, fmt.Sprintf("Invalid day for banked start usage: %s", bu.Day))
				continue
			}

			// Find which day this pitcher was banked
			bankedDay := ""
			for day, pid := range bankedPitchersByDay {
				if pid == bu.ID {
					bankedDay = day
					break
				}
			}
			if bankedDay == "" {
				errors = append(errors, fmt.Sprintf("%s: Pitcher is not banked on any day this week", capitalize(bu.Day)))
				continue
			}

			bankedDayIdx := dayIndex[bankedDay]

			// Usage must be on a later day than banking
			if usageDayIdx <= bankedDayIdx {
				errors = append(errors, fmt.Sprintf("%s: Banked start must be used on a day after it was banked (%s)", capitalize(bu.Day), capitalize(bankedDay)))
				continue
			}

			// Invalidation: pitcher can't have a regular start (pitcher_1) between banking day and usage day
			for _, checkDay := range days {
				checkIdx := dayIndex[checkDay]
				if checkIdx > bankedDayIdx && checkIdx <= usageDayIdx {
					if activeStartersByDay[checkDay] == bu.ID {
						errors = append(errors, fmt.Sprintf("%s: Banked start invalidated — pitcher has a regular start on %s", capitalize(bu.Day), capitalize(checkDay)))
						break
					}
				}
			}
		}

		if len(errors) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": strings.Join(errors, "; ")})
			return
		}

		// Group banked usage by target day
		bankedByDay := make(map[string][]bankedUsageEntry)
		for _, bu := range bankedUsage {
			bankedByDay[bu.Day] = append(bankedByDay[bu.Day], bu)
		}

		// Save all days in a transaction
		tx, err := db.Begin(c)
		if err != nil {
			fmt.Printf("ERROR [SubmitRotation]: begin tx: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
		defer tx.Rollback(c)

		for _, e := range entries {
			dayNum := dayIndex[e.day]
			if e.empty && len(bankedByDay[e.day]) == 0 {
				// Delete this day's entry if it exists
				_, err := tx.Exec(c,
					`DELETE FROM rotations WHERE team_id = $1 AND week_identifier = $2 AND day_of_week = $3`,
					teamID, week, dayNum)
				if err != nil {
					fmt.Printf("ERROR [SubmitRotation]: delete %s: %v\n", e.day, err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
					return
				}
			} else {
				var p1, p2 *string
				if e.p1ID != "" { p1 = &e.p1ID }
				if e.p2ID != "" { p2 = &e.p2ID }
				var d1, d2 *string
				if e.p1Date != "" { d1 = &e.p1Date }
				if e.p2Date != "" { d2 = &e.p2Date }

				// Build banked_starters JSONB for this day
				var bankedJSONB interface{}
				if dayBanked, ok := bankedByDay[e.day]; ok && len(dayBanked) > 0 {
					type bankedDB struct {
						ID   string `json:"id"`
						Name string `json:"name"`
						Date string `json:"date"`
					}
					var dbEntries []bankedDB
					for _, b := range dayBanked {
						dbEntries = append(dbEntries, bankedDB{ID: b.ID, Name: b.Name, Date: b.Date})
					}
					jsonBytes, _ := json.Marshal(dbEntries)
					bankedJSONB = jsonBytes
				}

				_, err := tx.Exec(c, `
					INSERT INTO rotations (team_id, league_id, week_identifier, day_of_week, pitcher_1_id, pitcher_1_date, pitcher_2_id, pitcher_2_date, banked_starters, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
					ON CONFLICT (team_id, week_identifier, day_of_week) DO UPDATE SET
						pitcher_1_id = EXCLUDED.pitcher_1_id,
						pitcher_1_date = EXCLUDED.pitcher_1_date,
						pitcher_2_id = EXCLUDED.pitcher_2_id,
						pitcher_2_date = EXCLUDED.pitcher_2_date,
						banked_starters = EXCLUDED.banked_starters,
						updated_at = NOW()
				`, teamID, leagueID, week, dayNum, p1, d1, p2, d2, bankedJSONB)
				if err != nil {
					fmt.Printf("ERROR [SubmitRotation]: upsert %s: %v\n", e.day, err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
					return
				}
			}
		}

		if err := tx.Commit(c); err != nil {
			fmt.Printf("ERROR [SubmitRotation]: commit: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Rotation saved successfully"})
	}
}

func GetTeamPitchersHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID := c.Query("team_id")
		pitchers, err := store.GetPitchersForTeam(db, teamID)
		if err != nil {
			fmt.Printf("ERROR [GetTeamPitchers]: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
		c.JSON(http.StatusOK, pitchers)
	}
}

// GetTeamRotationHandler returns existing rotation data for a team+week as JSON
func GetTeamRotationHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID := c.Query("team_id")
		week := c.Query("week")

		if teamID == "" || week == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "team_id and week are required"})
			return
		}

		data, err := store.GetTeamWeekRotation(db, teamID, week)
		if err != nil {
			fmt.Printf("ERROR [GetTeamRotation]: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		c.JSON(http.StatusOK, data)
	}
}
