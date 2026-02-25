package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
	"github.com/dwes123/fantasy-baseball-go/internal/handlers"
	"github.com/dwes123/fantasy-baseball-go/internal/middleware"
	"github.com/dwes123/fantasy-baseball-go/internal/notification"
	"github.com/dwes123/fantasy-baseball-go/internal/worker"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// 1. Initialize Database
	database := db.InitDB()
	defer database.Close()

	// 1b. Initialize Email Notifications
	notification.InitEmail()

	// 2. Start Background Workers with cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker.StartBidWorker(ctx, database)
	worker.StartWaiverWorker(ctx, database)
	worker.StartSeasonalWorker(ctx, database)
	worker.StartHRMonitor(ctx, database)
	worker.StartStatsWorker(ctx, database)

	// 3. Initialize Router
	r := gin.Default()
	r.SetTrustedProxies([]string{"127.0.0.1"})
	r.Use(middleware.SecurityHeaders())

	// 3. CORS Configuration
	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "https://frontofficedynastysports.com"
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{corsOrigin},
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
		public.POST("/login", middleware.RateLimit(10, time.Minute), handlers.LoginHandler(database))
		public.GET("/register", handlers.RegisterPageHandler())
		public.POST("/register", middleware.RateLimit(5, time.Minute), handlers.RegisterHandler(database))
		public.GET("/logout", handlers.LogoutHandler(database))
	}

	// --- PROTECTED ROUTES ---
	authorized := r.Group("/")
	authorized.Use(middleware.AuthMiddleware(database))
	{
		authorized.GET("/home", handlers.HomeHandler(database))
		authorized.GET("/roster/:id", handlers.RosterHandler(database))

		// League Rosters & Bid Calculator
		authorized.GET("/league/rosters", handlers.LeagueRostersHandler(database))
		authorized.GET("/bid-calculator", handlers.BidCalculatorHandler(database))

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
		authorized.POST("/profile/update-password", handlers.UpdatePasswordHandler(database))
		authorized.POST("/profile/update-theme", handlers.UpdateThemeHandler(database))

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
		authorized.POST("/trades/reject", handlers.RejectTradeHandler(database))
		authorized.GET("/trades/counter", handlers.CounterTradeHandler(database))
		authorized.POST("/trades/counter", handlers.SubmitCounterHandler(database))

		// Trade Block & Bid History
		authorized.GET("/trade-block", handlers.TradeBlockHandler(database))
		authorized.GET("/bids/pending", handlers.PendingBidsHandler(database))
		authorized.GET("/bids/history", handlers.BidHistoryHandler(database))

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
		authorized.POST("/roster/depth-order", handlers.SaveDepthOrderHandler(database))

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

		// Fantasy Stats
		authorized.GET("/stats/pitching", handlers.StatsLeaderboardHandler(database))
		authorized.GET("/stats/hitting", handlers.HittingLeaderboardHandler(database))
		authorized.GET("/api/player/:id/gamelog", handlers.PlayerGameLogHandler(database))
		authorized.POST("/admin/stats/backfill", handlers.AdminBackfillStatsHandler(database))

		// Commissioner Tools
		authorized.GET("/admin", handlers.AdminDashboardHandler(database))
		authorized.POST("/admin/process", handlers.ProcessActionHandler(database))
		authorized.GET("/admin/player-editor", handlers.AdminPlayerEditorHandler(database))
		authorized.POST("/admin/save-player", handlers.AdminSavePlayerHandler(database))
		authorized.GET("/admin/dead-cap/", handlers.AdminDeadCapHandler(database))
		authorized.POST("/admin/dead-cap/save", handlers.AdminSaveDeadCapHandler(database))
		authorized.POST("/admin/dead-cap/delete", handlers.AdminDeleteDeadCapHandler(database))

		// Commissioner Power Tools
		authorized.POST("/admin/trade-reverse", handlers.AdminReverseTradeHandler(database))
		authorized.POST("/admin/fantrax-toggle", handlers.ToggleFantraxHandler(database))
		authorized.GET("/admin/export-bids", handlers.BidExportHandler(database))
		authorized.GET("/admin/csv-import", handlers.AdminCSVImporterHandler(database))
		authorized.POST("/admin/csv-import", handlers.AdminProcessCSVHandler(database))
		authorized.GET("/admin/player-assign", handlers.AdminPlayerAssignHandler(database))
		authorized.POST("/admin/player-assign", handlers.AdminProcessAssignHandler(database))
		authorized.GET("/admin/trades", handlers.AdminTradeReviewHandler(database))
		authorized.GET("/admin/approvals", handlers.AdminApprovalsHandler(database))
		authorized.POST("/admin/approve-registration", handlers.AdminProcessRegistrationHandler(database))
		authorized.GET("/admin/settings", handlers.AdminSettingsHandler(database))
		authorized.POST("/admin/settings/save", handlers.AdminSaveSettingsHandler(database))
		authorized.GET("/admin/fantrax-queue", handlers.AdminFantraxQueueHandler(database))
		authorized.GET("/admin/waiver-audit", handlers.AdminWaiverAuditHandler(database))
		authorized.GET("/admin/roles", handlers.AdminRolesHandler(database))
		authorized.POST("/admin/roles/add", handlers.AdminAddRoleHandler(database))
		authorized.POST("/admin/roles/delete", handlers.AdminDeleteRoleHandler(database))
		authorized.GET("/admin/balance-editor", handlers.AdminBalanceEditorHandler(database))
		authorized.POST("/admin/balance-editor/save", handlers.AdminSaveBalanceHandler(database))
		authorized.GET("/admin/team-owners", handlers.AdminTeamOwnersHandler(database))
		authorized.POST("/admin/team-owners/add", handlers.AdminAddTeamOwnerHandler(database))
		authorized.POST("/admin/team-owners/remove", handlers.AdminRemoveTeamOwnerHandler(database))
		authorized.POST("/admin/team-owners/create-user", handlers.AdminCreateUserHandler(database))
		authorized.POST("/admin/team-owners/delete-user", handlers.AdminDeleteUserHandler(database))

		// AI Agent
		authorized.GET("/admin/agent", handlers.AdminAgentPageHandler(database))
		authorized.POST("/admin/agent", handlers.AdminAgentChatHandler(database))
	}

	// --- API ROUTES ---
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong", "status": "Moneyball API is Live ⚾"})
	})

	// 4. Start Server with graceful shutdown
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		fmt.Println("🚀 Server starting on http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting down server...")

	// Cancel worker context to stop background goroutines
	cancel()

	// Give outstanding requests 10 seconds to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Printf("Server forced to shutdown: %v\n", err)
	}

	fmt.Println("Server exited")
}
