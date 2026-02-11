package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func CreateUser(db *pgxpool.Pool, username, email, passwordHash string) (*User, error) {
	var u User
	err := db.QueryRow(context.Background(),
		`INSERT INTO users (username, email, password_hash) 
		 VALUES ($1, $2, $3) 
		 RETURNING id, username, email, role, created_at`,
		username, email, passwordHash).Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func GetUserByEmail(db *pgxpool.Pool, email string) (*User, error) {
	var u User
	err := db.QueryRow(context.Background(),
		`SELECT id, username, email, password_hash, role, created_at FROM users WHERE email = $1`,
		email).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func CreateSession(db *pgxpool.Pool, userID, token string, expiresAt time.Time) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO sessions (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID, token, expiresAt)
	return err
}

func GetUserBySessionToken(db *pgxpool.Pool, token string) (*User, error) {
	var u User
	err := db.QueryRow(context.Background(),
		`SELECT u.id, u.username, u.email, u.role 
		 FROM sessions s
		 JOIN users u ON s.user_id = u.id
		 WHERE s.token = $1 AND s.expires_at > NOW()`,
		token).Scan(&u.ID, &u.Username, &u.Email, &u.Role)
	
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Valid result, just no user found
		}
		return nil, err
	}
	return &u, nil
}

func GetAdminLeagues(db *pgxpool.Pool, userID string) ([]string, error) {
	rows, err := db.Query(context.Background(), `
		SELECT league_id FROM league_roles WHERE user_id = $1 AND role = 'commissioner'
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leagueIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			leagueIDs = append(leagueIDs, id)
		}
	}
	return leagueIDs, nil
}