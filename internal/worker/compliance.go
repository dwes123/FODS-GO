package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/notification"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LeagueIDs for all 4 leagues
var allLeagueIDs = []string{
	"11111111-1111-1111-1111-111111111111", // MLB
	"22222222-2222-2222-2222-222222222222", // AAA
	"33333333-3333-3333-3333-333333333333", // AA
	"44444444-4444-4444-4444-444444444444", // High-A
}

var leagueNames = map[string]string{
	"11111111-1111-1111-1111-111111111111": "MLB",
	"22222222-2222-2222-2222-222222222222": "AAA",
	"33333333-3333-3333-3333-333333333333": "AA",
	"44444444-4444-4444-4444-444444444444": "High-A",
}

// ComplianceViolation represents a single roster compliance issue
type ComplianceViolation struct {
	LeagueID   string
	LeagueName string
	TeamID     string
	TeamName   string
	Issue      string
}

// ComplianceReport is the full result of a compliance check
type ComplianceReport struct {
	Violations []ComplianceViolation
	CheckedAt  time.Time
}

// StartComplianceWorker runs a daily roster compliance check at 8 AM ET.
// Checks all teams across all leagues for roster limit violations and
// posts a summary to Slack if any violations are found.
func StartComplianceWorker(ctx context.Context, db *pgxpool.Pool) {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("Compliance worker stopped")
				return
			case <-ticker.C:
				checkComplianceIfNeeded(ctx, db)
			}
		}
	}()
}

func checkComplianceIfNeeded(ctx context.Context, db *pgxpool.Pool) {
	// Run once daily at 8 AM ET
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.FixedZone("ET", -5*3600)
	}
	now := time.Now().In(loc)

	if now.Hour() != 8 {
		return
	}

	// Only run during the regular season (Mar 15 – Oct 15)
	month := now.Month()
	day := now.Day()
	if month < 3 || (month == 3 && day < 15) || month > 10 || (month == 10 && day > 15) {
		return
	}

	// Prevent duplicate runs on the same day
	key := fmt.Sprintf("compliance_check_%s", now.Format("2006-01-02"))
	if hasRunThisYear(db, context.Background(), key) {
		return
	}

	report := RunComplianceCheck(db)
	markAsRun(db, context.Background(), key)

	if len(report.Violations) == 0 {
		fmt.Printf("Compliance Worker: All teams compliant (%s)\n", now.Format("2006-01-02"))
		return
	}

	fmt.Printf("Compliance Worker: Found %d violations (%s)\n", len(report.Violations), now.Format("2006-01-02"))
	for _, v := range report.Violations {
		fmt.Printf("  VIOLATION: [%s] %s — %s\n", v.LeagueName, v.TeamName, v.Issue)
	}

	// Group violations by league for Slack notifications
	byLeague := make(map[string][]ComplianceViolation)
	for _, v := range report.Violations {
		byLeague[v.LeagueID] = append(byLeague[v.LeagueID], v)
	}

	for leagueID, violations := range byLeague {
		msg := formatComplianceSlackMessage(violations)
		if err := notification.SendSlackNotification(db, leagueID, "transactions", msg); err != nil {
			fmt.Printf("Compliance Worker: Slack error for %s: %v\n", leagueNames[leagueID], err)
		}
	}
}

// RunComplianceCheck checks all teams across all leagues for roster violations.
// Exported so it can be triggered manually from the admin dashboard.
func RunComplianceCheck(db *pgxpool.Pool) ComplianceReport {
	report := ComplianceReport{CheckedAt: time.Now()}
	ctx := context.Background()
	year := time.Now().Year()

	for _, leagueID := range allLeagueIDs {
		settings := store.GetLeagueSettings(db, leagueID, year)

		rows, err := db.Query(ctx, "SELECT id, name FROM teams WHERE league_id = $1 ORDER BY name", leagueID)
		if err != nil {
			fmt.Printf("Compliance Worker: Error querying teams for %s: %v\n", leagueNames[leagueID], err)
			continue
		}

		type teamInfo struct{ ID, Name string }
		var teams []teamInfo
		for rows.Next() {
			var t teamInfo
			rows.Scan(&t.ID, &t.Name)
			teams = append(teams, t)
		}
		rows.Close()

		for _, t := range teams {
			count26, count40, err := store.GetTeamRosterCounts(db, t.ID)
			if err != nil {
				continue
			}
			spCount, _ := store.GetTeam26ManSPCount(db, t.ID)

			if count26 > settings.Roster26ManLimit {
				report.Violations = append(report.Violations, ComplianceViolation{
					LeagueID:   leagueID,
					LeagueName: leagueNames[leagueID],
					TeamID:     t.ID,
					TeamName:   t.Name,
					Issue:      fmt.Sprintf("26-man roster over limit: %d/%d", count26, settings.Roster26ManLimit),
				})
			}

			if count40 > settings.Roster40ManLimit {
				report.Violations = append(report.Violations, ComplianceViolation{
					LeagueID:   leagueID,
					LeagueName: leagueNames[leagueID],
					TeamID:     t.ID,
					TeamName:   t.Name,
					Issue:      fmt.Sprintf("40-man roster over limit: %d/%d", count40, settings.Roster40ManLimit),
				})
			}

			if spCount > settings.SP26ManLimit {
				report.Violations = append(report.Violations, ComplianceViolation{
					LeagueID:   leagueID,
					LeagueName: leagueNames[leagueID],
					TeamID:     t.ID,
					TeamName:   t.Name,
					Issue:      fmt.Sprintf("SP on 26-man over limit: %d/%d", spCount, settings.SP26ManLimit),
				})
			}
		}
	}

	return report
}

func formatComplianceSlackMessage(violations []ComplianceViolation) string {
	if len(violations) == 0 {
		return ""
	}

	leagueName := violations[0].LeagueName
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Roster Compliance Alert — %s*\n", leagueName))
	sb.WriteString(fmt.Sprintf("_%d violation(s) found:_\n", len(violations)))

	for _, v := range violations {
		sb.WriteString(fmt.Sprintf("• *%s* — %s\n", v.TeamName, v.Issue))
	}

	return sb.String()
}
