package db

import (
	"context"
)

// Program groups related objectives under a keyed programme.
type Program struct {
	ID             int64     `json:"id"`
	Identifier     string    `json:"identifier"`
	OrganizationID int       `json:"organization_id"`
	Key            string    `json:"key"`
	Title          string    `json:"title"`
	Description    string    `json:"description,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	Owner          string `json:"owner,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
	UpdatedAt      Epoch  `json:"updated_at"`
}

func (d *DB) CreateProgram(ctx context.Context, orgID int, p *Program) error {
	p.OrganizationID = orgID
	ident, err := d.NextIdentifier(ctx, orgID, "program")
	if err != nil {
		return err
	}
	p.Identifier = ident
	return d.pool.QueryRow(ctx, `
		INSERT INTO programs (organization_id, identifier, key, title, description, notes, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6,
			CASE WHEN $7 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $7) END)
		RETURNING id, created_at, updated_at
	`, orgID, p.Identifier, p.Key, p.Title, nilIfEmpty(p.Description), nilIfEmpty(p.Notes), p.Owner,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (d *DB) GetProgram(ctx context.Context, orgID int, id int64) (*Program, error) {
	var p Program
	err := d.pool.QueryRow(ctx, `
		SELECT id, identifier, organization_id, key, title, COALESCE(description, ''), COALESCE(notes, ''),
			COALESCE((SELECT email FROM users WHERE id = programs.owner_id), ''),
			created_at, updated_at
		FROM programs WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID).Scan(&p.ID, &p.Identifier, &p.OrganizationID, &p.Key, &p.Title,
		&p.Description, &p.Notes, &p.Owner, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) GetProgramByKey(ctx context.Context, orgID int, key string) (*Program, error) {
	var p Program
	err := d.pool.QueryRow(ctx, `
		SELECT id, identifier, organization_id, key, title, COALESCE(description, ''), COALESCE(notes, ''),
			COALESCE((SELECT email FROM users WHERE id = programs.owner_id), ''),
			created_at, updated_at
		FROM programs WHERE key = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, key, orgID).Scan(&p.ID, &p.Identifier, &p.OrganizationID, &p.Key, &p.Title,
		&p.Description, &p.Notes, &p.Owner, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) ListPrograms(ctx context.Context, orgID int) ([]Program, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, identifier, organization_id, key, title, COALESCE(description, ''), COALESCE(notes, ''),
			COALESCE((SELECT email FROM users WHERE id = programs.owner_id), ''),
			created_at, updated_at
		FROM programs WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY key
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	programs := []Program{}
	for rows.Next() {
		var p Program
		if err := rows.Scan(&p.ID, &p.Identifier, &p.OrganizationID, &p.Key, &p.Title,
			&p.Description, &p.Notes, &p.Owner, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		programs = append(programs, p)
	}
	return programs, nil
}

func (d *DB) UpdateProgram(ctx context.Context, orgID int, p *Program) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE programs SET key = $2, title = $3, description = $4, notes = $5,
			owner_id = CASE WHEN $6 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $6) END,
			updated_at = now()
		WHERE id = $1 AND organization_id = $7 AND deleted_at IS NULL
	`, p.ID, p.Key, p.Title, nilIfEmpty(p.Description), nilIfEmpty(p.Notes), p.Owner, orgID)
	return err
}

func (d *DB) DeleteProgram(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `UPDATE programs SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, orgID)
	return err
}

func (p *Program) ToChangeMap() map[string]string {
	return map[string]string{
		"key":         p.Key,
		"title":       p.Title,
		"description": p.Description,
		"notes":       p.Notes,
		"owner":       p.Owner,
	}
}
