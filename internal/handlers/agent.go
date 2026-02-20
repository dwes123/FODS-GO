package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/vertexai/genai"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

var agentSystemPromptTemplate = `You are a fantasy baseball commissioner assistant for Front Office Dynasty Sports.
You help commissioners manage their dynasty fantasy baseball leagues by searching players, checking team data, moving players, and running queries.

Key facts:
- There are 4 leagues: MLB, AAA, AA, and High-A
- League UUIDs: MLB=11111111-1111-1111-1111-111111111111, AAA=22222222-2222-2222-2222-222222222222, AA=33333333-3333-3333-3333-333333333333, High-A=44444444-4444-4444-4444-444444444444
- ~80 teams, ~39,000 players
- Contracts run from 2026-2040, stored as dollar amounts like "$1000000", or "TC" (team control), "ARB" (arbitration), "UFA" (unrestricted free agent)
- ISBP = International Signing Bonus Pool balance
- MiLB = Minor League balance
- Players have status flags: status_40_man (bool), status_26_man (bool), status_il (text), fa_status (text)
- Teams are identified by UUID but you can search by name
- The team_owners junction table links users to teams (not the legacy user_id column)

When answering:
- Be concise and direct
- Format monetary values with dollar signs and commas
- When showing player/team data, use clean formatting
- If a query could affect data, confirm what you're about to do before executing write operations
- For SQL queries, only SELECT statements are allowed

Available tools let you search players, get player/team details, manage rosters, update balances, approve/reject pending items, and run read-only SQL.

The current season year is %d. When users ask about salaries, payroll, or luxury tax without specifying a year, assume %d.

Key database tables and columns for run_query:
- teams: id (uuid), league_id (uuid), name (text), abbreviation (text), isbp_balance (numeric), milb_balance (numeric), owner_name (text)
- players: id (uuid), first_name (text), last_name (text), position (text), mlb_team (text), team_id (uuid), league_id (uuid), status_40_man (bool), status_26_man (bool), status_il (text), fa_status (text), is_international_free_agent (bool), contract_2026 through contract_2040 (text, e.g. "$1000000", "ARB", "TC", "UFA")
- leagues: id (uuid), name (text)
- league_settings: league_id (uuid), year (int), luxury_tax_limit (numeric), roster_26_man_limit (int), roster_40_man_limit (int), sp_26_man_limit (int)
- dead_cap_penalties: id (uuid), team_id (uuid), player_id (uuid, nullable), year (int), amount (numeric), note (text)
- trades: id (uuid), proposing_team_id (uuid), receiving_team_id (uuid), league_id (uuid), status (text — PENDING/ACCEPTED/REJECTED/REVERSED), created_at (timestamp), isbp_offered (numeric), isbp_requested (numeric) — holds trade PROPOSALS made through the app
- trade_players: trade_id (uuid), player_id (uuid), from_team_id (uuid), to_team_id (uuid) — players involved in a trade proposal
- transactions: id (uuid), team_id (uuid), league_id (uuid), transaction_type (text — ADD/DROP/TRADE/COMMISSIONER/ROSTER/WAIVER), summary (text), created_at (timestamp), fantrax_processed (bool) — the ACTIVITY LOG of all completed actions. For trade history, use get_recent_activity with action_type='TRADE'.
- team_owners: team_id (uuid), user_id (uuid) — junction table linking users to teams
- Contract values are TEXT like "$1000000" — to sum them, cast: REPLACE(REPLACE(contract_2026, '$', ''), ',', '')::NUMERIC

Important tool behaviors:
- get_pending_approvals returns ALL pending items with league_name on each. When the user asks for a specific league, call the tool and then filter/present only the matching results.
- get_team_roster searches by team name — no need to look up the team ID first.
- When approving/rejecting multiple items, call the tool once per item.
- When the user references items from a previous response (e.g. "approve Ashcraft"), use the IDs from the earlier tool call result. Do NOT ask the user to confirm the ID — just execute the action.
- Only ask for confirmation before executing WRITE operations if the request is ambiguous (e.g. multiple players with the same name). If the match is clear, proceed immediately.
- For questions about team salaries, payroll, total spending, or luxury tax, ALWAYS use the get_team_payrolls tool instead of run_query. It handles the TEXT contract column parsing correctly.
- For questions about recent trades, transactions, adds, drops, or league activity, ALWAYS use the get_recent_activity tool. Filter with action_type='TRADE' for trade history. Do NOT query the trades table with run_query for this — the transactions table is the complete activity log.
- When a user asks about player details from a trade or transaction (e.g. salary impact, contracts), use search_players to find each player mentioned in the summary, then use get_player to get their full contract details. Chain these tool calls automatically — do not ask the user for player IDs.
- For roster compliance questions (over limits, violations), use check_roster_compliance instead of run_query.
- For waiver questions (who's on waivers, waiver status), use get_waiver_status instead of run_query.
- For deadline/window questions (trade deadline, IFA window), use get_league_deadlines instead of run_query.
- To set or update league dates (opening day, trade deadline, etc.), use set_league_date. Pass league_id='all' to set for all leagues at once.
- For expiring contract questions (UFAs, last year of deal), use find_expiring_contracts instead of run_query.

Common SQL patterns for run_query (replace <league_uuid> with the actual UUID):
- Players without a team: SELECT p.first_name || ' ' || p.last_name, p.position, l.name FROM players p JOIN leagues l ON p.league_id = l.id WHERE p.team_id IS NULL AND p.league_id = '<league_uuid>' ORDER BY p.last_name LIMIT 50
- Teams with fewest players: SELECT t.name, COUNT(p.id) as cnt FROM teams t LEFT JOIN players p ON p.team_id = t.id WHERE t.league_id = '<league_uuid>' GROUP BY t.name ORDER BY cnt ASC
- Most expensive contracts: SELECT p.first_name || ' ' || p.last_name, t.name, p.contract_%d, REPLACE(REPLACE(REPLACE(COALESCE(p.contract_%d, ''), '(TO)', ''), '$', ''), ',', '')::NUMERIC as sal FROM players p JOIN teams t ON p.team_id = t.id WHERE p.league_id = '<league_uuid>' AND REPLACE(REPLACE(REPLACE(COALESCE(p.contract_%d, ''), '(TO)', ''), '$', ''), ',', '') ~ '^[0-9]+\.?[0-9]*$' ORDER BY sal DESC LIMIT 20
- Player counts by position: SELECT p.position, COUNT(*) FROM players p WHERE p.team_id IS NOT NULL AND p.league_id = '<league_uuid>' GROUP BY p.position ORDER BY count DESC
- Dead cap totals by team: SELECT t.name, SUM(dc.amount) FROM dead_cap_penalties dc JOIN teams t ON dc.team_id = t.id WHERE dc.year = %d AND t.league_id = '<league_uuid>' GROUP BY t.name ORDER BY sum DESC

Multi-step reasoning:
- When comparing trades, search_players for each player name, then get_player for full contracts on both sides. Sum dollar values to compare.
- For roster needs, use get_team_roster then analyze position distribution vs typical construction (5 SP, 3-4 RP, 8 position players on 26-man).
- For trade fairness, use get_team_payrolls for salary impact + check_roster_compliance to check limits after the trade.
- For farm system / prospect questions, use get_team_roster and filter for "Minors" status players.
- For upcoming free agents, use find_expiring_contracts with the current year.
- For complex questions, break into multiple tool calls. NEVER ask the user to run queries manually — chain tools yourself.`

// Tool definitions for Gemini function calling
func getAgentTools() []*genai.Tool {
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "search_players",
					Description: "Search for players by name. Returns up to 50 matching players with their ID, name, position, team, and league.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"name": {
								Type:        genai.TypeString,
								Description: "Player name to search for (first name, last name, or both)",
							},
						},
						Required: []string{"name"},
					},
				},
				{
					Name:        "get_player",
					Description: "Get full details for a specific player by their UUID, including contracts, status, team, and bid history.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"player_id": {
								Type:        genai.TypeString,
								Description: "Player UUID",
							},
						},
						Required: []string{"player_id"},
					},
				},
				{
					Name:        "list_teams",
					Description: "List all teams, optionally filtered by league. Returns team ID, name, owner, and league.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"league_id": {
								Type:        genai.TypeString,
								Description: "Optional league UUID to filter by. Leave empty for all leagues.",
							},
						},
					},
				},
				{
					Name:        "get_team_balance",
					Description: "Get a team's ISBP and MiLB balance by searching for the team name.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"team_name": {
								Type:        genai.TypeString,
								Description: "Team name or partial name to search for",
							},
						},
						Required: []string{"team_name"},
					},
				},
				{
					Name:        "assign_player_to_team",
					Description: "Move a player to a specific team. Sets team_id on the player record.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"player_id": {
								Type:        genai.TypeString,
								Description: "Player UUID",
							},
							"team_id": {
								Type:        genai.TypeString,
								Description: "Team UUID to assign the player to",
							},
						},
						Required: []string{"player_id", "team_id"},
					},
				},
				{
					Name:        "release_player",
					Description: "Release a player from their team (set to free agency). Clears team_id, 40-man, and 26-man status.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"player_id": {
								Type:        genai.TypeString,
								Description: "Player UUID",
							},
						},
						Required: []string{"player_id"},
					},
				},
				{
					Name:        "update_player_name",
					Description: "Update a player's first and/or last name.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"player_id": {
								Type:        genai.TypeString,
								Description: "Player UUID",
							},
							"first_name": {
								Type:        genai.TypeString,
								Description: "New first name",
							},
							"last_name": {
								Type:        genai.TypeString,
								Description: "New last name",
							},
						},
						Required: []string{"player_id", "first_name", "last_name"},
					},
				},
				{
					Name:        "update_team_balance",
					Description: "Set ISBP and/or MiLB balance for a team.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"team_id": {
								Type:        genai.TypeString,
								Description: "Team UUID",
							},
							"isbp_balance": {
								Type:        genai.TypeNumber,
								Description: "New ISBP balance (use -1 to leave unchanged)",
							},
							"milb_balance": {
								Type:        genai.TypeNumber,
								Description: "New MiLB balance (use -1 to leave unchanged)",
							},
						},
						Required: []string{"team_id", "isbp_balance", "milb_balance"},
					},
				},
				{
					Name:        "run_query",
					Description: "Run a read-only SQL SELECT query against the database. Only SELECT statements are allowed.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"sql": {
								Type:        genai.TypeString,
								Description: "SQL SELECT query to execute",
							},
						},
						Required: []string{"sql"},
					},
				},
				{
					Name:        "get_pending_approvals",
					Description: "Get all items needing commissioner approval: pending arbitration/extensions, pending trade proposals, and pending user registrations.",
					Parameters: &genai.Schema{
						Type:       genai.TypeObject,
						Properties: map[string]*genai.Schema{},
					},
				},
				{
					Name:        "get_team_roster",
					Description: "Get all players on a team's roster. Can search by team name. Returns each player's name, position, status, and contracts.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"team_name": {
								Type:        genai.TypeString,
								Description: "Team name or partial name to search for (e.g. 'Colorado Rockies', 'Rockies')",
							},
						},
						Required: []string{"team_name"},
					},
				},
				{
					Name:        "process_pending_action",
					Description: "Approve or reject a pending arbitration or extension action by its ID.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"action_id": {
								Type:        genai.TypeString,
								Description: "The pending action UUID",
							},
							"decision": {
								Type:        genai.TypeString,
								Description: "Either 'APPROVED' or 'REJECTED'",
							},
						},
						Required: []string{"action_id", "decision"},
					},
				},
				{
					Name:        "process_registration",
					Description: "Approve or deny a pending user registration request by its ID.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"request_id": {
								Type:        genai.TypeString,
								Description: "The registration request UUID",
							},
							"decision": {
								Type:        genai.TypeString,
								Description: "Either 'approve' or 'deny'",
							},
						},
						Required: []string{"request_id", "decision"},
					},
				},
				{
					Name:        "count_roster",
					Description: "Get roster counts (26-man and 40-man) for a specific team.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"team_id": {
								Type:        genai.TypeString,
								Description: "Team UUID",
							},
						},
						Required: []string{"team_id"},
					},
				},
				{
					Name:        "get_recent_activity",
					Description: "Get recent league activity (trades, adds, drops, roster moves, waivers, commissioner actions). Use this for questions about recent trades, transactions, or league activity.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"action_type": {
								Type:        genai.TypeString,
								Description: "Filter by action type: TRADE, ADD, DROP, ROSTER, WAIVER, COMMISSIONER. Leave empty for all types.",
							},
							"league_id": {
								Type:        genai.TypeString,
								Description: "Optional league UUID to filter by. Leave empty for all accessible leagues.",
							},
							"limit": {
								Type:        genai.TypeInteger,
								Description: "Number of results to return (default 20, max 50).",
							},
						},
					},
				},
				{
					Name:        "get_team_payrolls",
					Description: "Get total salary payroll for all teams in a league for a given year. Includes luxury tax comparison. Use this for questions about team salaries, payroll, or luxury tax.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"league_id": {
								Type:        genai.TypeString,
								Description: "League UUID (e.g. 11111111-1111-1111-1111-111111111111 for MLB)",
							},
							"year": {
								Type:        genai.TypeInteger,
								Description: "Contract year to sum (e.g. 2026). Defaults to current year if omitted.",
							},
						},
						Required: []string{"league_id"},
					},
				},
				{
					Name:        "check_roster_compliance",
					Description: "Check all teams in a league against roster limits (26-man, 40-man, SP count) and flag violations. Also flags teams under the 22-man minimum threshold.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"league_id": {
								Type:        genai.TypeString,
								Description: "League UUID (required)",
							},
						},
						Required: []string{"league_id"},
					},
				},
				{
					Name:        "get_waiver_status",
					Description: "Show all players currently on waivers, with waiving team, time remaining, clear action, and claiming teams.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"league_id": {
								Type:        genai.TypeString,
								Description: "Optional league UUID to filter by. Leave empty for all accessible leagues.",
							},
						},
					},
				},
				{
					Name:        "get_league_deadlines",
					Description: "Show all configured deadlines and windows (trade, extension, IFA, MiLB FA, roster expansion, option) for a league with current open/closed status.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"league_id": {
								Type:        genai.TypeString,
								Description: "League UUID (required)",
							},
							"year": {
								Type:        genai.TypeInteger,
								Description: "Year to check (defaults to current year)",
							},
						},
						Required: []string{"league_id"},
					},
				},
				{
					Name:        "find_expiring_contracts",
					Description: "Find players whose contracts expire after a given year (last year with a dollar value). Can filter by league or team.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"year": {
								Type:        genai.TypeInteger,
								Description: "The contract year to check (e.g. 2026). Finds players whose last dollar-value contract year is this year.",
							},
							"league_id": {
								Type:        genai.TypeString,
								Description: "Optional league UUID to filter by",
							},
							"team_name": {
								Type:        genai.TypeString,
								Description: "Optional team name to filter by (partial match)",
							},
						},
						Required: []string{"year"},
					},
				},
				{
					Name:        "update_player_contract",
					Description: "Set a player's contract value for a specific year. Can set dollar amounts (e.g. '$1000000'), 'TC', 'ARB', 'UFA', or empty string to clear.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"player_id": {
								Type:        genai.TypeString,
								Description: "Player UUID",
							},
							"year": {
								Type:        genai.TypeInteger,
								Description: "Contract year (2026-2040)",
							},
							"value": {
								Type:        genai.TypeString,
								Description: "Contract value: dollar amount like '$1000000', or 'TC', 'ARB', 'UFA', or '' to clear",
							},
						},
						Required: []string{"player_id", "year", "value"},
					},
				},
				{
					Name:        "add_dead_cap",
					Description: "Add a dead cap penalty to a team for a specific year.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"team_id": {
								Type:        genai.TypeString,
								Description: "Team UUID",
							},
							"player_name": {
								Type:        genai.TypeString,
								Description: "Player name for the dead cap entry (text label)",
							},
							"amount": {
								Type:        genai.TypeNumber,
								Description: "Dead cap amount in dollars (e.g. 750000)",
							},
							"year": {
								Type:        genai.TypeInteger,
								Description: "Year the dead cap applies to (e.g. 2026)",
							},
							"note": {
								Type:        genai.TypeString,
								Description: "Optional note explaining the dead cap",
							},
						},
						Required: []string{"team_id", "player_name", "amount", "year"},
					},
				},
				{
					Name:        "dfa_player",
					Description: "Designate a player for assignment (DFA). Sets 48-hour waiver period and removes from 26-man and 40-man roster.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"player_id": {
								Type:        genai.TypeString,
								Description: "Player UUID",
							},
							"clear_action": {
								Type:        genai.TypeString,
								Description: "What happens after waivers clear: 'release' (dead cap applied) or 'minors' (stays on team, off 40-man)",
							},
						},
						Required: []string{"player_id", "clear_action"},
					},
				},
				{
					Name:        "set_league_date",
					Description: "Set a league date (opening day, trade deadline, extension deadline, etc.) for a specific league and year. Can set dates for all leagues at once by passing league_id='all'.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"league_id": {
								Type:        genai.TypeString,
								Description: "League UUID, or 'all' to set for all 4 leagues (MLB, AAA, AA, High-A)",
							},
							"date_type": {
								Type:        genai.TypeString,
								Description: "Type of date: opening_day, trade_deadline, extension_deadline, option_deadline, ifa_window_open, ifa_window_close, milb_fa_window_open, milb_fa_window_close, roster_expansion_start, roster_expansion_end",
							},
							"date": {
								Type:        genai.TypeString,
								Description: "Date in YYYY-MM-DD format (e.g. '2026-03-26')",
							},
							"year": {
								Type:        genai.TypeInteger,
								Description: "Season year (defaults to current year if not provided)",
							},
						},
						Required: []string{"league_id", "date_type", "date"},
					},
				},
			},
		},
	}
}

// agentCtx holds user context for tool execution
type agentCtx struct {
	UserID    string
	LeagueIDs []string // leagues this commissioner manages
}

func (ac *agentCtx) canAccessLeague(leagueID string) bool {
	for _, id := range ac.LeagueIDs {
		if id == leagueID {
			return true
		}
	}
	return false
}

// ChatMessage represents a message in the conversation history
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AgentRequest is the POST body from the frontend
type AgentRequest struct {
	Message string        `json:"message"`
	History []ChatMessage `json:"history"`
}

// ToolCallInfo tracks what tools were called for the frontend
type ToolCallInfo struct {
	Name   string      `json:"name"`
	Args   interface{} `json:"args"`
	Result interface{} `json:"result"`
}

// AgentResponse is sent back to the frontend
type AgentResponse struct {
	Response  string         `json:"response"`
	ToolCalls []ToolCallInfo `json:"tool_calls"`
}

func AdminAgentPageHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}
		RenderTemplate(c, "admin_agent.html", gin.H{
			"User":      user,
			"IsCommish": true,
		})
	}
}

func AdminAgentChatHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Commissioner Only"})
			return
		}

		var req AgentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		if strings.TrimSpace(req.Message) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Message cannot be empty"})
			return
		}

		projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
		if projectID == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "AI agent not configured"})
			return
		}

		ctx := context.Background()
		client, err := genai.NewClient(ctx, projectID, "us-central1")
		if err != nil {
			fmt.Printf("ERROR [AdminAgentChat]: failed to create Gemini client: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to AI service"})
			return
		}
		defer client.Close()

		// Build dynamic system prompt with user's league access
		leagueNames := map[string]string{
			"11111111-1111-1111-1111-111111111111": "MLB",
			"22222222-2222-2222-2222-222222222222": "AAA",
			"33333333-3333-3333-3333-333333333333": "AA",
			"44444444-4444-4444-4444-444444444444": "High-A",
		}
		currentYear := time.Now().Year()
		prompt := fmt.Sprintf(agentSystemPromptTemplate, currentYear, currentYear)
		var names []string
		for _, lid := range adminLeagues {
			if n, ok := leagueNames[lid]; ok {
				names = append(names, n)
			}
		}
		prompt += fmt.Sprintf("\n\nThis commissioner has access to: %s. All tool results are already filtered to only show data from these leagues.", strings.Join(names, ", "))

		model := client.GenerativeModel("gemini-2.0-flash")
		model.SystemInstruction = &genai.Content{
			Parts: []genai.Part{genai.Text(prompt)},
		}
		model.Tools = getAgentTools()
		model.SetTemperature(0.1)

		chat := model.StartChat()
		chat.History = buildHistory(req.History)

		resp, err := chat.SendMessage(ctx, genai.Text(req.Message))
		if err != nil {
			fmt.Printf("ERROR [AdminAgentChat]: Gemini API error: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "AI service error"})
			return
		}

		var toolCalls []ToolCallInfo
		maxIterations := 10
		for i := 0; i < maxIterations; i++ {
			funcCalls := extractFunctionCalls(resp)
			if len(funcCalls) == 0 {
				break
			}

			var funcResponses []genai.Part
			ac := &agentCtx{
				UserID:    user.ID,
				LeagueIDs: adminLeagues,
			}
			for _, fc := range funcCalls {
				result := executeTool(db, ac, fc.Name, fc.Args)
				toolCalls = append(toolCalls, ToolCallInfo{
					Name:   fc.Name,
					Args:   fc.Args,
					Result: result,
				})
				// Serialize result to JSON string to avoid protobuf serialization issues
				// with nested []map[string]interface{} types
				jsonBytes, _ := json.Marshal(result)
				funcResponses = append(funcResponses, genai.FunctionResponse{
					Name:     fc.Name,
					Response: map[string]interface{}{"result": string(jsonBytes)},
				})
			}

			resp, err = chat.SendMessage(ctx, funcResponses...)
			if err != nil {
				fmt.Printf("ERROR [AdminAgentChat]: Gemini tool response error: %v\n", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "AI service error during tool processing"})
				return
			}
		}

		textResponse := extractText(resp)
		c.JSON(http.StatusOK, AgentResponse{
			Response:  textResponse,
			ToolCalls: toolCalls,
		})
	}
}

func buildHistory(messages []ChatMessage) []*genai.Content {
	var history []*genai.Content
	for _, msg := range messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		history = append(history, &genai.Content{
			Role:  role,
			Parts: []genai.Part{genai.Text(msg.Content)},
		})
	}
	return history
}

func extractFunctionCalls(resp *genai.GenerateContentResponse) []genai.FunctionCall {
	var calls []genai.FunctionCall
	if resp == nil {
		return calls
	}
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if fc, ok := part.(genai.FunctionCall); ok {
				calls = append(calls, fc)
			}
		}
	}
	return calls
}

func extractText(resp *genai.GenerateContentResponse) string {
	if resp == nil {
		return "No response from AI."
	}
	var texts []string
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if t, ok := part.(genai.Text); ok {
				texts = append(texts, string(t))
			}
		}
	}
	if len(texts) == 0 {
		return "No response from AI."
	}
	return strings.Join(texts, "\n")
}

func executeTool(db *pgxpool.Pool, ac *agentCtx, name string, args map[string]interface{}) map[string]interface{} {
	switch name {
	case "search_players":
		return toolSearchPlayers(db, ac, args)
	case "get_player":
		return toolGetPlayer(db, args)
	case "list_teams":
		return toolListTeams(db, ac, args)
	case "get_team_balance":
		return toolGetTeamBalance(db, ac, args)
	case "assign_player_to_team":
		return toolAssignPlayer(db, args)
	case "release_player":
		return toolReleasePlayer(db, args)
	case "update_player_name":
		return toolUpdatePlayerName(db, args)
	case "update_team_balance":
		return toolUpdateTeamBalance(db, args)
	case "run_query":
		return toolRunQuery(db, args)
	case "get_pending_approvals":
		return toolGetPendingApprovals(db, ac, args)
	case "process_pending_action":
		return toolProcessPendingAction(db, args)
	case "process_registration":
		return toolProcessRegistration(db, ac.UserID, args)
	case "get_team_roster":
		return toolGetTeamRoster(db, ac, args)
	case "count_roster":
		return toolCountRoster(db, args)
	case "get_recent_activity":
		return toolGetRecentActivity(db, ac, args)
	case "get_team_payrolls":
		return toolGetTeamPayrolls(db, ac, args)
	case "check_roster_compliance":
		return toolCheckRosterCompliance(db, ac, args)
	case "get_waiver_status":
		return toolGetWaiverStatus(db, ac, args)
	case "get_league_deadlines":
		return toolGetLeagueDeadlines(db, ac, args)
	case "find_expiring_contracts":
		return toolFindExpiringContracts(db, ac, args)
	case "update_player_contract":
		return toolUpdatePlayerContract(db, ac, args)
	case "add_dead_cap":
		return toolAddDeadCap(db, ac, args)
	case "dfa_player":
		return toolDFAPlayer(db, ac, args)
	case "set_league_date":
		return toolSetLeagueDate(db, ac, args)
	default:
		return map[string]interface{}{"error": "Unknown tool: " + name}
	}
}

func getStringArg(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getFloatArg(args map[string]interface{}, key string) float64 {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case json.Number:
			f, _ := n.Float64()
			return f
		}
	}
	return -1
}

func getIntArg(args map[string]interface{}, key string) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case json.Number:
			i, _ := n.Int64()
			return int(i)
		}
	}
	return 0
}

func toolSearchPlayers(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	name := getStringArg(args, "name")
	if name == "" {
		return map[string]interface{}{"error": "name is required"}
	}

	words := strings.Fields(name)
	if len(words) == 0 {
		return map[string]interface{}{"error": "name is required"}
	}

	conditions := make([]string, len(words))
	qargs := make([]interface{}, len(words))
	for i, word := range words {
		conditions[i] = fmt.Sprintf("(p.first_name ILIKE $%d OR p.last_name ILIKE $%d)", i+1, i+1)
		qargs[i] = "%" + word + "%"
	}

	// Filter by commissioner's leagues
	if len(ac.LeagueIDs) > 0 {
		argNum := len(qargs) + 1
		conditions = append(conditions, fmt.Sprintf("p.league_id = ANY($%d)", argNum))
		qargs = append(qargs, ac.LeagueIDs)
	}

	query := fmt.Sprintf(`
		SELECT p.id, p.first_name, p.last_name, p.position, COALESCE(p.mlb_team, ''),
		       COALESCE(t.name, ''), COALESCE(t.id::TEXT, ''), l.name
		FROM players p
		JOIN leagues l ON p.league_id = l.id
		LEFT JOIN teams t ON p.team_id = t.id
		WHERE %s
		ORDER BY l.name ASC, p.first_name ASC
		LIMIT 50
	`, strings.Join(conditions, " AND "))

	ctx := context.Background()
	rows, err := db.Query(ctx, query, qargs...)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:search_players]: %v\n", err)
		return map[string]interface{}{"error": "Search failed"}
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id, firstName, lastName, position, mlbTeam, teamName, teamID, leagueName string
		if err := rows.Scan(&id, &firstName, &lastName, &position, &mlbTeam, &teamName, &teamID, &leagueName); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"id":          id,
			"first_name":  firstName,
			"last_name":   lastName,
			"position":    position,
			"mlb_team":    mlbTeam,
			"team_name":   teamName,
			"team_id":     teamID,
			"league_name": leagueName,
		})
	}

	return map[string]interface{}{
		"count":   len(results),
		"players": results,
	}
}

func toolGetPlayer(db *pgxpool.Pool, args map[string]interface{}) map[string]interface{} {
	playerID := getStringArg(args, "player_id")
	if playerID == "" {
		return map[string]interface{}{"error": "player_id is required"}
	}

	p, err := store.GetPlayerByID(db, playerID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:get_player]: %v\n", err)
		return map[string]interface{}{"error": "Player not found"}
	}

	contracts := make(map[string]string)
	for year, val := range p.Contracts {
		if val != "" {
			contracts[fmt.Sprintf("%d", year)] = val
		}
	}

	return map[string]interface{}{
		"id":            p.ID,
		"first_name":    p.FirstName,
		"last_name":     p.LastName,
		"position":      p.Position,
		"mlb_team":      p.MLBTeam,
		"team_id":       p.TeamID,
		"league_name":   p.LeagueName,
		"status":        p.Status,
		"status_40_man": p.Status40Man,
		"status_26_man": p.Status26Man,
		"status_il":     p.StatusIL,
		"is_ifa":        p.IsIFA,
		"contracts":     contracts,
		"on_trade_block": p.OnTradeBlock,
	}
}

func toolListTeams(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	leagueID := getStringArg(args, "league_id")

	leagues, err := store.GetLeaguesWithTeams(db)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:list_teams]: %v\n", err)
		return map[string]interface{}{"error": "Failed to load teams"}
	}

	var teams []map[string]interface{}
	for _, league := range leagues {
		if leagueID != "" && league.ID != leagueID {
			continue
		}
		if !ac.canAccessLeague(league.ID) {
			continue
		}
		for _, t := range league.Teams {
			teams = append(teams, map[string]interface{}{
				"id":          t.ID,
				"name":        t.Name,
				"owner":       t.Owner,
				"league_id":   league.ID,
				"league_name": league.Name,
			})
		}
	}

	return map[string]interface{}{
		"count": len(teams),
		"teams": teams,
	}
}

func toolGetTeamBalance(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	teamName := getStringArg(args, "team_name")
	if teamName == "" {
		return map[string]interface{}{"error": "team_name is required"}
	}

	// Search across all leagues for matching teams
	balances, err := store.GetTeamsWithBalances(db, "")
	if err != nil {
		fmt.Printf("ERROR [AgentTool:get_team_balance]: %v\n", err)
		return map[string]interface{}{"error": "Failed to load balances"}
	}

	searchLower := strings.ToLower(teamName)
	var matches []map[string]interface{}
	for _, b := range balances {
		if !ac.canAccessLeague(b.LeagueID) {
			continue
		}
		if strings.Contains(strings.ToLower(b.Name), searchLower) ||
			strings.Contains(strings.ToLower(b.Abbreviation), searchLower) {
			matches = append(matches, map[string]interface{}{
				"team_id":      b.ID,
				"team_name":    b.Name,
				"abbreviation": b.Abbreviation,
				"league_name":  b.LeagueName,
				"isbp_balance": b.IsbpBalance,
				"milb_balance": b.MilbBalance,
			})
		}
	}

	if len(matches) == 0 {
		return map[string]interface{}{"error": fmt.Sprintf("No teams found matching '%s'", teamName)}
	}

	return map[string]interface{}{
		"count": len(matches),
		"teams": matches,
	}
}

func toolAssignPlayer(db *pgxpool.Pool, args map[string]interface{}) map[string]interface{} {
	playerID := getStringArg(args, "player_id")
	teamID := getStringArg(args, "team_id")
	if playerID == "" || teamID == "" {
		return map[string]interface{}{"error": "player_id and team_id are required"}
	}

	ctx := context.Background()
	_, err := db.Exec(ctx,
		"UPDATE players SET team_id = $1 WHERE id = $2",
		teamID, playerID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:assign_player]: %v\n", err)
		return map[string]interface{}{"error": "Failed to assign player"}
	}

	fmt.Printf("AGENT ACTION: Assigned player %s to team %s\n", playerID, teamID)
	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Player %s assigned to team %s", playerID, teamID),
	}
}

func toolReleasePlayer(db *pgxpool.Pool, args map[string]interface{}) map[string]interface{} {
	playerID := getStringArg(args, "player_id")
	if playerID == "" {
		return map[string]interface{}{"error": "player_id is required"}
	}

	ctx := context.Background()
	_, err := db.Exec(ctx,
		"UPDATE players SET team_id = NULL, status_40_man = FALSE, status_26_man = FALSE WHERE id = $1",
		playerID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:release_player]: %v\n", err)
		return map[string]interface{}{"error": "Failed to release player"}
	}

	fmt.Printf("AGENT ACTION: Released player %s to free agency\n", playerID)
	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Player %s released to free agency", playerID),
	}
}

func toolUpdatePlayerName(db *pgxpool.Pool, args map[string]interface{}) map[string]interface{} {
	playerID := getStringArg(args, "player_id")
	firstName := getStringArg(args, "first_name")
	lastName := getStringArg(args, "last_name")
	if playerID == "" || firstName == "" || lastName == "" {
		return map[string]interface{}{"error": "player_id, first_name, and last_name are required"}
	}

	ctx := context.Background()
	_, err := db.Exec(ctx,
		"UPDATE players SET first_name = $1, last_name = $2 WHERE id = $3",
		firstName, lastName, playerID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:update_player_name]: %v\n", err)
		return map[string]interface{}{"error": "Failed to update player name"}
	}

	fmt.Printf("AGENT ACTION: Updated player %s name to %s %s\n", playerID, firstName, lastName)
	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Player name updated to %s %s", firstName, lastName),
	}
}

func toolUpdateTeamBalance(db *pgxpool.Pool, args map[string]interface{}) map[string]interface{} {
	teamID := getStringArg(args, "team_id")
	isbp := getFloatArg(args, "isbp_balance")
	milb := getFloatArg(args, "milb_balance")
	if teamID == "" {
		return map[string]interface{}{"error": "team_id is required"}
	}

	ctx := context.Background()
	// Get current balances first
	var currentISBP, currentMiLB float64
	err := db.QueryRow(ctx,
		"SELECT COALESCE(isbp_balance, 0), COALESCE(milb_balance, 0) FROM teams WHERE id = $1",
		teamID).Scan(&currentISBP, &currentMiLB)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:update_team_balance]: %v\n", err)
		return map[string]interface{}{"error": "Team not found"}
	}

	newISBP := currentISBP
	newMiLB := currentMiLB
	if isbp >= 0 {
		newISBP = isbp
	}
	if milb >= 0 {
		newMiLB = milb
	}

	err = store.SetTeamBalance(db, teamID, newISBP, newMiLB)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:update_team_balance]: %v\n", err)
		return map[string]interface{}{"error": "Failed to update balance"}
	}

	fmt.Printf("AGENT ACTION: Updated team %s balances: ISBP=%.2f, MiLB=%.2f\n", teamID, newISBP, newMiLB)
	return map[string]interface{}{
		"success":      true,
		"isbp_balance": newISBP,
		"milb_balance": newMiLB,
	}
}

func toolRunQuery(db *pgxpool.Pool, args map[string]interface{}) map[string]interface{} {
	sql := getStringArg(args, "sql")
	if sql == "" {
		return map[string]interface{}{"error": "sql is required"}
	}

	// Safety: only allow SELECT statements
	trimmed := strings.TrimSpace(strings.ToUpper(sql))
	if !strings.HasPrefix(trimmed, "SELECT") {
		return map[string]interface{}{"error": "Only SELECT queries are allowed"}
	}

	// Block dangerous keywords using word boundary matching
	// (simple Contains would false-positive on column names like created_at, updated_at)
	blocked := []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "TRUNCATE", "CREATE", "GRANT", "REVOKE", "EXEC"}
	for _, kw := range blocked {
		pattern := `(?i)\b` + kw + `\b`
		if matched, _ := regexp.MatchString(pattern, trimmed); matched {
			// Allow column names that contain the keyword as a prefix (e.g., created_at, updated_at, deleted_at)
			colPattern := `(?i)\b` + kw + `[A-Z_]`
			if colMatch, _ := regexp.MatchString(colPattern, trimmed); colMatch {
				continue
			}
			return map[string]interface{}{"error": fmt.Sprintf("Query contains blocked keyword: %s", kw)}
		}
	}

	ctx := context.Background()
	rows, err := db.Query(ctx, sql)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:run_query]: %v\n", err)
		return map[string]interface{}{"error": "Query execution failed: " + err.Error()}
	}
	defer rows.Close()

	columns := rows.FieldDescriptions()
	colNames := make([]string, len(columns))
	for i, col := range columns {
		colNames[i] = string(col.Name)
	}

	var results []map[string]interface{}
	rowCount := 0
	maxRows := 100
	for rows.Next() {
		if rowCount >= maxRows {
			break
		}
		values, err := rows.Values()
		if err != nil {
			continue
		}
		row := make(map[string]interface{})
		for i, col := range colNames {
			row[col] = values[i]
		}
		results = append(results, row)
		rowCount++
	}

	return map[string]interface{}{
		"columns":    colNames,
		"rows":       results,
		"row_count":  rowCount,
		"truncated":  rowCount >= maxRows,
	}
}

func toolGetPendingApprovals(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}

	// 1. Pending actions (arbitration/extensions) — filtered by commissioner's leagues
	ctx := context.Background()
	actionRows, err := db.Query(ctx, `
		SELECT pa.id, p.first_name || ' ' || p.last_name, t.name, l.name,
		       pa.action_type, pa.target_year, pa.salary_amount
		FROM pending_actions pa
		JOIN players p ON pa.player_id = p.id
		JOIN teams t ON pa.team_id = t.id
		JOIN leagues l ON pa.league_id = l.id
		WHERE pa.status = 'PENDING' AND pa.league_id = ANY($1)
		ORDER BY l.name, pa.created_at ASC`, ac.LeagueIDs)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:get_pending_approvals]: actions: %v\n", err)
	}
	var actionList []map[string]interface{}
	if actionRows != nil {
		for actionRows.Next() {
			var id, playerName, teamName, leagueName, actionType string
			var targetYear int
			var salary float64
			actionRows.Scan(&id, &playerName, &teamName, &leagueName, &actionType, &targetYear, &salary)
			actionList = append(actionList, map[string]interface{}{
				"id":            id,
				"type":          actionType,
				"player_name":   playerName,
				"team_name":     teamName,
				"league_name":   leagueName,
				"target_year":   targetYear,
				"salary_amount": salary,
			})
		}
		actionRows.Close()
	}
	result["pending_actions"] = actionList
	result["pending_actions_count"] = len(actionList)

	// 2. Pending registrations
	regs, err := store.GetPendingRegistrations(db)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:get_pending_approvals]: registrations: %v\n", err)
	}
	var regList []map[string]interface{}
	for _, r := range regs {
		regList = append(regList, map[string]interface{}{
			"id":       r.ID,
			"username": r.Username,
			"email":    r.Email,
		})
	}
	result["pending_registrations"] = regList
	result["pending_registrations_count"] = len(regList)

	// 3. Pending trades — filtered by commissioner's leagues
	rows, err := db.Query(ctx, `
		SELECT t.id, tp.name, tr.name, t.created_at,
		       COALESCE(t.isbp_offered, 0), COALESCE(t.isbp_requested, 0)
		FROM trades t
		JOIN teams tp ON t.proposing_team_id = tp.id
		JOIN teams tr ON t.receiving_team_id = tr.id
		WHERE t.status = 'PENDING' AND t.league_id = ANY($1)
		ORDER BY t.created_at ASC`, ac.LeagueIDs)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:get_pending_approvals]: trades: %v\n", err)
	} else {
		var tradeList []map[string]interface{}
		for rows.Next() {
			var id, proposer, receiver, createdAt string
			var isbpOff, isbpReq float64
			rows.Scan(&id, &proposer, &receiver, &createdAt, &isbpOff, &isbpReq)
			tradeList = append(tradeList, map[string]interface{}{
				"id":             id,
				"proposing_team": proposer,
				"receiving_team": receiver,
				"created_at":     createdAt,
				"isbp_offered":   isbpOff,
				"isbp_requested": isbpReq,
			})
		}
		rows.Close()
		result["pending_trades"] = tradeList
		result["pending_trades_count"] = len(tradeList)
	}

	return result
}

func toolProcessPendingAction(db *pgxpool.Pool, args map[string]interface{}) map[string]interface{} {
	actionID := getStringArg(args, "action_id")
	decision := getStringArg(args, "decision")
	if actionID == "" || decision == "" {
		return map[string]interface{}{"error": "action_id and decision are required"}
	}
	decision = strings.ToUpper(decision)
	if decision != "APPROVED" && decision != "REJECTED" {
		return map[string]interface{}{"error": "decision must be 'APPROVED' or 'REJECTED'"}
	}

	err := store.ProcessAction(db, actionID, decision)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:process_pending_action]: %v\n", err)
		return map[string]interface{}{"error": "Failed to process action"}
	}

	fmt.Printf("AGENT ACTION: %s pending action %s\n", decision, actionID)
	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Action %s: %s", actionID, decision),
	}
}

func toolProcessRegistration(db *pgxpool.Pool, userID string, args map[string]interface{}) map[string]interface{} {
	requestID := getStringArg(args, "request_id")
	decision := getStringArg(args, "decision")
	if requestID == "" || decision == "" {
		return map[string]interface{}{"error": "request_id and decision are required"}
	}
	decision = strings.ToLower(decision)
	if decision != "approve" && decision != "deny" {
		return map[string]interface{}{"error": "decision must be 'approve' or 'deny'"}
	}

	var err error
	if decision == "approve" {
		err = store.ApproveRegistration(db, requestID, userID)
	} else {
		err = store.DenyRegistration(db, requestID, userID)
	}
	if err != nil {
		fmt.Printf("ERROR [AgentTool:process_registration]: %v\n", err)
		return map[string]interface{}{"error": "Failed to process registration"}
	}

	fmt.Printf("AGENT ACTION: %sd registration %s\n", decision, requestID)
	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Registration %s: %sd", requestID, decision),
	}
}

func toolGetTeamRoster(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	teamName := getStringArg(args, "team_name")
	if teamName == "" {
		return map[string]interface{}{"error": "team_name is required"}
	}

	ctx := context.Background()

	// Find matching team(s) — filtered by commissioner's leagues
	teamRows, err := db.Query(ctx,
		`SELECT t.id, t.name, l.name FROM teams t JOIN leagues l ON t.league_id = l.id
		 WHERE t.name ILIKE $1 AND t.league_id = ANY($2) ORDER BY l.name`, "%"+teamName+"%", ac.LeagueIDs)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:get_team_roster]: %v\n", err)
		return map[string]interface{}{"error": "Failed to search teams"}
	}

	type teamMatch struct {
		ID, Name, League string
	}
	var teams []teamMatch
	for teamRows.Next() {
		var t teamMatch
		teamRows.Scan(&t.ID, &t.Name, &t.League)
		teams = append(teams, t)
	}
	teamRows.Close()

	if len(teams) == 0 {
		return map[string]interface{}{"error": fmt.Sprintf("No teams found matching '%s'", teamName)}
	}

	// Get roster for each matching team
	var allTeams []map[string]interface{}
	for _, t := range teams {
		rows, err := db.Query(ctx,
			`SELECT p.id, p.first_name, p.last_name, p.position, COALESCE(p.mlb_team, ''),
			        p.status_40_man, p.status_26_man, COALESCE(p.status_il, ''),
			        COALESCE(p.contract_2026, ''), COALESCE(p.contract_2027, ''),
			        COALESCE(p.contract_2028, ''), COALESCE(p.contract_2029, ''),
			        COALESCE(p.contract_2030, '')
			 FROM players p WHERE p.team_id = $1
			 ORDER BY p.status_26_man DESC, p.status_40_man DESC, p.position, p.last_name`, t.ID)
		if err != nil {
			continue
		}

		var players []map[string]interface{}
		for rows.Next() {
			var id, firstName, lastName, pos, mlbTeam, statusIL string
			var c26, c27, c28, c29, c30 string
			var on40, on26 bool
			rows.Scan(&id, &firstName, &lastName, &pos, &mlbTeam, &on40, &on26, &statusIL,
				&c26, &c27, &c28, &c29, &c30)

			status := "Minors (Non-40)"
			if statusIL != "" {
				status = statusIL
			} else if on26 {
				status = "Active (26-Man)"
			} else if on40 {
				status = "40-Man (Minors)"
			}

			player := map[string]interface{}{
				"id":        id,
				"name":      firstName + " " + lastName,
				"position":  pos,
				"mlb_team":  mlbTeam,
				"status":    status,
			}
			if c26 != "" {
				player["contract_2026"] = c26
			}
			if c27 != "" {
				player["contract_2027"] = c27
			}
			if c28 != "" {
				player["contract_2028"] = c28
			}
			if c29 != "" {
				player["contract_2029"] = c29
			}
			if c30 != "" {
				player["contract_2030"] = c30
			}
			players = append(players, player)
		}
		rows.Close()

		allTeams = append(allTeams, map[string]interface{}{
			"team_id":      t.ID,
			"team_name":    t.Name,
			"league_name":  t.League,
			"player_count": len(players),
			"players":      players,
		})
	}

	return map[string]interface{}{
		"teams_found": len(allTeams),
		"teams":       allTeams,
	}
}

func toolGetRecentActivity(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	actionType := strings.ToUpper(getStringArg(args, "action_type"))
	leagueID := getStringArg(args, "league_id")
	limit := int(getFloatArg(args, "limit"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	ctx := context.Background()
	queryArgs := []interface{}{ac.LeagueIDs}
	// Match transactions by their own league_id OR by the team's league_id (for NULL league_id rows)
	conditions := []string{"(t.league_id = ANY($1) OR tm.league_id = ANY($1))"}

	if leagueID != "" {
		if !ac.canAccessLeague(leagueID) {
			return map[string]interface{}{"error": "You don't have access to this league"}
		}
		conditions = append(conditions, fmt.Sprintf("(t.league_id = $%d OR tm.league_id = $%d)", len(queryArgs)+1, len(queryArgs)+1))
		queryArgs = append(queryArgs, leagueID)
	}

	if actionType != "" {
		conditions = append(conditions, fmt.Sprintf("t.transaction_type = $%d", len(queryArgs)+1))
		queryArgs = append(queryArgs, actionType)
	}

	queryArgs = append(queryArgs, limit)
	// Use COALESCE to fall back to team's league_id when transaction league_id is NULL
	// (TRADE transactions from WordPress sync have NULL league_id)
	query := fmt.Sprintf(`
		SELECT t.id, t.transaction_type, COALESCE(t.summary, ''), t.created_at,
		       COALESCE(tm.name, ''), COALESCE(l.name, tl.name, ''),
		       COALESCE(t.player_id::TEXT, '')
		FROM transactions t
		LEFT JOIN teams tm ON t.team_id = tm.id
		LEFT JOIN leagues l ON t.league_id = l.id
		LEFT JOIN leagues tl ON tm.league_id = tl.id
		WHERE %s
		ORDER BY t.created_at DESC
		LIMIT $%d`, strings.Join(conditions, " AND "), len(queryArgs))

	rows, err := db.Query(ctx, query, queryArgs...)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:get_recent_activity]: %v\n", err)
		return map[string]interface{}{"error": "Failed to query activity"}
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id, txnType, summary, teamName, leagueName, playerID string
		var createdAt time.Time
		if err := rows.Scan(&id, &txnType, &summary, &createdAt, &teamName, &leagueName, &playerID); err != nil {
			fmt.Printf("ERROR [AgentTool:get_recent_activity]: scan: %v\n", err)
			continue
		}
		entry := map[string]interface{}{
			"id":               id,
			"transaction_type": txnType,
			"summary":          summary,
			"created_at":       createdAt.Format("2006-01-02 3:04 PM"),
			"team_name":        teamName,
			"league_name":      leagueName,
		}
		if playerID != "" {
			entry["player_id"] = playerID
		}
		results = append(results, entry)
	}

	return map[string]interface{}{
		"count":        len(results),
		"transactions": results,
	}
}

func toolGetTeamPayrolls(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	leagueID := getStringArg(args, "league_id")
	if leagueID == "" {
		return map[string]interface{}{"error": "league_id is required"}
	}
	if !ac.canAccessLeague(leagueID) {
		return map[string]interface{}{"error": "You don't have access to this league"}
	}

	year := int(getFloatArg(args, "year"))
	if year <= 0 {
		year = time.Now().Year()
	}

	// Validate year is in contract column range
	if year < 2026 || year > 2040 {
		return map[string]interface{}{"error": fmt.Sprintf("Year %d is outside the contract range (2026-2040)", year)}
	}

	contractCol := fmt.Sprintf("contract_%d", year)
	ctx := context.Background()

	// Sum salary for each team, parsing the TEXT contract column
	// Contract values can be: "$1000000", "$1,500,000", "1000000", "1000000(TO)"
	// Non-numeric values: "TC", "ARB", "UFA", "" — excluded via regex after cleanup
	// Pattern: strip (TO), $, commas first, then check if result is numeric
	query := fmt.Sprintf(`
		SELECT t.id, t.name,
		       COALESCE(SUM(
		           CASE WHEN REPLACE(REPLACE(REPLACE(COALESCE(p.%s, ''), '(TO)', ''), '$', ''), ',', '') ~ '^[0-9]+\.?[0-9]*$'
		                THEN REPLACE(REPLACE(REPLACE(p.%s, '(TO)', ''), '$', ''), ',', '')::NUMERIC
		                ELSE 0 END
		       ), 0) as total_salary,
		       COUNT(CASE WHEN REPLACE(REPLACE(REPLACE(COALESCE(p.%s, ''), '(TO)', ''), '$', ''), ',', '') ~ '^[0-9]+\.?[0-9]*$' THEN 1 END) as players_with_salary,
		       COUNT(CASE WHEN UPPER(COALESCE(p.%s, '')) = 'ARB' THEN 1 END) as arb_players,
		       COUNT(CASE WHEN UPPER(COALESCE(p.%s, '')) = 'TC' THEN 1 END) as tc_players
		FROM teams t
		LEFT JOIN players p ON p.team_id = t.id
		WHERE t.league_id = $1
		GROUP BY t.id, t.name
		ORDER BY total_salary DESC`,
		contractCol, contractCol, contractCol, contractCol, contractCol)

	rows, err := db.Query(ctx, query, leagueID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:get_team_payrolls]: %v\n", err)
		return map[string]interface{}{"error": "Failed to query payrolls"}
	}
	defer rows.Close()

	var teams []map[string]interface{}
	for rows.Next() {
		var teamID, teamName string
		var totalSalary float64
		var salaryCount, arbCount, tcCount int
		if err := rows.Scan(&teamID, &teamName, &totalSalary, &salaryCount, &arbCount, &tcCount); err != nil {
			fmt.Printf("ERROR [AgentTool:get_team_payrolls]: scan: %v\n", err)
			continue
		}
		teams = append(teams, map[string]interface{}{
			"team_id":             teamID,
			"team_name":           teamName,
			"total_salary":        totalSalary,
			"players_with_salary": salaryCount,
			"arb_players":         arbCount,
			"tc_players":          tcCount,
		})
	}

	// Get luxury tax limit for this league/year
	var luxuryTaxLimit float64
	err = db.QueryRow(ctx,
		"SELECT COALESCE(luxury_tax_limit, 0) FROM league_settings WHERE league_id = $1 AND year = $2",
		leagueID, year).Scan(&luxuryTaxLimit)
	if err != nil {
		luxuryTaxLimit = 0
	}

	// Get dead cap penalties per team for this year
	deadCapRows, err := db.Query(ctx,
		`SELECT team_id, COALESCE(SUM(amount), 0)
		 FROM dead_cap_penalties
		 WHERE year = $1 AND team_id IN (SELECT id FROM teams WHERE league_id = $2)
		 GROUP BY team_id`, year, leagueID)
	deadCapMap := map[string]float64{}
	if err == nil {
		for deadCapRows.Next() {
			var tid string
			var amt float64
			deadCapRows.Scan(&tid, &amt)
			deadCapMap[tid] = amt
		}
		deadCapRows.Close()
	}

	// Add dead cap to team results and mark over-tax
	for _, t := range teams {
		tid := t["team_id"].(string)
		dc := deadCapMap[tid]
		t["dead_cap"] = dc
		t["total_with_dead_cap"] = t["total_salary"].(float64) + dc
		if luxuryTaxLimit > 0 {
			t["over_luxury_tax"] = (t["total_salary"].(float64) + dc) > luxuryTaxLimit
		}
	}

	result := map[string]interface{}{
		"year":       year,
		"team_count": len(teams),
		"teams":      teams,
	}
	if luxuryTaxLimit > 0 {
		result["luxury_tax_limit"] = luxuryTaxLimit
	}

	return result
}

func toolCountRoster(db *pgxpool.Pool, args map[string]interface{}) map[string]interface{} {
	teamID := getStringArg(args, "team_id")
	if teamID == "" {
		return map[string]interface{}{"error": "team_id is required"}
	}

	count26, count40, err := store.GetTeamRosterCounts(db, teamID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:count_roster]: %v\n", err)
		return map[string]interface{}{"error": "Failed to get roster counts"}
	}

	spCount, _ := store.GetTeam26ManSPCount(db, teamID)

	return map[string]interface{}{
		"team_id":    teamID,
		"count_26":   count26,
		"count_40":   count40,
		"sp_on_26":   spCount,
	}
}

func toolCheckRosterCompliance(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	leagueID := getStringArg(args, "league_id")
	if leagueID == "" {
		return map[string]interface{}{"error": "league_id is required"}
	}
	if !ac.canAccessLeague(leagueID) {
		return map[string]interface{}{"error": "You don't have access to this league"}
	}

	year := time.Now().Year()
	settings := store.GetLeagueSettings(db, leagueID, year)

	ctx := context.Background()
	teamRows, err := db.Query(ctx, "SELECT id, name FROM teams WHERE league_id = $1 ORDER BY name", leagueID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:check_roster_compliance]: %v\n", err)
		return map[string]interface{}{"error": "Failed to query teams"}
	}

	type teamInfo struct{ ID, Name string }
	var teams []teamInfo
	for teamRows.Next() {
		var t teamInfo
		teamRows.Scan(&t.ID, &t.Name)
		teams = append(teams, t)
	}
	teamRows.Close()

	var results []map[string]interface{}
	violationCount := 0
	for _, t := range teams {
		count26, count40, _ := store.GetTeamRosterCounts(db, t.ID)
		spCount, _ := store.GetTeam26ManSPCount(db, t.ID)

		over26 := count26 > settings.Roster26ManLimit
		over40 := count40 > settings.Roster40ManLimit
		overSP := spCount > settings.SP26ManLimit
		under22 := count26 < 22

		entry := map[string]interface{}{
			"team_name":     t.Name,
			"count_26":      count26,
			"limit_26":      settings.Roster26ManLimit,
			"over_26":       over26,
			"under_22":      under22,
			"count_40":      count40,
			"limit_40":      settings.Roster40ManLimit,
			"over_40":       over40,
			"sp_on_26":      spCount,
			"sp_limit":      settings.SP26ManLimit,
			"over_sp_limit": overSP,
		}
		if over26 || over40 || overSP || under22 {
			entry["has_violation"] = true
			violationCount++
		}
		results = append(results, entry)
	}

	return map[string]interface{}{
		"teams":           results,
		"team_count":      len(results),
		"violation_count": violationCount,
	}
}

func toolGetWaiverStatus(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	leagueID := getStringArg(args, "league_id")
	if leagueID != "" && !ac.canAccessLeague(leagueID) {
		return map[string]interface{}{"error": "You don't have access to this league"}
	}

	ctx := context.Background()
	query := `
		SELECT p.id, p.first_name, p.last_name, p.position,
		       COALESCE(l.name, 'Unknown'), COALESCE(wt.name, 'Unknown'),
		       COALESCE(p.waiver_end_time, NOW()), COALESCE(p.dfa_clear_action, 'release')
		FROM players p
		LEFT JOIN leagues l ON p.league_id = l.id
		LEFT JOIN teams wt ON p.waiving_team_id = wt.id
		WHERE p.fa_status = 'on waivers'`
	queryArgs := []interface{}{}

	if leagueID != "" {
		query += " AND p.league_id = $1"
		queryArgs = append(queryArgs, leagueID)
	} else {
		query += " AND p.league_id = ANY($1)"
		queryArgs = append(queryArgs, ac.LeagueIDs)
	}
	query += " ORDER BY p.waiver_end_time ASC"

	rows, err := db.Query(ctx, query, queryArgs...)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:get_waiver_status]: %v\n", err)
		return map[string]interface{}{"error": "Failed to query waivers"}
	}

	type waiverPlayer struct {
		entry    map[string]interface{}
		playerID string
	}
	var results []waiverPlayer
	for rows.Next() {
		var id, firstName, lastName, pos, leagueName, waivingTeam, clearAction string
		var endTime time.Time
		if err := rows.Scan(&id, &firstName, &lastName, &pos,
			&leagueName, &waivingTeam, &endTime, &clearAction); err != nil {
			continue
		}

		entry := map[string]interface{}{
			"player_id":    id,
			"player_name":  firstName + " " + lastName,
			"position":     pos,
			"league_name":  leagueName,
			"waiving_team": waivingTeam,
			"clear_action": clearAction,
			"waiver_end":   endTime.Format("2006-01-02 3:04 PM"),
		}

		if time.Now().After(endTime) {
			entry["expired"] = true
			entry["time_remaining"] = "Expired"
		} else {
			remaining := time.Until(endTime)
			hours := int(remaining.Hours())
			minutes := int(remaining.Minutes()) % 60
			entry["time_remaining"] = fmt.Sprintf("%dh %dm", hours, minutes)
			entry["expired"] = false
		}

		results = append(results, waiverPlayer{entry: entry, playerID: id})
	}
	rows.Close()

	// Second pass: fetch claiming teams (rows already closed to avoid deadlock)
	var finalResults []map[string]interface{}
	for _, wp := range results {
		claimRows, err := db.Query(ctx, `
			SELECT COALESCE(t.name, 'Unknown')
			FROM waiver_claims wc
			JOIN teams t ON wc.team_id = t.id
			WHERE wc.player_id = $1 AND wc.status = 'pending'
			ORDER BY wc.claim_priority ASC`, wp.playerID)
		if err == nil {
			var claimingTeams []string
			for claimRows.Next() {
				var name string
				claimRows.Scan(&name)
				claimingTeams = append(claimingTeams, name)
			}
			claimRows.Close()
			wp.entry["claiming_teams"] = claimingTeams
			wp.entry["claim_count"] = len(claimingTeams)
		}
		finalResults = append(finalResults, wp.entry)
	}

	return map[string]interface{}{
		"count":   len(finalResults),
		"players": finalResults,
	}
}

func toolGetLeagueDeadlines(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	leagueID := getStringArg(args, "league_id")
	if leagueID == "" {
		return map[string]interface{}{"error": "league_id is required"}
	}
	if !ac.canAccessLeague(leagueID) {
		return map[string]interface{}{"error": "You don't have access to this league"}
	}

	year := int(getFloatArg(args, "year"))
	if year <= 0 {
		year = time.Now().Year()
	}

	dateTypes := []string{
		"trade_deadline", "opening_day", "extension_deadline", "option_deadline",
		"ifa_window_open", "ifa_window_close",
		"milb_fa_window_open", "milb_fa_window_close",
		"roster_expansion_start", "roster_expansion_end",
	}

	now := time.Now()
	var deadlines []map[string]interface{}
	for _, dt := range dateTypes {
		dateVal, err := store.GetLeagueDateValue(db, leagueID, year, dt)
		entry := map[string]interface{}{"date_type": dt}
		if err != nil {
			entry["configured"] = false
		} else {
			entry["configured"] = true
			entry["date"] = dateVal.Format("2006-01-02")
			entry["is_past"] = now.After(dateVal)
		}
		deadlines = append(deadlines, entry)
	}

	// Check window statuses
	windows := map[string][2]string{
		"ifa_window":       {"ifa_window_open", "ifa_window_close"},
		"milb_fa_window":   {"milb_fa_window_open", "milb_fa_window_close"},
		"roster_expansion": {"roster_expansion_start", "roster_expansion_end"},
	}

	windowStatuses := map[string]interface{}{}
	for name, pair := range windows {
		isOpen, msg := store.IsWithinDateWindow(db, leagueID, year, pair[0], pair[1])
		windowStatuses[name] = map[string]interface{}{
			"is_open": isOpen,
			"message": msg,
		}
	}

	return map[string]interface{}{
		"league_id": leagueID,
		"year":      year,
		"deadlines": deadlines,
		"windows":   windowStatuses,
	}
}

func toolFindExpiringContracts(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	year := int(getFloatArg(args, "year"))
	if year < 2026 || year > 2040 {
		return map[string]interface{}{"error": "year must be between 2026 and 2040"}
	}
	leagueID := getStringArg(args, "league_id")
	teamName := getStringArg(args, "team_name")

	if leagueID != "" && !ac.canAccessLeague(leagueID) {
		return map[string]interface{}{"error": "You don't have access to this league"}
	}

	contractCol := fmt.Sprintf("contract_%d", year)

	// Player has a dollar value in target year
	conditions := []string{
		fmt.Sprintf(`REPLACE(REPLACE(REPLACE(COALESCE(p.%s, ''), '(TO)', ''), '$', ''), ',', '') ~ '^[0-9]+\.?[0-9]*$'`, contractCol),
		"p.team_id IS NOT NULL",
		"p.league_id = ANY($1)",
	}
	queryArgs := []interface{}{ac.LeagueIDs}
	argIdx := 2

	// No dollar contract the year after
	if year < 2040 {
		nextCol := fmt.Sprintf("contract_%d", year+1)
		conditions = append(conditions,
			fmt.Sprintf(`(COALESCE(p.%s, '') = '' OR UPPER(COALESCE(p.%s, '')) IN ('UFA', 'TC', 'ARB'))`, nextCol, nextCol))
	}

	if leagueID != "" {
		conditions = append(conditions, fmt.Sprintf("p.league_id = $%d", argIdx))
		queryArgs = append(queryArgs, leagueID)
		argIdx++
	}

	if teamName != "" {
		conditions = append(conditions, fmt.Sprintf("t.name ILIKE $%d", argIdx))
		queryArgs = append(queryArgs, "%"+teamName+"%")
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT p.id, p.first_name, p.last_name, p.position,
		       COALESCE(t.name, 'No Team'), COALESCE(l.name, ''),
		       COALESCE(p.%s, '')
		FROM players p
		LEFT JOIN teams t ON p.team_id = t.id
		JOIN leagues l ON p.league_id = l.id
		WHERE %s
		ORDER BY l.name, t.name, p.last_name
		LIMIT 100`, contractCol, strings.Join(conditions, " AND "))

	ctx := context.Background()
	rows, err := db.Query(ctx, query, queryArgs...)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:find_expiring_contracts]: %v\n", err)
		return map[string]interface{}{"error": "Query failed"}
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id, firstName, lastName, pos, team, league, contractVal string
		if err := rows.Scan(&id, &firstName, &lastName, &pos, &team, &league, &contractVal); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"player_id":      id,
			"player_name":    firstName + " " + lastName,
			"position":       pos,
			"team_name":      team,
			"league_name":    league,
			"contract_value": contractVal,
			"expiring_year":  year,
		})
	}

	return map[string]interface{}{
		"year":    year,
		"count":   len(results),
		"players": results,
	}
}

func toolUpdatePlayerContract(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	playerID := getStringArg(args, "player_id")
	year := int(getFloatArg(args, "year"))
	value := getStringArg(args, "value")

	if playerID == "" {
		return map[string]interface{}{"error": "player_id is required"}
	}
	if year < 2026 || year > 2040 {
		return map[string]interface{}{"error": "year must be between 2026 and 2040"}
	}

	ctx := context.Background()
	var playerLeague string
	err := db.QueryRow(ctx,
		"SELECT COALESCE(league_id::TEXT, '') FROM players WHERE id = $1", playerID).Scan(&playerLeague)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:update_player_contract]: %v\n", err)
		return map[string]interface{}{"error": "Player not found"}
	}
	if !ac.canAccessLeague(playerLeague) {
		return map[string]interface{}{"error": "Player is not in your managed leagues"}
	}

	contractCol := fmt.Sprintf("contract_%d", year)
	query := fmt.Sprintf("UPDATE players SET %s = $1 WHERE id = $2", contractCol)

	var dbValue *string
	if value != "" {
		dbValue = &value
	}

	_, err = db.Exec(ctx, query, dbValue, playerID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:update_player_contract]: %v\n", err)
		return map[string]interface{}{"error": "Failed to update contract"}
	}

	fmt.Printf("AGENT ACTION: Updated player %s contract_%d = %s\n", playerID, year, value)
	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Contract for %d set to '%s'", year, value),
	}
}

func toolAddDeadCap(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	teamID := getStringArg(args, "team_id")
	playerName := getStringArg(args, "player_name")
	amount := getFloatArg(args, "amount")
	year := int(getFloatArg(args, "year"))
	note := getStringArg(args, "note")

	if teamID == "" || playerName == "" {
		return map[string]interface{}{"error": "team_id and player_name are required"}
	}
	if amount <= 0 {
		return map[string]interface{}{"error": "amount must be positive"}
	}
	if year < 2026 || year > 2040 {
		return map[string]interface{}{"error": "year must be between 2026 and 2040"}
	}

	teamLeague, err := store.GetTeamLeagueID(db, teamID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:add_dead_cap]: %v\n", err)
		return map[string]interface{}{"error": "Team not found"}
	}
	if !ac.canAccessLeague(teamLeague) {
		return map[string]interface{}{"error": "Team is not in your managed leagues"}
	}

	if note == "" {
		note = fmt.Sprintf("Agent: dead cap for %s", playerName)
	}

	err = store.AddDeadCapPenalty(db, teamID, "", amount, year, note)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:add_dead_cap]: %v\n", err)
		return map[string]interface{}{"error": "Failed to add dead cap"}
	}

	fmt.Printf("AGENT ACTION: Added dead cap $%.0f for %s on team %s year %d\n", amount, playerName, teamID, year)
	return map[string]interface{}{
		"success":     true,
		"player_name": playerName,
		"amount":      amount,
		"year":        year,
		"note":        note,
	}
}

func toolDFAPlayer(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	playerID := getStringArg(args, "player_id")
	clearAction := getStringArg(args, "clear_action")

	if playerID == "" {
		return map[string]interface{}{"error": "player_id is required"}
	}
	if clearAction != "release" && clearAction != "minors" {
		return map[string]interface{}{"error": "clear_action must be 'release' or 'minors'"}
	}

	ctx := context.Background()
	var teamID, playerLeague, firstName, lastName string
	err := db.QueryRow(ctx,
		`SELECT COALESCE(team_id::TEXT, ''), COALESCE(league_id::TEXT, ''),
		        first_name, last_name
		 FROM players WHERE id = $1`, playerID).Scan(&teamID, &playerLeague, &firstName, &lastName)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:dfa_player]: %v\n", err)
		return map[string]interface{}{"error": "Player not found"}
	}
	if teamID == "" {
		return map[string]interface{}{"error": "Player is not on a team"}
	}
	if !ac.canAccessLeague(playerLeague) {
		return map[string]interface{}{"error": "Player is not in your managed leagues"}
	}

	waiverEnd := time.Now().Add(48 * time.Hour)

	_, err = db.Exec(ctx,
		`UPDATE players SET
			fa_status = 'on waivers',
			waiver_end_time = $1,
			waiving_team_id = $2,
			dfa_clear_action = $3,
			status_26_man = FALSE,
			status_40_man = FALSE
		WHERE id = $4 AND team_id = $2`,
		waiverEnd, teamID, clearAction, playerID)
	if err != nil {
		fmt.Printf("ERROR [AgentTool:dfa_player]: %v\n", err)
		return map[string]interface{}{"error": "Failed to DFA player"}
	}

	store.AppendRosterMove(db, playerID, teamID, "Designated for Assignment")

	var teamName string
	db.QueryRow(ctx, "SELECT name FROM teams WHERE id = $1", teamID).Scan(&teamName)
	playerName := firstName + " " + lastName
	store.LogActivity(db, playerLeague, teamID, "ROSTER",
		fmt.Sprintf("%s designated %s for assignment", teamName, playerName))

	fmt.Printf("AGENT ACTION: DFA player %s (%s) from team %s, clear_action=%s\n",
		playerName, playerID, teamID, clearAction)
	return map[string]interface{}{
		"success":      true,
		"player_name":  playerName,
		"team_name":    teamName,
		"clear_action": clearAction,
		"waiver_end":   waiverEnd.Format("2006-01-02 3:04 PM"),
		"message":      fmt.Sprintf("%s designated for assignment. Waivers expire %s.", playerName, waiverEnd.Format("Jan 2, 3:04 PM")),
	}
}

func toolSetLeagueDate(db *pgxpool.Pool, ac *agentCtx, args map[string]interface{}) map[string]interface{} {
	leagueID := getStringArg(args, "league_id")
	dateType := getStringArg(args, "date_type")
	dateStr := getStringArg(args, "date")

	if leagueID == "" || dateType == "" || dateStr == "" {
		return map[string]interface{}{"error": "league_id, date_type, and date are required"}
	}

	// Validate date_type
	validTypes := map[string]bool{
		"opening_day": true, "trade_deadline": true, "extension_deadline": true,
		"option_deadline": true, "ifa_window_open": true, "ifa_window_close": true,
		"milb_fa_window_open": true, "milb_fa_window_close": true,
		"roster_expansion_start": true, "roster_expansion_end": true,
	}
	if !validTypes[dateType] {
		return map[string]interface{}{"error": fmt.Sprintf("Invalid date_type: %s", dateType)}
	}

	// Validate date format
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		return map[string]interface{}{"error": "Invalid date format. Use YYYY-MM-DD (e.g. 2026-03-26)"}
	}

	year := time.Now().Year()
	if y := getIntArg(args, "year"); y > 0 {
		year = y
	}

	// Determine which leagues to update
	allLeagues := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
	}
	leagueNames := map[string]string{
		"11111111-1111-1111-1111-111111111111": "MLB",
		"22222222-2222-2222-2222-222222222222": "AAA",
		"33333333-3333-3333-3333-333333333333": "AA",
		"44444444-4444-4444-4444-444444444444": "High-A",
	}

	var targetLeagues []string
	if strings.ToLower(leagueID) == "all" {
		// Filter to only leagues the commissioner has access to
		for _, lid := range allLeagues {
			if ac.canAccessLeague(lid) {
				targetLeagues = append(targetLeagues, lid)
			}
		}
		if len(targetLeagues) == 0 {
			return map[string]interface{}{"error": "You don't have access to any leagues"}
		}
	} else {
		if !ac.canAccessLeague(leagueID) {
			return map[string]interface{}{"error": "You don't have access to this league"}
		}
		targetLeagues = []string{leagueID}
	}

	var updated []string
	for _, lid := range targetLeagues {
		if err := store.UpsertLeagueDate(db, lid, year, dateType, dateStr); err != nil {
			fmt.Printf("ERROR [AgentTool:set_league_date]: league=%s err=%v\n", lid, err)
			return map[string]interface{}{"error": fmt.Sprintf("Failed to set date for %s: %v", leagueNames[lid], err)}
		}
		updated = append(updated, leagueNames[lid])
	}

	fmt.Printf("AGENT ACTION: Set %s=%s for year %d in leagues: %v\n", dateType, dateStr, year, updated)
	return map[string]interface{}{
		"success":  true,
		"leagues":  updated,
		"date_type": dateType,
		"date":     dateStr,
		"year":     year,
		"message":  fmt.Sprintf("Set %s to %s for %d in: %s", dateType, dateStr, year, strings.Join(updated, ", ")),
	}
}
