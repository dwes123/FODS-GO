package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func BugReportFormHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		RenderTemplate(c, "bug_report.html", gin.H{
			"User":      user,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}

func SubmitBugReportHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		title := c.PostForm("title")
		description := c.PostForm("description")

		_, err := db.Exec(context.Background(),
			"INSERT INTO bug_reports (user_id, title, description) VALUES ($1, $2, $3)",
			user.ID, title, description)

		if err != nil {
			fmt.Printf("ERROR [SubmitBugReport]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		RenderTemplate(c, "bug_report.html", gin.H{
			"User":      user,
			"Success":   true,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}

func AdminBugReportsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(context.Background(), `
			SELECT b.id, b.title, b.description, b.status, b.created_at, u.username
			FROM bug_reports b
			JOIN users u ON b.user_id = u.id
			ORDER BY b.created_at DESC
		`)
		if err != nil {
			fmt.Printf("ERROR [AdminBugReports]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}
		defer rows.Close()

		type BugReport struct {
			ID          string
			Title       string
			Description string
			Status      string
			CreatedAt   string
			Username    string
		}

		var bugs []BugReport
		for rows.Next() {
			var b BugReport
			rows.Scan(&b.ID, &b.Title, &b.Description, &b.Status, &b.CreatedAt, &b.Username)
			bugs = append(bugs, b)
		}

		user := c.MustGet("user").(*store.User)
		RenderTemplate(c, "admin_bugs.html", gin.H{
			"Bugs": bugs,
			"User": user,
		})
	}
}
