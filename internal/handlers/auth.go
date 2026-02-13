package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func RegisterPageHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		RenderTemplate(c, "register.html", nil)
	}
}

func LoginPageHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		RenderTemplate(c, "login.html", nil)
	}
}

func RegisterHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.PostForm("username")
		email := c.PostForm("email")
		password := c.PostForm("password")

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to hash password")
			return
		}

		err = store.CreateRegistrationRequest(db, username, email, string(hashedPassword))
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to submit registration: %v", err)
			return
		}

		RenderTemplate(c, "register.html", gin.H{
			"Success": true,
		})
	}
}

func LoginHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := c.PostForm("email")
		password := c.PostForm("password")

		user, err := store.GetUserByEmailOrUsername(db, identifier)
		if err != nil {
			c.String(http.StatusUnauthorized, "Invalid username/email or password")
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
		if err != nil {
			c.String(http.StatusUnauthorized, "Invalid email or password")
			return
		}

		tokenBytes := make([]byte, 16)
		rand.Read(tokenBytes)
		token := hex.EncodeToString(tokenBytes)

		expiresAt := time.Now().Add(24 * time.Hour)
		err = store.CreateSession(db, user.ID, token, expiresAt)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to create session")
			return
		}

		c.SetCookie("session_token", token, 3600*24, "/", "", false, true)
		c.Redirect(http.StatusFound, "/home")
	}
}

func LogoutHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.SetCookie("session_token", "", -1, "/", "", false, true)
		c.Redirect(http.StatusFound, "/login")
	}
}

func RenderTemplate(c *gin.Context, tmplName string, data interface{}) {
	funcMap := template.FuncMap{
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 { return nil, fmt.Errorf("invalid dict call") }
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok { return nil, fmt.Errorf("dict keys must be strings") }
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"seq": func(start, end int) []int {
			var s []int
			for i := start; i <= end; i++ {
				s = append(s, i)
			}
			return s
		},
		"formatMoney": func(v interface{}) string {
			p := message.NewPrinter(language.English)
			switch val := v.(type) {
			case float64: return p.Sprintf("%d", int64(val))
			case int: return p.Sprintf("%d", val)
			case string: return val
			default: return fmt.Sprintf("%v", v)
			}
		},
	}

	lp := filepath.Join("templates", "layout.html")
	fp := filepath.Join("templates", tmplName)

	// NEW: Use template.Must and Clone to ensure FuncMap is correctly attached before parsing.
	baseTmpl := template.New("layout").Funcs(funcMap)
	tmpl, err := baseTmpl.ParseFiles(lp, fp)
	
	if err != nil {
		fmt.Printf("TEMPLATE PARSE ERROR: %v\n", err)
		c.String(http.StatusInternalServerError, "Error loading template: %v", err)
		return
	}

	if err := tmpl.Execute(c.Writer, data); err != nil {
		fmt.Printf("TEMPLATE EXECUTE ERROR: %v\n", err)
		c.String(http.StatusInternalServerError, "Error rendering template: %v", err)
	}
}
