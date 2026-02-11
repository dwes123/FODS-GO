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

	fmt.Println("Running final team mapping check...")

	rows, err := db.Query(context.Background(), `
		SELECT DISTINCT p.raw_fantasy_team_id, p.team_id 
		FROM players p 
		JOIN teams t ON p.team_id = t.id 
		WHERE t.abbreviation IS NULL AND p.raw_fantasy_team_id != ''
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	mapped := 0
	for rows.Next() {
		var abbrev, teamID string
		if err := rows.Scan(&abbrev, &teamID); err != nil {
			continue
		}

		_, err := db.Exec(context.Background(), "UPDATE teams SET abbreviation = $1 WHERE id = $2", abbrev, teamID)
		if err == nil {
			mapped++
			fmt.Printf("Fixed missing abbrev: Team %s -> %s\n", teamID, abbrev)
		}
	}

	fmt.Printf("Successfully fixed %d more team mappings.\n", mapped)
}