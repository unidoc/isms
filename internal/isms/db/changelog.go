package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ChangelogEntry represents a single field-level change in the entity changelog.
type ChangelogEntry struct {
	ID             int64     `json:"id"`
	OrganizationID int       `json:"organization_id"`
	EntityType     string    `json:"entity_type"`
	EntityID       int64     `json:"entity_id"`
	Action         string    `json:"action"`
	Field          string    `json:"field,omitempty"`
	OldValue       *string   `json:"old_value,omitempty"`
	NewValue       *string   `json:"new_value,omitempty"`
	ChangedBy      string    `json:"changed_by"`
	APIKeyID       *int      `json:"api_key_id,omitempty"`
	Reason         string `json:"reason,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
}

// LogChange inserts a single changelog entry.
func (d *DB) LogChange(ctx context.Context, orgID int, entry *ChangelogEntry) error {
	entry.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO entity_changelog (organization_id, entity_type, entity_id, action, field, old_value, new_value, changed_by, changed_by_user_id, api_key_id, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, (SELECT id FROM users WHERE email = $8), $9, $10)
		RETURNING id, created_at
	`, orgID, entry.EntityType, entry.EntityID, entry.Action,
		nilIfEmpty(entry.Field), entry.OldValue, entry.NewValue,
		entry.ChangedBy, entry.APIKeyID, nilIfEmpty(entry.Reason),
	).Scan(&entry.ID, &entry.CreatedAt)
}

// LogChanges inserts multiple changelog entries in a single batch.
func (d *DB) LogChanges(ctx context.Context, orgID int, entries []ChangelogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString(`INSERT INTO entity_changelog (organization_id, entity_type, entity_id, action, field, old_value, new_value, changed_by, changed_by_user_id, api_key_id, reason) VALUES `)

	args := make([]interface{}, 0, len(entries)*10)
	for i, e := range entries {
		if i > 0 {
			b.WriteString(", ")
		}
		base := i * 10
		b.WriteString(fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, (SELECT id FROM users WHERE email = $%d), $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+8, base+9, base+10))
		args = append(args, orgID, e.EntityType, e.EntityID, e.Action,
			nilIfEmpty(e.Field), e.OldValue, e.NewValue,
			e.ChangedBy, e.APIKeyID, nilIfEmpty(e.Reason))
	}

	_, err := d.pool.Exec(ctx, b.String(), args...)
	return err
}

// ListEntityChangelog returns the full changelog for a specific entity.
func (d *DB) ListEntityChangelog(ctx context.Context, orgID int, entityType string, entityID int64) ([]ChangelogEntry, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, entity_type, entity_id, action,
			COALESCE(field, ''), old_value, new_value,
			changed_by, api_key_id, COALESCE(reason, ''), created_at
		FROM entity_changelog
		WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3
		ORDER BY created_at DESC
	`, orgID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanChangelog(rows)
}

// ListChangelog returns recent changelog entries, optionally filtered by entity type.
func (d *DB) ListChangelog(ctx context.Context, orgID int, entityType string, limit int) ([]ChangelogEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []interface{}
	if entityType != "" {
		query = `
			SELECT id, organization_id, entity_type, entity_id, action,
				COALESCE(field, ''), old_value, new_value,
				changed_by, api_key_id, COALESCE(reason, ''), created_at
			FROM entity_changelog
			WHERE organization_id = $1 AND entity_type = $2
			ORDER BY created_at DESC
			LIMIT $3`
		args = []interface{}{orgID, entityType, limit}
	} else {
		query = `
			SELECT id, organization_id, entity_type, entity_id, action,
				COALESCE(field, ''), old_value, new_value,
				changed_by, api_key_id, COALESCE(reason, ''), created_at
			FROM entity_changelog
			WHERE organization_id = $1
			ORDER BY created_at DESC
			LIMIT $2`
		args = []interface{}{orgID, limit}
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanChangelog(rows)
}

type changelogRows interface {
	Next() bool
	Scan(dest ...interface{}) error
}

func scanChangelog(rows changelogRows) ([]ChangelogEntry, error) {
	var entries []ChangelogEntry
	for rows.Next() {
		var e ChangelogEntry
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.EntityType, &e.EntityID, &e.Action,
			&e.Field, &e.OldValue, &e.NewValue,
			&e.ChangedBy, &e.APIKeyID, &e.Reason, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// intPtrStr converts *int to string for changelog diffing. nil → "".
func intPtrStr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// epochToString converts an *Epoch to a date string for changelog diffing.
func epochToString(e *Epoch) string {
	if e == nil || e.IsZero() {
		return ""
	}
	return e.Format("2006-01-02")
}

// DiffFields compares two maps of field values and returns changelog entries for changed fields.
func DiffFields(entityType string, entityID int64, changedBy, reason string, oldFields, newFields map[string]string) []ChangelogEntry {
	var entries []ChangelogEntry
	for key, newVal := range newFields {
		oldVal, exists := oldFields[key]
		if !exists || oldVal != newVal {
			old := oldVal
			nv := newVal
			entries = append(entries, ChangelogEntry{
				EntityType: entityType,
				EntityID:   entityID,
				Action:     "update",
				Field:      key,
				OldValue:   &old,
				NewValue:   &nv,
				ChangedBy:  changedBy,
				Reason:     reason,
			})
		}
	}
	return entries
}

// --- ToChangeMap methods for all register entities ---

func (a *Asset) ToChangeMap() map[string]string {
	return map[string]string{
		"name":             a.Name,
		"description":      a.Description,
		"asset_type":       a.AssetType,
		"status":           a.Status,
		"owner":            a.Owner,
		"primary_location": a.PrimaryLocation,
		"confidentiality":  intPtrStr(a.Confidentiality),
		"integrity":        intPtrStr(a.Integrity),
		"availability":     intPtrStr(a.Availability),
		"last_review":      epochToString(a.LastReview),
		"next_review":      epochToString(a.NextReview),
		"notes":            a.Notes,
	}
}

func (r *Risk) ToChangeMap() map[string]string {
	return map[string]string{
		"title":                           r.Title,
		"description":                     r.Description,
		"risk_type":                       r.RiskType,
		"origin":                          r.Origin,
		"category":                        r.Category,
		"current_likelihood":              intPtrStr(r.CurrentLikelihood),
		"current_impact":                  intPtrStr(r.CurrentImpact),
		"current_score":                   intPtrStr(r.CurrentScore),
		"current_level":                   r.CurrentLevel,
		"confidentiality_impact":          intPtrStr(r.ConfidentialityImpact),
		"integrity_impact":                intPtrStr(r.IntegrityImpact),
		"availability_impact":             intPtrStr(r.AvailabilityImpact),
		"inherent_likelihood":             intPtrStr(r.InherentLikelihood),
		"inherent_impact":                 intPtrStr(r.InherentImpact),
		"inherent_score":                  intPtrStr(r.InherentScore),
		"inherent_confidentiality_impact": intPtrStr(r.InherentConfidentialityImpact),
		"inherent_integrity_impact":       intPtrStr(r.InherentIntegrityImpact),
		"inherent_availability_impact":    intPtrStr(r.InherentAvailabilityImpact),
		"target_likelihood":               intPtrStr(r.TargetLikelihood),
		"target_impact":                   intPtrStr(r.TargetImpact),
		"target_score":                    intPtrStr(r.TargetScore),
		"target_level":                    r.TargetLevel,
		"treatment":                       r.Treatment,
		"treatment_plan":                  r.TreatmentPlan,
		"treatment_due_date":              epochToString(r.TreatmentDueDate),
		"accepted_at":                     epochToString(r.AcceptedAt),
		"accepted_by_id":                  intPtrStr(r.AcceptedByID),
		"owner":                           r.Owner,
		"status":                          r.Status,
		"last_review":                     epochToString(r.LastReview),
		"next_review":                     epochToString(r.NextReview),
		"notes":                           r.Notes,
	}
}

func (sup *Supplier) ToChangeMap() map[string]string {
	return map[string]string{
		"name":            sup.Name,
		"supplier_type":   sup.SupplierType,
		"criticality":     sup.Criticality,
		"data_access":     strconv.FormatBool(sup.DataAccess),
		"contact":         sup.Contact,
		"contract_ref":    sup.ContractRef,
		"status":          sup.Status,
		"owner":           sup.Owner,
		"confidentiality": intPtrStr(sup.Confidentiality),
		"integrity":       intPtrStr(sup.Integrity),
		"availability":    intPtrStr(sup.Availability),
		"last_review":     epochToString(sup.LastReview),
		"next_review":     epochToString(sup.NextReview),
		"notes":           sup.Notes,
	}
}

// System.ToChangeMap is defined in systems.go

func (lr *LegalRequirement) ToChangeMap() map[string]string {
	return map[string]string{
		"title":              lr.Title,
		"description":        lr.Description,
		"jurisdiction":       lr.Jurisdiction,
		"category":           lr.Category,
		"reference":          lr.Reference,
		"url":                lr.URL,
		"status":             lr.Status,
		"owner":              lr.Owner,
		"last_review":        epochToString(lr.LastReview),
		"next_review":        epochToString(lr.NextReview),
		"notes":              lr.Notes,
		"current_likelihood": intPtrStr(lr.CurrentLikelihood),
		"current_impact":     intPtrStr(lr.CurrentImpact),
		"current_score":      intPtrStr(lr.CurrentScore),
		"current_level":      lr.CurrentLevel,
		"treatment":          lr.Treatment,
		"treatment_plan":     lr.TreatmentPlan,
		"target_likelihood":  intPtrStr(lr.TargetLikelihood),
		"target_impact":      intPtrStr(lr.TargetImpact),
		"completion":         strconv.Itoa(lr.Completion),
	}
}

func (inc *Incident) ToChangeMap() map[string]string {
	return map[string]string{
		"title":              inc.Title,
		"description":        inc.Description,
		"severity":           inc.Severity,
		"status":             inc.Status,
		"affects_c":          fmt.Sprintf("%t", inc.AffectsC),
		"affects_i":          fmt.Sprintf("%t", inc.AffectsI),
		"affects_a":          fmt.Sprintf("%t", inc.AffectsA),
		"incident_type":      inc.IncidentType,
		"source":             inc.Source,
		"notes":              inc.Notes,
		"data_breach":        fmt.Sprintf("%t", inc.DataBreach),
		"gdpr_role":          inc.GDPRRole,
		"authority_notified": inc.AuthorityNotified,
		"subjects_notified":  inc.SubjectsNotified,
		"assignee":           inc.Assignee,
		"root_cause":         inc.RootCause,
		"lessons_learned":    inc.LessonsLearned,
	}
}

func (ca *CorrectiveAction) ToChangeMap() map[string]string {
	return map[string]string{
		"title":       ca.Title,
		"description": ca.Description,
		"source":      ca.Source,
		"severity":    ca.Severity,
		"status":      ca.Status,
		"assignee":    ca.Assignee,
		"due_date":    epochToString(ca.DueDate),
		"root_cause":  ca.RootCause,
		"notes":       ca.Notes,
	}
}
