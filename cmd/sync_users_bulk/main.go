package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
	"golang.org/x/crypto/bcrypt"
)

type WPUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

func main() {
	database := db.InitDB()
	defer database.Close()

	wpUser := "djwes487@gmail.com"
	wpPass := "ab4H TPEh vyrc 9lOL T91Z Zt5L"
	auth := base64.StdEncoding.EncodeToString([]byte(wpUser + ":" + wpPass))

	fmt.Println("Starting Bulk User Sync...")

	tempPass := "dynasty2026"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(tempPass), bcrypt.DefaultCost)

	page := 1
	totalCreated := 0

	for {
		url := "https://frontofficedynastysports.com/wp-json/wp/v2/users?context=edit&per_page=100&page=" + strconv.Itoa(page)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Basic "+auth)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode >= 400 {
			break
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var wpUsers []WPUser
		json.Unmarshal(body, &wpUsers)

		if len(wpUsers) == 0 {
			break
		}

		for _, wpu := range wpUsers {
			var goUserID string
			err := database.QueryRow(context.Background(), `
				INSERT INTO users (username, email, password_hash, wp_id)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (wp_id) DO UPDATE SET email = EXCLUDED.email
				RETURNING id
			`, wpu.Username, wpu.Email, string(hashedPassword), wpu.ID).Scan(&goUserID)

			if err != nil {
				database.QueryRow(context.Background(), "SELECT id FROM users WHERE wp_id = $1", wpu.ID).Scan(&goUserID)
			}

			if goUserID != "" {
				result, _ := database.Exec(context.Background(), `
					UPDATE teams 
					SET user_id = $1, owner_name = $2
					WHERE wp_id = $3
				`, goUserID, wpu.Name, wpu.ID)
				
				rowsAffected := result.RowsAffected()
				if rowsAffected > 0 {
					fmt.Printf("Synced User: %-20s | Linked %d teams\n", wpu.Username, rowsAffected)
				} else {
					fmt.Printf("Synced User: %-20s\n", wpu.Username)
				}
				totalCreated++
			}
		}
		page++
	}

	fmt.Printf("\nDone! Synced %d users.\n", totalCreated)
}