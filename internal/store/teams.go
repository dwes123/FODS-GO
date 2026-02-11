package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RosterPlayer struct {
	ID          string         `json:"id"`
	FirstName   string         `json:"first_name"`
	LastName    string         `json:"last_name"`
	Position    string         `json:"position"`
	MLBTeam     string         `json:"mlb_team"`
	Status      string         `json:"status"`
	Status40Man bool           `json:"status_40_man"`
	Status26Man bool           `json:"status_26_man"`
	StatusIL    string         `json:"status_il"`
	OptionYears int            `json:"option_years_used"`
	LeagueID    string         `json:"league_id"`
	LeagueName  string         `json:"league_name"`
	TeamID      string         `json:"team_id"`
	Contracts   map[int]string `json:"contracts"` // Year -> Amount
}

type SalaryYearSummary struct {
	Year           int     `json:"year"`
	ActivePayroll  float64 `json:"active_payroll"`
	DeadCap        float64 `json:"dead_cap"`
	TotalPayroll   float64 `json:"total_payroll"`
	LuxuryTaxLimit float64 `json:"luxury_tax_limit"`
	TaxSpace       float64 `json:"tax_space"`
}

type TeamDetail struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Owner         string              `json:"owner"` // Legacy single owner name
	Owners        []string            `json:"owners"` // List of user names who own this team
	UserID        string              `json:"user_id"`
	LeagueID      string              `json:"league_id"`
	LeagueName    string              `json:"league_name"`
	IsbpBalance   float64             `json:"isbp_balance"`
	Players       []RosterPlayer      `json:"players"`
	SalarySummary []SalaryYearSummary `json:"salary_summary"`
	Years         []int               `json:"years"`
}

func GetTeamWithRoster(db *pgxpool.Pool, teamID string) (*TeamDetail, error) {
	ctx := context.Background()
	var team TeamDetail
	for y := 2026; y <= 2040; y++ {
		team.Years = append(team.Years, y)
	}

	// Updated query to join leagues and fetch name
	err := db.QueryRow(ctx,
		`SELECT t.id, t.name, t.owner_name, COALESCE(t.user_id, '00000000-0000-0000-0000-000000000000'), t.league_id, t.isbp_balance, l.name
		 FROM teams t
		 JOIN leagues l ON t.league_id = l.id
		 WHERE t.id = $1`,
		teamID).Scan(&team.ID, &team.Name, &team.Owner, &team.UserID, &team.LeagueID, &team.IsbpBalance, &team.LeagueName)

	if err != nil {
		return nil, err
	}

	// Fetch all owners
	ownerRows, _ := db.Query(ctx, "SELECT u.username FROM users u JOIN team_owners town ON u.id = town.user_id WHERE town.team_id = $1", teamID)
	if ownerRows != nil {
		for ownerRows.Next() {
			var name string
			ownerRows.Scan(&name)
			team.Owners = append(team.Owners, name)
		}
		ownerRows.Close()
	}

	query := `
		SELECT id, first_name, last_name, position, mlb_team,
		       status_40_man, status_26_man, COALESCE(status_il, ''), option_years_used,
		       COALESCE(contract_2026, ''), COALESCE(contract_2027, ''), COALESCE(contract_2028, ''),
		       COALESCE(contract_2029, ''), COALESCE(contract_2030, ''), COALESCE(contract_2031, ''),
		       COALESCE(contract_2032, ''), COALESCE(contract_2033, ''), COALESCE(contract_2034, ''),
		       COALESCE(contract_2035, ''), COALESCE(contract_2036, ''), COALESCE(contract_2037, ''),
		       COALESCE(contract_2038, ''), COALESCE(contract_2039, ''), COALESCE(contract_2040, '')
		FROM players
		WHERE team_id = $1
	`

	rows, err := db.Query(ctx, query, teamID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p RosterPlayer
			p.Contracts = make(map[int]string)
			contracts := make([]string, 15)

			dest := []interface{}{
				&p.ID, &p.FirstName, &p.LastName, &p.Position, &p.MLBTeam,
				&p.Status40Man, &p.Status26Man, &p.StatusIL, &p.OptionYears,
			}
			for i := range contracts {
				dest = append(dest, &contracts[i])
			}

			if err := rows.Scan(dest...); err != nil {
				continue
			}

			for i, year := range team.Years {
				p.Contracts[year] = contracts[i]
			}

			if p.StatusIL != "" {
				p.Status = p.StatusIL
			} else if p.Status26Man {
				p.Status = "Active (26-Man)"
			} else if p.Status40Man {
				p.Status = "40-Man (Minors)"
			} else {
				p.Status = "Minors (Non-40)"
			}
			team.Players = append(team.Players, p)
		}
	}

	for _, year := range team.Years {
		summary := CalculateYearlySummary(db, team.ID, team.LeagueID, year)
		team.SalarySummary = append(team.SalarySummary, summary)
	}

	if team.Players == nil {
		team.Players = []RosterPlayer{}
	}

	return &team, nil
}

func CalculateYearlySummary(db *pgxpool.Pool, teamID, leagueID string, year int) SalaryYearSummary {
	ctx := context.Background()
	var s SalaryYearSummary
	s.Year = year

	contractCol := fmt.Sprintf("contract_%d", year)
	rows, _ := db.Query(ctx, fmt.Sprintf("SELECT %s FROM players WHERE team_id = $1", contractCol), teamID)
	if rows != nil {
		for rows.Next() {
			var val string
			if err := rows.Scan(&val); err == nil && val != "" {
				clean := strings.ReplaceAll(strings.ReplaceAll(val, "$", ""), ",", "")
				amt, _ := strconv.ParseFloat(clean, 64)
				s.ActivePayroll += amt
			}
		}
		rows.Close()
	}

	db.QueryRow(ctx, "SELECT COALESCE(SUM(amount), 0) FROM dead_cap_penalties WHERE team_id = $1 AND year = $2", teamID, year).Scan(&s.DeadCap)
	db.QueryRow(ctx, "SELECT COALESCE(luxury_tax_limit, 0) FROM league_settings WHERE league_id = $1 AND year = $2", leagueID, year).Scan(&s.LuxuryTaxLimit)

	s.TotalPayroll = s.ActivePayroll + s.DeadCap
	if s.LuxuryTaxLimit > 0 {
		s.TaxSpace = s.LuxuryTaxLimit - s.TotalPayroll
	}

	return s
}

func IsTeamOwner(db *pgxpool.Pool, teamID, userID string) (bool, error) {
	var count int
	err := db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM team_owners WHERE team_id = $1 AND user_id = $2`,
		teamID, userID).Scan(&count)
	return count > 0, err
}

func GetManagedTeams(db *pgxpool.Pool, userID string) ([]TeamDetail, error) {
	rows, err := db.Query(context.Background(), `
		SELECT t.id, t.name, t.owner_name, t.league_id, l.name as league_name
		FROM teams t
		JOIN team_owners town ON t.id = town.team_id
		JOIN leagues l ON t.league_id = l.id
		WHERE town.user_id = $1
	`, userID)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []TeamDetail
	for rows.Next() {
		var t TeamDetail
		if err := rows.Scan(&t.ID, &t.Name, &t.Owner, &t.LeagueID, &t.LeagueName); err != nil {
			continue
		}
		
		// Optional: Fetch roster if needed, but for Home page teaser maybe not?
		// We'll skip deep fetch for Home page to keep it fast.
		
		teams = append(teams, t)
	}
	return teams, nil
}

func GetUnassignedTeams(db *pgxpool.Pool) ([]TeamDetail, error) {
	rows, err := db.Query(context.Background(), `
		SELECT t.id, t.name, t.owner_name, l.name as league_name
		FROM teams t
		JOIN leagues l ON t.league_id = l.id
		WHERE NOT EXISTS (SELECT 1 FROM team_owners WHERE team_id = t.id)
		ORDER BY l.name, t.name
	`)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []TeamDetail
	for rows.Next() {
		var t TeamDetail
		if err := rows.Scan(&t.ID, &t.Name, &t.Owner, &t.LeagueName); err != nil {
			continue
		}
		t.Name = t.Name + " (" + t.LeagueName + ")"
		teams = append(teams, t)
	}
	return teams, nil
}

func ClaimTeam(db *pgxpool.Pool, teamID, userID, username string) error {
	// First check if anyone already owns it
	var count int
	db.QueryRow(context.Background(), "SELECT COUNT(*) FROM team_owners WHERE team_id = $1", teamID).Scan(&count)
	if count > 0 {
		return context.DeadlineExceeded
	}

	_, err := db.Exec(context.Background(), `
		INSERT INTO team_owners (team_id, user_id)
		VALUES ($1, $2)
	`, teamID, userID)

	return err
}

func GetTeamRosterCounts(db *pgxpool.Pool, teamID string) (count26 int, count40 int, err error) {
	err = db.QueryRow(context.Background(), `
		SELECT 
			COUNT(*) FILTER (WHERE status_26_man = TRUE),
			COUNT(*) FILTER (WHERE status_40_man = TRUE)
		FROM players 
		WHERE team_id = $1
	`, teamID).Scan(&count26, &count40)
	return
}