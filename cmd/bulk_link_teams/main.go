package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbUrl := "postgres://admin:password123@localhost:5433/fantasy_db"
	db, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("🔗 Starting Bulk Team Linking...")

	// This query attempts to match teams.owner_name to users.username
	// It only updates teams that are currently unassigned.
	result, err := db.Exec(context.Background(), `
		UPDATE teams t
		SET user_id = u.id
		FROM users u
		WHERE LOWER(TRIM(t.owner_name)) = LOWER(TRIM(u.username))
		AND (t.user_id IS NULL OR t.user_id = '00000000-0000-0000-0000-000000000000')
	`)

	if err != nil {
		log.Fatalf("Linking failed: %v", err)
	}

	rowsAffected := result.RowsAffected()
	fmt.Printf("✅ Successfully linked %d teams to their owners!
", rowsAffected)

	// Final check: List remaining unassigned teams
	var remaining int
	db.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM teams 
		WHERE user_id IS NULL OR user_id = '00000000-0000-0000-0000-000000000000'
	`).Scan(&remaining)

	if remaining > 0 {
		fmt.Printf("⚠️  Note: %d teams remain unassigned (no matching username found).
", remaining)
	} else {
		fmt.Println("🏆 All teams have been successfully linked!")
	}
}
