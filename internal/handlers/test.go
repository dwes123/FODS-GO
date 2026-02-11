package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTemplateHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		data := map[string]interface{}{
			"Message": "If you see this, templates are working!",
			"HTMLContent": "<strong>Bold Text</strong>",
		}
		RenderTemplate(c, "test.html", data)
	}
}
