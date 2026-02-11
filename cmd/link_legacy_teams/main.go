package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

const UsersURL = "https://frontofficedynastysports.com/wp-json/wp/v2/users?per_page=100"

type WPUser struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
}

func main() {
	dbUrl := "postgres://admin:password123@localhost:5433/fantasy_db"
	db, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("Attempting to auto-link teams based on legacy information...")

	resp, err := http.Get(UsersURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var wpUsers []WPUser
	json.Unmarshal(body, &wpUsers)

	linkedCount := 0

	for _, wpu := range wpUsers {
		var goUserID string
		err := db.QueryRow(context.Background(), 
			"SELECT id FROM users WHERE username = $1", wpu.Name).Scan(&goUserID)

		if err != nil {
			continue
		}

		db.Exec(context.Background(), "UPDATE users SET wp_id = $1 WHERE id = $2", wpu.ID, goUserID)

		result, err := db.Exec(context.Background(), `
			UPDATE teams 
			SET user_id = $1, owner_name = $2
			WHERE wp_id = $3
		`, goUserID, wpu.Name, wpu.ID)

		if err == nil {
			rows := result.RowsAffected()
			if rows > 0 {
				linkedCount += int(rows)
				fmt.Printf("✅ Auto-linked %d teams to user '%s'\n", rows, wpu.Name)
			}
		}
	}

	fmt.Printf("\nDone. Auto-linked %d total teams.\n", linkedCount)
}