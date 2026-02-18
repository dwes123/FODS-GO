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

func GetUserByEmailOrUsername(db *pgxpool.Pool, identifier string) (*User, error) {
	var u User
	err := db.QueryRow(context.Background(),
		`SELECT id, username, email, password_hash, role, created_at FROM users
		 WHERE LOWER(email) = LOWER($1) OR LOWER(username) = LOWER($1)`,
		identifier).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func UpdateUserPassword(db *pgxpool.Pool, userID, newHash string) error {
	_, err := db.Exec(context.Background(), 
		"UPDATE users SET password_hash = $1 WHERE id = $2", 
		newHash, userID)
	return err
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

// --- Registration Request Queue (Feature 9) ---

type RegistrationRequest struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

func CreateRegistrationRequest(db *pgxpool.Pool, username, email, passwordHash string) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO registration_requests (username, email, password_hash)
		 VALUES ($1, $2, $3)`,
		username, email, passwordHash)
	return err
}

func GetPendingRegistrations(db *pgxpool.Pool) ([]RegistrationRequest, error) {
	rows, err := db.Query(context.Background(),
		`SELECT id, username, email, status, created_at
		 FROM registration_requests
		 WHERE status = 'pending'
		 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []RegistrationRequest
	for rows.Next() {
		var r RegistrationRequest
		if err := rows.Scan(&r.ID, &r.Username, &r.Email, &r.Status, &r.CreatedAt); err != nil {
			continue
		}
		reqs = append(reqs, r)
	}
	return reqs, nil
}

func ApproveRegistration(db *pgxpool.Pool, requestID, reviewerID string) error {
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var username, email, passwordHash string
	err = tx.QueryRow(ctx,
		`SELECT username, email, password_hash FROM registration_requests WHERE id = $1 AND status = 'pending'`,
		requestID).Scan(&username, &email, &passwordHash)
	if err != nil {
		return err
	}

	// Create the actual user
	_, err = tx.Exec(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3)`,
		username, email, passwordHash)
	if err != nil {
		return err
	}

	// Mark request as approved
	_, err = tx.Exec(ctx,
		`UPDATE registration_requests SET status = 'approved', reviewed_by = $1, reviewed_at = NOW() WHERE id = $2`,
		reviewerID, requestID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func DenyRegistration(db *pgxpool.Pool, requestID, reviewerID string) error {
	_, err := db.Exec(context.Background(),
		`UPDATE registration_requests SET status = 'denied', reviewed_by = $1, reviewed_at = NOW() WHERE id = $2`,
		reviewerID, requestID)
	return err
}

func GetTeamOwnerEmails(db *pgxpool.Pool, teamID string) ([]string, error) {
	rows, err := db.Query(context.Background(), `
		SELECT DISTINCT u.email FROM users u
		LEFT JOIN team_owners tow ON tow.user_id = u.id
		LEFT JOIN teams t ON t.user_id = u.id
		WHERE tow.team_id = $1 OR t.id = $1
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err == nil && email != "" {
			emails = append(emails, email)
		}
	}
	return emails, nil
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