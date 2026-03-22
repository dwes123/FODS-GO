package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	leagueMLB   = "11111111-1111-1111-1111-111111111111"
	leagueAAA   = "22222222-2222-2222-2222-222222222222"
	leagueAA    = "33333333-3333-3333-3333-333333333333"
	leagueHighA = "44444444-4444-4444-4444-444444444444"
)

type IRLTeamFinancial struct {
	TeamID        string
	TeamName      string
	OwnerName     string
	TotalSalary   float64
	BuyIn         float64
	LuxuryTaxPen  float64
	TotalIRL      float64
	IsOverTax     bool
	AmountOverTax float64
}

// billingBand represents a cumulative salary band with a flat IRL fee
type billingBand struct {
	Ceiling float64
	Fee     float64
}

// taxTier represents a tier of luxury tax penalties
type taxTier struct {
	WidthMillions float64 // width of tier in millions
	RatePerM      float64 // IRL $ per million over
}

func IRLFinancialsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagueID := c.Query("league_id")
		yearStr := c.Query("year")

		year, err := strconv.Atoi(yearStr)
		if err != nil {
			year = 2026
		}
		if year < 2026 || year > 2027 {
			year = 2026
		}

		if leagueID == "" {
			db.QueryRow(context.Background(), "SELECT t.league_id FROM teams t JOIN team_owners to2 ON t.id = to2.team_id WHERE to2.user_id = $1 LIMIT 1", user.ID).Scan(&leagueID)
		}
		if leagueID == "" {
			leagueID = leagueMLB
		}

		var leagueName string
		db.QueryRow(context.Background(), "SELECT name FROM leagues WHERE id = $1", leagueID).Scan(&leagueName)

		// Get luxury tax limit from league_settings
		var luxuryTaxLimit float64
		db.QueryRow(context.Background(), "SELECT COALESCE(luxury_tax_limit, 0) FROM league_settings WHERE league_id = $1 AND year = $2", leagueID, year).Scan(&luxuryTaxLimit)

		// Fetch teams
		rows, err := db.Query(context.Background(), "SELECT id, name, COALESCE(owner_name, '') FROM teams WHERE league_id = $1 ORDER BY name", leagueID)
		if err != nil {
			fmt.Printf("ERROR [IRLFinancials]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		type teamBasic struct {
			ID, Name, Owner string
		}
		var teams []teamBasic
		for rows.Next() {
			var t teamBasic
			if err := rows.Scan(&t.ID, &t.Name, &t.Owner); err == nil {
				teams = append(teams, t)
			}
		}
		rows.Close()

		var results []IRLTeamFinancial
		for _, t := range teams {
			summary := store.CalculateYearlySummary(db, t.ID, leagueID, year)
			salary := summary.TotalPayroll

			buyIn := calculateBuyIn(leagueID, salary, year)
			amountOver := salary - luxuryTaxLimit
			taxPen := calculateLuxuryTaxPenalty(leagueID, amountOver, year)

			results = append(results, IRLTeamFinancial{
				TeamID:        t.ID,
				TeamName:      t.Name,
				OwnerName:     t.Owner,
				TotalSalary:   salary,
				BuyIn:         buyIn,
				LuxuryTaxPen:  taxPen,
				TotalIRL:      buyIn + taxPen,
				IsOverTax:     amountOver > 0,
				AmountOverTax: math.Max(amountOver, 0),
			})
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].TotalSalary > results[j].TotalSalary
		})

		// Calculate league totals
		var totalBuyIn, totalTax, totalIRL float64
		for _, r := range results {
			totalBuyIn += r.BuyIn
			totalTax += r.LuxuryTaxPen
			totalIRL += r.TotalIRL
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "irl_financials.html", gin.H{
			"User":           user,
			"Teams":          results,
			"Leagues":        leagues,
			"LeagueID":       leagueID,
			"LeagueName":     leagueName,
			"Year":           year,
			"LuxuryTaxLimit": luxuryTaxLimit,
			"TotalBuyIn":     totalBuyIn,
			"TotalTax":       totalTax,
			"TotalIRL":       totalIRL,
			"IsCommish":      len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}

func calculateBuyIn(leagueID string, salary float64, year int) float64 {
	if salary <= 0 {
		return 0
	}
	switch leagueID {
	case leagueMLB:
		return calculateMLBBuyIn(salary, year)
	case leagueAAA:
		return calculateAAABuyIn(salary, year)
	case leagueAA:
		return calculateAABuyIn(salary, year)
	case leagueHighA:
		return calculateHighABuyIn(salary, year)
	}
	return 0
}

// MLB: Cumulative billing bands ($20M increments, $10/band base)
func calculateMLBBuyIn(salary float64, year int) float64 {
	leagueMin := 760000.0
	if year == 2027 {
		leagueMin = 780000.0
	}
	firstBandCeiling := leagueMin * 20

	firstBandFee := 7.60
	secondBandFee := 17.40
	taxBandFee := 15.00
	taxLine := 241000000.0
	if year == 2027 {
		firstBandFee = 7.80
		secondBandFee = 17.20
		taxBandFee = 17.00
		taxLine = 244000000.0
	}

	bands := []billingBand{
		{firstBandCeiling, firstBandFee},
		{50000000, secondBandFee},
		{70000000, 10.00},
		{90000000, 10.00},
		{110000000, 10.00},
		{130000000, 10.00},
		{150000000, 10.00},
		{170000000, 10.00},
		{190000000, 10.00},
		{210000000, 10.00},
		{taxLine, taxBandFee},
	}

	total := 0.0
	for _, b := range bands {
		total += b.Fee
		if salary <= b.Ceiling {
			break
		}
	}
	return total
}

// AAA: Per-dollar billing at $0.40/1M (2026-2027)
func calculateAAABuyIn(salary float64, year int) float64 {
	rate := 0.40
	taxLine := 241000000.0
	if year == 2027 {
		taxLine = 244000000.0
	}
	billable := math.Min(salary, taxLine)
	return billable / 1000000.0 * rate
}

// AA: Cumulative billing bands ($20M increments, $7/band base)
func calculateAABuyIn(salary float64, year int) float64 {
	leagueMin := 760000.0
	if year == 2027 {
		leagueMin = 780000.0
	}
	firstBandCeiling := leagueMin * 22

	firstBandFee := 5.85
	secondBandFee := 11.65
	taxBandFee := 10.50
	taxLine := 241000000.0
	if year == 2027 {
		firstBandFee = 6.01
		secondBandFee = 11.49
		taxBandFee = 11.90
		taxLine = 244000000.0
	}

	bands := []billingBand{
		{firstBandCeiling, firstBandFee},
		{50000000, secondBandFee},
		{70000000, 7.00},
		{90000000, 7.00},
		{110000000, 7.00},
		{130000000, 7.00},
		{150000000, 7.00},
		{170000000, 7.00},
		{190000000, 7.00},
		{210000000, 7.00},
		{taxLine, taxBandFee},
	}

	total := 0.0
	for _, b := range bands {
		total += b.Fee
		if salary <= b.Ceiling {
			break
		}
	}
	return total
}

// High-A: Range-based hard cap (cumulative ranges)
func calculateHighABuyIn(salary float64, year int) float64 {
	if salary <= 0 {
		return 0
	}
	// Cumulative: base $20, +$10 per additional range, last range +$20
	ranges := []billingBand{
		{100000000, 20.00},
		{150000000, 10.00},
		{200000000, 10.00},
		{241000000, 20.00}, // $200M to tax line
	}
	if year == 2027 {
		ranges[3].Ceiling = 244000000
	}

	total := 0.0
	for _, r := range ranges {
		total += r.Fee
		if salary <= r.Ceiling {
			break
		}
	}
	return total
}

func calculateLuxuryTaxPenalty(leagueID string, amountOver float64, year int) float64 {
	if amountOver <= 0 {
		return 0
	}

	var tiers []taxTier
	switch leagueID {
	case leagueMLB:
		tiers = []taxTier{
			{20, 0.75},
			{20, 1.00},
			{20, 1.45},
			{math.MaxFloat64, 2.00},
		}
	case leagueAAA:
		tiers = []taxTier{
			{20, 0.75},
			{20, 1.00},
			{20, 1.40},
			{math.MaxFloat64, 1.90},
		}
	case leagueAA:
		tiers = []taxTier{
			{20, 0.65},
			{20, 0.90},
			{20, 1.25},
			{math.MaxFloat64, 1.75},
		}
	case leagueHighA:
		tiers = []taxTier{
			{20, 0.50},
			{20, 0.75},
			{20, 1.00},
			{math.MaxFloat64, 1.50},
		}
	default:
		return 0
	}

	overM := amountOver / 1000000.0
	total := 0.0
	remaining := overM
	for _, t := range tiers {
		if remaining <= 0 {
			break
		}
		taxable := math.Min(remaining, t.WidthMillions)
		total += taxable * t.RatePerM
		remaining -= taxable
	}
	return total
}
