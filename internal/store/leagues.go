package store

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Team struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

type League struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Teams []Team `json:"teams"`
}

type KeyDate struct {
	ID        string `json:"id"`
	LeagueID  string `json:"league_id"`
	LeagueName string `json:"league_name"`
	EventDate string `json:"event_date"`
	EventName string `json:"event_name"`
}

func GetLeaguesWithTeams(db *pgxpool.Pool) ([]League, error) {
	rows, err := db.Query(context.Background(), `SELECT id, name FROM leagues ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leagues []League
	for rows.Next() {
		var l League
		if err := rows.Scan(&l.ID, &l.Name); err != nil {
			return nil, err
		}
		l.Teams = []Team{}
		leagues = append(leagues, l)
	}

	for i := range leagues {
		teamRows, err := db.Query(context.Background(),
			`SELECT id, name, owner_name FROM teams WHERE league_id = $1 ORDER BY name`,
			leagues[i].ID)
		if err != nil {
			return nil, err
		}
		defer teamRows.Close()

		for teamRows.Next() {
			var t Team
			if err := teamRows.Scan(&t.ID, &t.Name, &t.Owner); err != nil {
				continue
			}
			leagues[i].Teams = append(leagues[i].Teams, t)
		}
	}

	return leagues, nil
}

func GetKeyDates(db *pgxpool.Pool, leagueID string) ([]KeyDate, error) {
	query := `
		SELECT kd.id, kd.league_id, l.name, kd.event_date, kd.event_name
		FROM key_dates kd
		JOIN leagues l ON kd.league_id = l.id
	`
	var args []interface{}
	if leagueID != "" {
		query += " WHERE kd.league_id = $1 "
		args = append(args, leagueID)
	}
	
	query += " ORDER BY l.name ASC, kd.created_at ASC"

	rows, err := db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []KeyDate
	for rows.Next() {
		var kd KeyDate
		if err := rows.Scan(&kd.ID, &kd.LeagueID, &kd.LeagueName, &kd.EventDate, &kd.EventName); err != nil {
			continue
		}
		dates = append(dates, kd)
	}
	return dates, nil
}
