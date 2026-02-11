package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
	"github.com/dwes123/fantasy-baseball-go/internal/handlers"
	"github.com/dwes123/fantasy-baseball-go/internal/middleware"
	"github.com/dwes123/fantasy-baseball-go/internal/worker"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// 1. Initialize Database
	database := db.InitDB()
	defer database.Close()

	        // 2. Start Background Workers
	        worker.StartBidWorker(database)
	        worker.StartWaiverWorker(database)
	// 3. Initialize Router
	r := gin.Default()

	// 3. CORS Configuration
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// --- PUBLIC ROUTES ---
	public := r.Group("/")
	{
		public.GET("/test", handlers.TestTemplateHandler(database))
		public.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "/home")
		})
		public.GET("/dashboard", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "/home")
		})
		public.GET("/login", handlers.LoginPageHandler())
		public.POST("/login", handlers.LoginHandler(database))
		public.GET("/register", handlers.RegisterPageHandler())
		public.POST("/register", handlers.RegisterHandler(database))
		public.GET("/logout", handlers.LogoutHandler(database))
	}

	// --- PROTECTED ROUTES ---
	authorized := r.Group("/")
	authorized.Use(middleware.AuthMiddleware(database))
	{
		authorized.GET("/home", handlers.HomeHandler(database))
		authorized.GET("/roster/:id", handlers.RosterHandler(database))

		// Free Agents & Players
		authorized.GET("/free-agents", handlers.FreeAgentHandler(database))
		authorized.GET("/player/:id", handlers.PlayerProfileHandler(database))
		authorized.POST("/bid", handlers.SubmitBidHandler(database))
		authorized.POST("/extension", handlers.SubmitExtensionHandler(database))
		authorized.POST("/restructure", handlers.ProcessRestructureHandler(database))

		// Standings & Financials
		authorized.GET("/standings", handlers.StandingsHandler(database))
		authorized.GET("/league/financials", handlers.LeagueFinancialsHandler(database))
		authorized.GET("/team/financials/:id", handlers.TeamFinancialsHandler(database))

		// Profile & Team Management
		authorized.GET("/profile", handlers.ProfileHandler(database))
		authorized.POST("/claim-team", handlers.ClaimTeamHandler(database))

		// Bug Reporting
		authorized.GET("/bug-report", handlers.BugReportFormHandler(database))
		authorized.POST("/bug-report", handlers.SubmitBugReportHandler(database))
		authorized.GET("/admin/bugs", handlers.AdminBugReportsHandler(database))

		// Waivers
		authorized.POST("/claim-waiver", handlers.ClaimWaiverHandler(database))
		authorized.GET("/waivers", handlers.WaiverWireHandler(database))

		// Trades
		authorized.GET("/trades", handlers.TradeCenterHandler(database))
		authorized.GET("/trades/new", handlers.NewTradeHandler(database))
		authorized.POST("/trades/submit", handlers.SubmitTradeHandler(database))
		authorized.POST("/trades/accept", handlers.AcceptTradeHandler(database))

		// Activity Feed
		authorized.GET("/activity", handlers.ActivityFeedHandler(database))

		// Roster Moves
		authorized.POST("/roster/move/40man", handlers.PromoteTo40ManHandler(database))
		authorized.POST("/roster/move/26man", handlers.PromoteTo26ManHandler(database))
		authorized.POST("/roster/move/option", handlers.OptionToMinorsHandler(database))
		authorized.POST("/roster/move/il", handlers.MoveToILHandler(database))
		authorized.POST("/roster/move/activate", handlers.ActivateFromILHandler(database))
		authorized.POST("/roster/move/dfa", handlers.DFAPlayerHandler(database))
		authorized.POST("/roster/move/trade-block", handlers.ToggleTradeBlockHandler(database))

		// Arbitration
		authorized.GET("/arbitration", handlers.ArbitrationHandler(database))
		authorized.POST("/arbitration/submit", handlers.SubmitArbitrationHandler(database))
		authorized.POST("/extension/submit", handlers.SubmitArbExtensionHandler(database))

		// Contracts & Options
		authorized.GET("/team-options", handlers.TeamOptionsHandler(database))
		authorized.POST("/team-options/decision", handlers.ProcessOptionDecisionHandler(database))

		// Weekly Rotations
		authorized.GET("/rotations", handlers.RotationsDashboardHandler(database))
		authorized.GET("/rotations/submit", handlers.RotationsSubmitPageHandler(database))
		authorized.POST("/rotations/save", handlers.SubmitRotationHandler(database))
		authorized.GET("/api/team/pitchers", handlers.GetTeamPitchersHandler(database))

		                // Commissioner Tools
		                authorized.GET("/admin", handlers.AdminDashboardHandler(database))
		                authorized.POST("/admin/process", handlers.ProcessActionHandler(database))
		                authorized.GET("/admin/player-editor", handlers.AdminPlayerEditorHandler(database))
		                authorized.POST("/admin/save-player", handlers.AdminSavePlayerHandler(database))
		                authorized.GET("/admin/dead-cap/", handlers.AdminDeadCapHandler(database))
		                authorized.POST("/admin/dead-cap/save", handlers.AdminSaveDeadCapHandler(database))
		                authorized.POST("/admin/dead-cap/delete", handlers.AdminDeleteDeadCapHandler(database))
		
		                // Commissioner Power Tools
		                authorized.GET("/admin/csv-import", handlers.AdminCSVImporterHandler(database))
		                authorized.POST("/admin/csv-import", handlers.AdminProcessCSVHandler(database))
		                authorized.GET("/admin/player-assign", handlers.AdminPlayerAssignHandler(database))
		                authorized.POST("/admin/player-assign", handlers.AdminProcessAssignHandler(database))
		                authorized.GET("/admin/trades", handlers.AdminTradeReviewHandler(database))	}

	// --- API ROUTES ---
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong", "status": "Moneyball API is Live ⚾"})
	})

	// 4. Start Server
	fmt.Println("🚀 Server starting on http://localhost:8080")
	r.Run(":8080")
}