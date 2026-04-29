package nba

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AssignmentRow represents a single role assignment row for the admin UI.
// Same shape used for team owners + agency members + commissioners.
type AssignmentRow struct {
	ScopeID   string // team_id / agency_id / league_id
	ScopeName string // team name / agency name / league name
	UserID    string // soft FK to fantasy_db.users.id
	IsPrimary bool   // for team_owners / agency_members
}

// ListNBATeamOwnerships returns one row per team→user assignment, joined to team name.
func ListNBATeamOwnerships(nbaDB *pgxpool.Pool) ([]AssignmentRow, error) {
	rows, err := nbaDB.Query(context.Background(), `
		SELECT t.id::TEXT, t.name, o.user_id::TEXT, o.is_primary
		FROM team_owners o
		JOIN teams t ON t.id = o.team_id
		ORDER BY t.name, o.created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssignmentRow
	for rows.Next() {
		var a AssignmentRow
		if err := rows.Scan(&a.ScopeID, &a.ScopeName, &a.UserID, &a.IsPrimary); err == nil {
			out = append(out, a)
		}
	}
	return out, nil
}

// ListAgencyMemberships returns one row per agency→user assignment.
func ListAgencyMemberships(nbaDB *pgxpool.Pool) ([]AssignmentRow, error) {
	rows, err := nbaDB.Query(context.Background(), `
		SELECT a.id::TEXT, a.name, m.user_id::TEXT, m.is_primary
		FROM agency_members m
		JOIN agencies a ON a.id = m.agency_id
		ORDER BY a.name, m.created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssignmentRow
	for rows.Next() {
		var a AssignmentRow
		if err := rows.Scan(&a.ScopeID, &a.ScopeName, &a.UserID, &a.IsPrimary); err == nil {
			out = append(out, a)
		}
	}
	return out, nil
}

// AddTeamOwner inserts a team_owners row. Idempotent via ON CONFLICT DO NOTHING.
func AddTeamOwner(nbaDB *pgxpool.Pool, teamID, userID string, isPrimary bool) error {
	_, err := nbaDB.Exec(context.Background(), `
		INSERT INTO team_owners (team_id, user_id, is_primary)
		VALUES ($1, $2, $3)
		ON CONFLICT (team_id, user_id) DO NOTHING
	`, teamID, userID, isPrimary)
	return err
}

// RemoveTeamOwner deletes a team_owners row.
func RemoveTeamOwner(nbaDB *pgxpool.Pool, teamID, userID string) error {
	_, err := nbaDB.Exec(context.Background(),
		`DELETE FROM team_owners WHERE team_id = $1 AND user_id = $2`,
		teamID, userID)
	return err
}

// AddAgencyMember inserts an agency_members row. Idempotent.
func AddAgencyMember(nbaDB *pgxpool.Pool, agencyID, userID string, isPrimary bool) error {
	_, err := nbaDB.Exec(context.Background(), `
		INSERT INTO agency_members (agency_id, user_id, is_primary)
		VALUES ($1, $2, $3)
		ON CONFLICT (agency_id, user_id) DO NOTHING
	`, agencyID, userID, isPrimary)
	return err
}

// RemoveAgencyMember deletes an agency_members row.
func RemoveAgencyMember(nbaDB *pgxpool.Pool, agencyID, userID string) error {
	_, err := nbaDB.Exec(context.Background(),
		`DELETE FROM agency_members WHERE agency_id = $1 AND user_id = $2`,
		agencyID, userID)
	return err
}

// ListAgencies returns all agencies for the league (used to populate the add form).
func ListAgencies(nbaDB *pgxpool.Pool) ([]struct{ ID, Name string }, error) {
	rows, err := nbaDB.Query(context.Background(),
		`SELECT id::TEXT, name FROM agencies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct{ ID, Name string }
	for rows.Next() {
		var r struct{ ID, Name string }
		if err := rows.Scan(&r.ID, &r.Name); err == nil {
			out = append(out, r)
		}
	}
	return out, nil
}
