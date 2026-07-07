package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Tx-aware variants of DB methods for atomic suggestion apply.
// These accept a pgx.Tx instead of using the pool, so the entire
// apply flow (entity mutation + suggestion mark + changelog) runs
// in a single transaction.

// nextIdentifierTx allocates the next per-org identifier within an existing transaction.
// Mirrors DB.NextIdentifier but operates on a tx — used by Tx-variant create methods.
func nextIdentifierTx(ctx context.Context, tx pgx.Tx, orgID int, entityType string) (string, error) {
	prefix := map[string]string{
		"risk": "RISK", "asset": "ASSET", "supplier": "SUPPLIER",
		"system": "SYSTEM", "legal_requirement": "LEGAL", "program": "PROG",
		"incident": "INC", "change_request": "CR", "task": "TASK", "corrective_action": "CA",
		"objective": "OBJ", "audit": "AUDIT", "audit_finding": "FIND",
	}[entityType]
	if prefix == "" {
		prefix = strings.ToUpper(entityType)
	}
	var seq int
	err := tx.QueryRow(ctx, `
		INSERT INTO identifier_sequences (organization_id, entity_type, next_value)
		VALUES ($1, $2, 1)
		ON CONFLICT (organization_id, entity_type) DO UPDATE SET next_value = identifier_sequences.next_value + 1
		RETURNING next_value
	`, orgID, entityType).Scan(&seq)
	if err != nil {
		return "", fmt.Errorf("allocate identifier: %w", err)
	}
	return fmt.Sprintf("%s-%d", prefix, seq), nil
}

// ApplySuggestionTx marks a suggestion as applied within an existing transaction.
func ApplySuggestionTx(ctx context.Context, tx pgx.Tx, orgID int, id int64, reviewerEmail string, appliedEntityID string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE suggestions SET
			status = 'applied',
			reviewed_by = $3,
			reviewed_by_user_id = (SELECT id FROM users WHERE email = $3),
			reviewed_at = now(),
			applied_at = now(),
			applied_entity_id = NULLIF($4, ''),
			updated_at = now()
		WHERE id = $1 AND organization_id = $2
			AND status IN ('open', 'in_review')
	`, id, orgID, reviewerEmail, appliedEntityID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("suggestion not found or already in terminal state")
	}
	return nil
}

// CreateRiskTx creates a risk within an existing transaction.
func CreateRiskTx(ctx context.Context, tx pgx.Tx, orgID int, r *Risk) error {
	r.OrganizationID = orgID
	if err := r.Validate(); err != nil {
		return err
	}
	r.CalculateScore(nil) // use defaults within tx

	// Allocate identifier within tx
	var seq int
	err := tx.QueryRow(ctx, `
		INSERT INTO identifier_sequences (organization_id, entity_type, next_value)
		VALUES ($1, 'risk', 1)
		ON CONFLICT (organization_id, entity_type)
		DO UPDATE SET next_value = identifier_sequences.next_value + 1
		RETURNING next_value
	`, orgID).Scan(&seq)
	if err != nil {
		return fmt.Errorf("allocate identifier: %w", err)
	}
	r.Identifier = fmt.Sprintf("RISK-%d", seq)

	return tx.QueryRow(ctx, `
		INSERT INTO risks (organization_id, identifier, title, description, risk_type, origin, category,
			current_likelihood, current_impact, current_score, current_level,
			confidentiality_impact, integrity_impact, availability_impact,
			inherent_likelihood, inherent_impact, inherent_score,
			inherent_confidentiality_impact, inherent_integrity_impact, inherent_availability_impact,
			target_likelihood, target_impact, target_score, target_level,
			treatment, treatment_plan, treatment_due_date,
			accepted_at, accepted_by_id,
			owner_id, status, last_review, next_review, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
			$22, $23, $24,
			$25, $26, $27,
			$28, $29,
			(SELECT id FROM users WHERE email = $30), $31, $32, $33, $34)
		RETURNING id, created_at, updated_at
	`, orgID, r.Identifier, r.Title, nilIfEmpty(r.Description), r.RiskType, r.Origin, nilIfEmpty(r.Category),
		r.CurrentLikelihood, r.CurrentImpact, r.CurrentScore, nilIfEmpty(r.CurrentLevel),
		r.ConfidentialityImpact, r.IntegrityImpact, r.AvailabilityImpact,
		r.InherentLikelihood, r.InherentImpact, r.InherentScore,
		r.InherentConfidentialityImpact, r.InherentIntegrityImpact, r.InherentAvailabilityImpact,
		r.TargetLikelihood, r.TargetImpact, r.TargetScore, nilIfEmpty(r.TargetLevel),
		nilIfEmpty(r.Treatment), nilIfEmpty(r.TreatmentPlan), r.TreatmentDueDate,
		r.AcceptedAt, r.AcceptedByID,
		r.Owner, r.Status, r.LastReview, r.NextReview, nilIfEmpty(r.Notes),
	).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
}

// UpdateRiskTx updates a risk within an existing transaction.
func UpdateRiskTx(ctx context.Context, tx pgx.Tx, orgID int, r *Risk) error {
	r.CalculateScore(nil)
	_, err := tx.Exec(ctx, `
		UPDATE risks SET title = $2, description = $3, risk_type = $4, origin = $5, category = $6,
			current_likelihood = $7, current_impact = $8, current_score = $9, current_level = $10,
			confidentiality_impact = $11, integrity_impact = $12, availability_impact = $13,
			inherent_likelihood = $14, inherent_impact = $15, inherent_score = $16,
			inherent_confidentiality_impact = $17, inherent_integrity_impact = $18, inherent_availability_impact = $19,
			target_likelihood = $20, target_impact = $21, target_score = $22, target_level = $23,
			treatment = $24, treatment_plan = $25, treatment_due_date = $26,
			accepted_at = $27, accepted_by_id = $28,
			owner_id = (SELECT id FROM users WHERE email = $29), status = $30, last_review = $31, next_review = $32,
			notes = $33, updated_at = now()
		WHERE id = $1 AND organization_id = $34 AND deleted_at IS NULL
	`, r.ID, r.Title, nilIfEmpty(r.Description), r.RiskType, r.Origin, nilIfEmpty(r.Category),
		r.CurrentLikelihood, r.CurrentImpact, r.CurrentScore, nilIfEmpty(r.CurrentLevel),
		r.ConfidentialityImpact, r.IntegrityImpact, r.AvailabilityImpact,
		r.InherentLikelihood, r.InherentImpact, r.InherentScore,
		r.InherentConfidentialityImpact, r.InherentIntegrityImpact, r.InherentAvailabilityImpact,
		r.TargetLikelihood, r.TargetImpact, r.TargetScore, nilIfEmpty(r.TargetLevel),
		nilIfEmpty(r.Treatment), nilIfEmpty(r.TreatmentPlan), r.TreatmentDueDate,
		r.AcceptedAt, r.AcceptedByID,
		nilIfEmpty(r.Owner), r.Status, r.LastReview, r.NextReview,
		nilIfEmpty(r.Notes), orgID)
	return err
}

// CreateIncidentTx creates an incident within an existing transaction.
func CreateIncidentTx(ctx context.Context, tx pgx.Tx, orgID int, inc *Incident) error {
	inc.OrganizationID = orgID
	ident, err := nextIdentifierTx(ctx, tx, orgID, "incident")
	if err != nil {
		return err
	}
	inc.Identifier = ident
	return tx.QueryRow(ctx, `
		INSERT INTO incidents (organization_id, identifier, title, description, severity, status,
			affects_c, affects_i, affects_a,
			incident_type, source, notes, data_breach, gdpr_role,
			authority_notified, subjects_notified,
			reporter, reporter_user_id, assignee_id, detected_at,
			root_cause, lessons_learned)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16,
			$17, (SELECT id FROM users WHERE email = $17), (SELECT id FROM users WHERE email = $18), $19, $20, $21)
		RETURNING id, created_at, updated_at
	`, orgID, inc.Identifier, inc.Title, inc.Description, inc.Severity, inc.Status,
		inc.AffectsC, inc.AffectsI, inc.AffectsA,
		inc.IncidentType, inc.Source, nilIfEmpty(inc.Notes), inc.DataBreach, nilIfEmpty(inc.GDPRRole),
		inc.AuthorityNotified, inc.SubjectsNotified,
		inc.Reporter, inc.Assignee,
		inc.DetectedAt, nilIfEmpty(inc.RootCause), nilIfEmpty(inc.LessonsLearned),
	).Scan(&inc.ID, &inc.CreatedAt, &inc.UpdatedAt)
}

// UpdateIncidentTx updates an incident within an existing transaction.
func UpdateIncidentTx(ctx context.Context, tx pgx.Tx, orgID int, inc *Incident) error {
	_, err := tx.Exec(ctx, `
		UPDATE incidents SET title = $2, description = $3, severity = $4, status = $5,
			affects_c = $6, affects_i = $7, affects_a = $8,
			incident_type = $9, source = $10,
			notes = $11, data_breach = $12, gdpr_role = $13,
			authority_notified = $14, authority_notified_at = $15,
			subjects_notified = $16, subjects_notified_at = $17,
			assignee_id = CASE WHEN $18 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $18) END,
			root_cause = $19, lessons_learned = $20,
			updated_at = now()
		WHERE id = $1 AND organization_id = $21 AND deleted_at IS NULL
	`, inc.ID, inc.Title, inc.Description, inc.Severity, inc.Status,
		inc.AffectsC, inc.AffectsI, inc.AffectsA,
		inc.IncidentType, inc.Source,
		nilIfEmpty(inc.Notes), inc.DataBreach, nilIfEmpty(inc.GDPRRole),
		inc.AuthorityNotified, inc.AuthorityNotifiedAt,
		inc.SubjectsNotified, inc.SubjectsNotifiedAt,
		inc.Assignee,
		nilIfEmpty(inc.RootCause), nilIfEmpty(inc.LessonsLearned),
		orgID)
	return err
}

// CountOpenCAsByIncidentTx is the transaction-aware twin of CountOpenCAsByIncident
// (corrective_actions.go). Used by the unified incident write path (#26) so the
// open-CA guard runs identically from the HTTP handler and suggestion-apply.
func CountOpenCAsByIncidentTx(ctx context.Context, tx pgx.Tx, orgID int, incidentIdentifier string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT ca.id) FROM corrective_actions ca
		JOIN entity_references r ON r.organization_id = ca.organization_id
		WHERE ca.organization_id = $1
		  AND ca.status != 'resolved'
		  AND ca.deleted_at IS NULL
		  AND (
		    (r.source_type = 'corrective_action' AND r.source_id = ca.identifier
		      AND r.target_type = 'incident' AND r.target_id = $2)
		    OR
		    (r.target_type = 'corrective_action' AND r.target_id = ca.identifier
		      AND r.source_type = 'incident' AND r.source_id = $2)
		  )
	`, orgID, incidentIdentifier).Scan(&n)
	return n, err
}

// SetIncidentLifecycleTx stamps/clears the lifecycle timestamps for a status,
// mirroring UpdateIncidentStatusWithDetails (incidents.go) but on a transaction.
// UpdateIncidentTx writes status but NOT the timestamps, so the unified write
// path (#26) calls this right after it to keep contained/resolved/closed_at
// correct on both forward transitions and reopens.
func SetIncidentLifecycleTx(ctx context.Context, tx pgx.Tx, orgID, id int, status string) error {
	query := `UPDATE incidents SET updated_at = now()`
	switch status {
	case "draft", "open", "investigating":
		query += `, contained_at = NULL, resolved_at = NULL, closed_at = NULL`
	case "contained":
		query += `, contained_at = COALESCE(contained_at, now()), resolved_at = NULL, closed_at = NULL`
	case "resolved":
		query += `, resolved_at = COALESCE(resolved_at, now()), closed_at = NULL`
	case "closed":
		query += `, closed_at = COALESCE(closed_at, now())`
	}
	query += ` WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`
	_, err := tx.Exec(ctx, query, id, orgID)
	return err
}

// CountOpenTasksByCATx is the transaction-aware twin of CountOpenTasksByCA
// (corrective_actions.go). Used by the unified CA write path (#26) so the
// open-task guard runs identically from the HTTP handler and suggestion-apply.
func CountOpenTasksByCATx(ctx context.Context, tx pgx.Tx, orgID int, caIdentifier string) (int, error) {
	var n int
	err := tx.QueryRow(ctx,
		`SELECT count(*) FROM tasks WHERE organization_id = $1
		   AND task_type = 'ca_followup'
		   AND status NOT IN ('done','cancelled')
		   AND deleted_at IS NULL
		   AND (title LIKE $2 OR COALESCE(description,'') LIKE $2)`,
		orgID, "%"+caIdentifier+"%").Scan(&n)
	return n, err
}

// SetCorrectiveActionResolvedTx stamps resolved_at / resolved_by_id, mirroring
// UpdateCorrectiveActionStatus's resolved branch (corrective_actions.go) but on a
// transaction. UpdateCorrectiveActionTx writes status but NOT this closure
// metadata, so the unified CA write path (#26) calls this on a →resolved
// transition — the exact field suggestion-apply previously skipped.
func SetCorrectiveActionResolvedTx(ctx context.Context, tx pgx.Tx, orgID, id int, actor string) error {
	_, err := tx.Exec(ctx, `
		UPDATE corrective_actions
		SET resolved_at = now(), resolved_by_id = (SELECT id FROM users WHERE email = $3), updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID, actor)
	return err
}

// CreateReferenceTx creates an entity reference within an existing transaction.
// Idempotent: on conflict returns the existing row.
func CreateReferenceTx(ctx context.Context, tx pgx.Tx, orgID int, ref *EntityReference) error {
	ref.OrganizationID = orgID
	return tx.QueryRow(ctx, `
		INSERT INTO entity_references (organization_id, source_type, source_id, target_type, target_id, created_by, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, (SELECT id FROM users WHERE email = $6))
		ON CONFLICT (organization_id, source_type, source_id, target_type, target_id)
		DO UPDATE SET created_at = entity_references.created_at
		RETURNING id, created_at
	`, orgID, ref.SourceType, ref.SourceID, ref.TargetType, ref.TargetID, ref.CreatedBy,
	).Scan(&ref.ID, &ref.CreatedAt)
}

// LogChangeTx inserts a single changelog entry within an existing transaction.
func LogChangeTx(ctx context.Context, tx pgx.Tx, orgID int, entry *ChangelogEntry) error {
	entry.OrganizationID = orgID
	return tx.QueryRow(ctx, `
		INSERT INTO entity_changelog (organization_id, entity_type, entity_id, action, field, old_value, new_value, changed_by, changed_by_user_id, api_key_id, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, (SELECT id FROM users WHERE email = $8), $9, $10)
		RETURNING id, created_at
	`, orgID, entry.EntityType, entry.EntityID, entry.Action,
		nilIfEmpty(entry.Field), entry.OldValue, entry.NewValue,
		entry.ChangedBy, entry.APIKeyID, nilIfEmpty(entry.Reason),
	).Scan(&entry.ID, &entry.CreatedAt)
}

// LogChangesTx inserts multiple changelog entries within an existing transaction.
func LogChangesTx(ctx context.Context, tx pgx.Tx, orgID int, entries []ChangelogEntry) error {
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

	_, err := tx.Exec(ctx, b.String(), args...)
	return err
}

// CreateSupplierTx creates a supplier within an existing transaction.
func CreateSupplierTx(ctx context.Context, tx pgx.Tx, orgID int, s *Supplier) error {
	s.OrganizationID = orgID
	s.CalculateNextReview()

	var seq int
	err := tx.QueryRow(ctx, `
		INSERT INTO identifier_sequences (organization_id, entity_type, next_value)
		VALUES ($1, 'supplier', 1)
		ON CONFLICT (organization_id, entity_type)
		DO UPDATE SET next_value = identifier_sequences.next_value + 1
		RETURNING next_value
	`, orgID).Scan(&seq)
	if err != nil {
		return fmt.Errorf("allocate identifier: %w", err)
	}
	s.Identifier = fmt.Sprintf("SUPPLIER-%d", seq)

	return tx.QueryRow(ctx, `
		INSERT INTO suppliers (organization_id, identifier, name, supplier_type, criticality,
			data_access, contact, contract_ref,
			status, owner_id, contract_expiry,
			confidentiality, integrity, availability,
			last_review, next_review, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8,
			$9,
			CASE WHEN $10 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $10) END,
			$11,
			$12, $13, $14,
			$15, $16, $17)
		RETURNING id, created_at, updated_at
	`, orgID, s.Identifier, s.Name, s.SupplierType, s.Criticality,
		s.DataAccess, nilIfEmpty(s.Contact), nilIfEmpty(s.ContractRef),
		nilIfEmpty(s.Status), s.Owner, s.ContractExpiry,
		s.Confidentiality, s.Integrity, s.Availability,
		s.LastReview, s.NextReview, nilIfEmpty(s.Notes),
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

// UpdateSupplierTx updates a supplier within an existing transaction.
func UpdateSupplierTx(ctx context.Context, tx pgx.Tx, orgID int, s *Supplier) error {
	s.CalculateNextReview()
	_, err := tx.Exec(ctx, `
		UPDATE suppliers SET name = $2, supplier_type = $3, criticality = $4,
			data_access = $5, contact = $6, contract_ref = $7,
			status = $8,
			owner_id = CASE WHEN $9 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $9) END,
			contract_expiry = $10,
			confidentiality = $11, integrity = $12, availability = $13,
			last_review = $14, next_review = $15,
			notes = $16, updated_at = now()
		WHERE id = $1 AND organization_id = $17 AND deleted_at IS NULL
	`, s.ID, s.Name, s.SupplierType, s.Criticality,
		s.DataAccess, nilIfEmpty(s.Contact), nilIfEmpty(s.ContractRef),
		nilIfEmpty(s.Status), s.Owner, s.ContractExpiry,
		s.Confidentiality, s.Integrity, s.Availability,
		s.LastReview, s.NextReview,
		nilIfEmpty(s.Notes), orgID)
	return err
}

// CreateLegalRequirementTx creates a legal requirement within an existing transaction.
func CreateLegalRequirementTx(ctx context.Context, tx pgx.Tx, orgID int, lr *LegalRequirement) error {
	lr.OrganizationID = orgID
	lr.CalculateRiskScore(nil)

	var seq int
	err := tx.QueryRow(ctx, `
		INSERT INTO identifier_sequences (organization_id, entity_type, next_value)
		VALUES ($1, 'legal', 1)
		ON CONFLICT (organization_id, entity_type)
		DO UPDATE SET next_value = identifier_sequences.next_value + 1
		RETURNING next_value
	`, orgID).Scan(&seq)
	if err != nil {
		return fmt.Errorf("allocate identifier: %w", err)
	}
	lr.Identifier = fmt.Sprintf("LEGAL-%d", seq)

	return tx.QueryRow(ctx, `
		INSERT INTO legal_requirements (organization_id, identifier, `+legalCols+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
			CASE WHEN $10 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $10) END,
			$11, $12, $13,
			$14, $15, $16, $17, $18, $19,
			$20, $21, $22)
		RETURNING id, created_at, updated_at
	`, orgID, lr.Identifier, lr.Title, nilIfEmpty(lr.Description), lr.Jurisdiction, lr.Category,
		nilIfEmpty(lr.Reference), nilIfEmpty(lr.URL),
		lr.Status,
		lr.Owner,
		lr.LastReview, lr.NextReview,
		nilIfEmpty(lr.Notes),
		lr.CurrentLikelihood, lr.CurrentImpact, lr.CurrentScore, nilIfEmpty(lr.CurrentLevel), nilIfEmpty(lr.Treatment), nilIfEmpty(lr.TreatmentPlan),
		lr.TargetLikelihood, lr.TargetImpact, lr.Completion,
	).Scan(&lr.ID, &lr.CreatedAt, &lr.UpdatedAt)
}

// UpdateLegalRequirementTx updates a legal requirement within an existing transaction.
func UpdateLegalRequirementTx(ctx context.Context, tx pgx.Tx, orgID int, lr *LegalRequirement) error {
	lr.CalculateRiskScore(nil)
	_, err := tx.Exec(ctx, `
		UPDATE legal_requirements SET title = $2, description = $3, jurisdiction = $4, category = $5,
			reference = $6, url = $7,
			status = $8,
			owner_id = CASE WHEN $9 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $9) END,
			last_review = $10, next_review = $11, notes = $12,
			current_likelihood = $13, current_impact = $14, current_score = $15, current_level = $16, treatment = $17, treatment_plan = $18,
			target_likelihood = $19, target_impact = $20, completion = $21,
			updated_at = now()
		WHERE id = $1 AND organization_id = $22 AND deleted_at IS NULL
	`, lr.ID, lr.Title, nilIfEmpty(lr.Description), lr.Jurisdiction, lr.Category,
		nilIfEmpty(lr.Reference), nilIfEmpty(lr.URL),
		lr.Status,
		lr.Owner,
		lr.LastReview, lr.NextReview,
		nilIfEmpty(lr.Notes),
		lr.CurrentLikelihood, lr.CurrentImpact, lr.CurrentScore, nilIfEmpty(lr.CurrentLevel), nilIfEmpty(lr.Treatment), nilIfEmpty(lr.TreatmentPlan),
		lr.TargetLikelihood, lr.TargetImpact, lr.Completion,
		orgID)
	return err
}

// CreateChangeRequestTx creates a change request within an existing transaction.
func CreateChangeRequestTx(ctx context.Context, tx pgx.Tx, orgID int, cr *ChangeRequest) error {
	cr.OrganizationID = orgID
	ident, err := nextIdentifierTx(ctx, tx, orgID, "change_request")
	if err != nil {
		return err
	}
	cr.Identifier = ident
	return tx.QueryRow(ctx, `
		INSERT INTO change_requests (organization_id, identifier, title, description, justification, priority, category, risk_level, rollback_plan, notes, requested_by_id, assigned_to_id, status, planned_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, (SELECT id FROM users WHERE email = $11),
			CASE WHEN $12 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $12) END, $13, $14)
		RETURNING id, created_at, updated_at
	`, orgID, cr.Identifier, cr.Title, cr.Description, nilIfEmpty(cr.Justification),
		cr.Priority, cr.Category, cr.RiskLevel, nilIfEmpty(cr.RollbackPlan), nilIfEmpty(cr.Notes),
		cr.RequestedBy, cr.AssignedTo, cr.Status, cr.PlannedAt,
	).Scan(&cr.ID, &cr.CreatedAt, &cr.UpdatedAt)
}

// UpdateChangeRequestTx updates a change request within an existing transaction.
func UpdateChangeRequestTx(ctx context.Context, tx pgx.Tx, orgID int, id int, cr *ChangeRequest) error {
	if cr.Type == "" {
		cr.Type = "change"
	}
	_, err := tx.Exec(ctx, `
		UPDATE change_requests SET title = $2, description = $3, justification = $4,
			priority = $5, category = $6, risk_level = $7, rollback_plan = $8, notes = $9,
			assigned_to_id = CASE WHEN $10 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $10) END,
			planned_at = $11, type = $13,
			updated_at = now()
		WHERE id = $1 AND organization_id = $12 AND deleted_at IS NULL
	`, id, cr.Title, cr.Description, nilIfEmpty(cr.Justification),
		cr.Priority, cr.Category, cr.RiskLevel, nilIfEmpty(cr.RollbackPlan), nilIfEmpty(cr.Notes),
		cr.AssignedTo, cr.PlannedAt, orgID, cr.Type)
	return err
}

// CreateCorrectiveActionTx creates a corrective action within an existing transaction.
func CreateCorrectiveActionTx(ctx context.Context, tx pgx.Tx, orgID int, ca *CorrectiveAction) error {
	ca.OrganizationID = orgID
	ident, err := nextIdentifierTx(ctx, tx, orgID, "corrective_action")
	if err != nil {
		return err
	}
	ca.Identifier = ident
	return tx.QueryRow(ctx, `
		INSERT INTO corrective_actions (organization_id, identifier, title, description, source, severity, status,
			assignee_id, created_by, created_by_user_id, due_date, root_cause, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7,
			CASE WHEN $8 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $8) END,
			$9, (SELECT id FROM users WHERE email = $9), $10, $11, $12)
		RETURNING id, created_at, updated_at
	`, orgID, ca.Identifier, ca.Title, ca.Description, ca.Source, ca.Severity, ca.Status,
		ca.Assignee, ca.CreatedBy, ca.DueDate,
		nilIfEmpty(ca.RootCause),
		nilIfEmpty(ca.Notes),
	).Scan(&ca.ID, &ca.CreatedAt, &ca.UpdatedAt)
}

// UpdateCorrectiveActionTx updates a corrective action within an existing transaction.
func UpdateCorrectiveActionTx(ctx context.Context, tx pgx.Tx, orgID int, ca *CorrectiveAction) error {
	_, err := tx.Exec(ctx, `
		UPDATE corrective_actions SET title = $2, description = $3, source = $4, severity = $5, status = $6,
			assignee_id = CASE WHEN $7 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $7) END,
			root_cause = $8,
			notes = $9, updated_at = now()
		WHERE id = $1 AND organization_id = $10 AND deleted_at IS NULL
	`, ca.ID, ca.Title, ca.Description, ca.Source, ca.Severity, ca.Status,
		ca.Assignee,
		nilIfEmpty(ca.RootCause), nilIfEmpty(ca.Notes), orgID)
	return err
}

// CreateTaskTx creates a task within an existing transaction.
func CreateTaskTx(ctx context.Context, tx pgx.Tx, orgID int, t *Task) error {
	t.OrganizationID = orgID
	ident, err := nextIdentifierTx(ctx, tx, orgID, "task")
	if err != nil {
		return err
	}
	t.Identifier = ident
	return tx.QueryRow(ctx, `
		INSERT INTO tasks (organization_id, identifier, title, description, task_type, assignee_id, created_by, created_by_user_id, status, priority, due_date, recurrence_days, notes)
		VALUES ($1, $2, $3, $4, $5,
			CASE WHEN $6 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $6) END,
			$7, (SELECT id FROM users WHERE email = $7), $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`, orgID, t.Identifier, t.Title, t.Description, t.TaskType,
		t.Assignee, t.CreatedBy, t.Status, t.Priority, t.DueDate, t.RecurrenceDays, nilIfEmpty(t.Notes),
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

// UpdateTaskTx updates a task within an existing transaction.
func UpdateTaskTx(ctx context.Context, tx pgx.Tx, orgID int, t *Task) error {
	_, err := tx.Exec(ctx, `
		UPDATE tasks SET
			title = $2, description = $3,
			assignee_id = CASE WHEN $4 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $4) END,
			priority = $5, due_date = $6, task_type = $7, status = $8,
			completed_at = CASE WHEN $8 = 'done' AND completed_at IS NULL THEN now() ELSE completed_at END,
			notes = $9, updated_at = now()
		WHERE id = $1 AND organization_id = $10 AND deleted_at IS NULL
	`, t.ID, t.Title, nilIfEmpty(t.Description), t.Assignee,
		t.Priority, t.DueDate, t.TaskType, t.Status, nilIfEmpty(t.Notes), orgID)
	return err
}

// UpdateObjectiveTx updates an objective within an existing transaction.
func UpdateObjectiveTx(ctx context.Context, tx pgx.Tx, orgID int, o *Objective) error {
	_, err := tx.Exec(ctx, `
		UPDATE objectives SET
			title = $2, description = $3, owner_id = CASE WHEN $4 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $4) END,
			source = $5, measurement_method = $6,
			target_value = $7, target_operator = $8, unit = $9,
			window_seconds = $10, grace_seconds = $11, checkin_cycle = $12, status = $13,
			started_at = $14, notes = $15, updated_at = now()
		WHERE id = $1 AND organization_id = $16 AND deleted_at IS NULL
	`, o.ID, o.Title, nilIfEmpty(o.Description), o.Owner,
		nilIfEmpty(o.Source), nilIfEmpty(o.MeasurementMethod),
		o.TargetValue, o.TargetOperator, nilIfEmpty(o.Unit),
		o.WindowSeconds, o.GraceSeconds, o.CheckinCycle, o.Status,
		o.StartedAt, nilIfEmpty(o.Notes), orgID)
	return err
}

// UpdateSystemTx updates a system within an existing transaction.
func UpdateSystemTx(ctx context.Context, tx pgx.Tx, orgID int, sys *System) error {
	sys.CalculateNextReview()
	if sys.Status == "" {
		sys.Status = "active"
	}
	_, err := tx.Exec(ctx, `
		UPDATE systems SET name = $2, description = $3, supplier_id = $4,
			department = $5, classification = $6, criticality = $7,
			status = $8, rpo_hours = $9, rto_hours = $10,
			confidentiality = $11, integrity = $12, availability = $13,
			last_review = $14, next_review = $15,
			owner_id = CASE WHEN $16 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $16) END,
			notes = $17, updated_at = now()
		WHERE id = $1 AND organization_id = $18 AND deleted_at IS NULL
	`, sys.ID, sys.Name, nilIfEmpty(sys.Description), sys.SupplierID,
		nilIfEmpty(sys.Department),
		sys.Classification, sys.Criticality, sys.Status, sys.RPOHours, sys.RTOHours,
		sys.Confidentiality, sys.Integrity, sys.Availability,
		sys.LastReview, sys.NextReview,
		sys.Owner, nilIfEmpty(sys.Notes), orgID)
	return err
}

// UpdateAssetTx updates an asset within an existing transaction.
func UpdateAssetTx(ctx context.Context, tx pgx.Tx, orgID int, a *Asset) error {
	_, err := tx.Exec(ctx, `
		UPDATE assets SET name = $2, description = $3, asset_type = $4, status = $5,
			owner_id = CASE WHEN $6 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $6) END,
			primary_location = $7,
			confidentiality = $8, integrity = $9, availability = $10,
			last_review = $11, next_review = $12,
			notes = $13, updated_at = now()
		WHERE id = $1 AND organization_id = $14 AND deleted_at IS NULL
	`, a.ID, a.Name, nilIfEmpty(a.Description), a.AssetType, a.Status,
		a.Owner, nilIfEmpty(a.PrimaryLocation),
		a.Confidentiality, a.Integrity, a.Availability,
		a.LastReview, a.NextReview,
		nilIfEmpty(a.Notes), orgID)
	return err
}

// CreateObjectiveTx creates an objective within an existing transaction.
func CreateObjectiveTx(ctx context.Context, tx pgx.Tx, orgID int, o *Objective) error {
	o.OrganizationID = orgID

	// Get program key for display_id generation
	var progKey string
	err := tx.QueryRow(ctx,
		`SELECT key FROM programs WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`,
		o.ProgramID, orgID).Scan(&progKey)
	if err != nil {
		return fmt.Errorf("program not found: %w", err)
	}

	// Get next seq number for this program
	var maxSeq int
	_ = tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(seq_number), 0) FROM objectives WHERE program_id = $1 AND deleted_at IS NULL`,
		o.ProgramID).Scan(&maxSeq)
	o.SeqNumber = maxSeq + 1
	o.DisplayID = fmt.Sprintf("%s-%d", progKey, o.SeqNumber)

	if o.TargetOperator == "" {
		o.TargetOperator = "gte"
	}
	if o.Status == "" {
		o.Status = "draft"
	}
	if o.CheckinCycle <= 0 {
		o.CheckinCycle = 12
	}

	return tx.QueryRow(ctx, `
		INSERT INTO objectives (organization_id, program_id, display_id, seq_number,
			title, description, owner_id, source, measurement_method,
			target_value, target_operator, unit, window_seconds, grace_seconds,
			checkin_cycle, status, started_at, notes)
		VALUES ($1, $2, $3, $4, $5, $6, (SELECT id FROM users WHERE email = $7), $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id, created_at, updated_at
	`, orgID, o.ProgramID, o.DisplayID, o.SeqNumber,
		o.Title, nilIfEmpty(o.Description), nilIfEmpty(o.Owner),
		nilIfEmpty(o.Source), nilIfEmpty(o.MeasurementMethod),
		o.TargetValue, o.TargetOperator, nilIfEmpty(o.Unit),
		o.WindowSeconds, o.GraceSeconds, o.CheckinCycle, o.Status, o.StartedAt, nilIfEmpty(o.Notes),
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
}

// CreateSystemTx creates a system within an existing transaction.
func CreateSystemTx(ctx context.Context, tx pgx.Tx, orgID int, sys *System) error {
	sys.OrganizationID = orgID
	sys.CalculateNextReview()
	if sys.Status == "" {
		sys.Status = "active"
	}

	var seq int
	err := tx.QueryRow(ctx, `
		INSERT INTO identifier_sequences (organization_id, entity_type, next_value)
		VALUES ($1, 'system', 1)
		ON CONFLICT (organization_id, entity_type)
		DO UPDATE SET next_value = identifier_sequences.next_value + 1
		RETURNING next_value
	`, orgID).Scan(&seq)
	if err != nil {
		return fmt.Errorf("allocate identifier: %w", err)
	}
	sys.Identifier = fmt.Sprintf("SYSTEM-%d", seq)

	return tx.QueryRow(ctx, `
		INSERT INTO systems (organization_id, identifier, name, description, supplier_id, department,
			classification, criticality, status, rpo_hours, rto_hours,
			confidentiality, integrity, availability,
			last_review, next_review,
			owner_id, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
			CASE WHEN $17 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $17) END,
			$18)
		RETURNING id, created_at, updated_at
	`, orgID, sys.Identifier, sys.Name, nilIfEmpty(sys.Description), sys.SupplierID,
		nilIfEmpty(sys.Department),
		sys.Classification, sys.Criticality, sys.Status,
		sys.RPOHours, sys.RTOHours,
		sys.Confidentiality, sys.Integrity, sys.Availability,
		sys.LastReview, sys.NextReview,
		sys.Owner, nilIfEmpty(sys.Notes),
	).Scan(&sys.ID, &sys.CreatedAt, &sys.UpdatedAt)
}

// CreateAssetTx creates an asset within an existing transaction.
func CreateAssetTx(ctx context.Context, tx pgx.Tx, orgID int, a *Asset) error {
	a.OrganizationID = orgID

	var seq int
	err := tx.QueryRow(ctx, `
		INSERT INTO identifier_sequences (organization_id, entity_type, next_value)
		VALUES ($1, 'asset', 1)
		ON CONFLICT (organization_id, entity_type)
		DO UPDATE SET next_value = identifier_sequences.next_value + 1
		RETURNING next_value
	`, orgID).Scan(&seq)
	if err != nil {
		return fmt.Errorf("allocate identifier: %w", err)
	}
	a.Identifier = fmt.Sprintf("AST-%d", seq)

	return tx.QueryRow(ctx, `
		INSERT INTO assets (organization_id, identifier, name, description, asset_type, status,
			owner_id, primary_location, confidentiality, integrity, availability,
			last_review, next_review, notes)
		VALUES ($1, $2, $3, $4, $5, $6,
			CASE WHEN $7 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $7) END,
			$8, $9, $10, $11,
			$12, $13, $14)
		RETURNING id, created_at, updated_at
	`, orgID, a.Identifier, a.Name, nilIfEmpty(a.Description), a.AssetType, a.Status,
		a.Owner, nilIfEmpty(a.PrimaryLocation),
		a.Confidentiality, a.Integrity, a.Availability,
		a.LastReview, a.NextReview, nilIfEmpty(a.Notes),
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

// AddAuditFindingTx creates an audit finding within an existing transaction.
func AddAuditFindingTx(ctx context.Context, tx pgx.Tx, orgID int, f *AuditFinding) error {
	f.OrganizationID = orgID
	return tx.QueryRow(ctx, `
		INSERT INTO audit_findings (organization_id, audit_id, audit_item_id, finding_type, title, description,
			status, due_date, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8,
			CASE WHEN $9 = '' THEN NULL ELSE (SELECT id FROM users WHERE email = $9) END)
		RETURNING id, created_at, updated_at
	`, orgID, f.AuditID, f.AuditItemID, f.FindingType, f.Title, f.Description,
		f.Status, f.DueDate, f.Owner,
	).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
}

// UpdateAuditFindingFieldTx updates a single audit finding field within an existing transaction.
// Restricted to text fields that the AI suggestion apply-handler may modify; stamps updated_at.
// Note: corrective_action content is now folded into description (## Corrective Action heading).
func UpdateAuditFindingFieldTx(ctx context.Context, tx pgx.Tx, orgID int, id int, field, value string) error {
	allowed := map[string]bool{"description": true, "title": true}
	if !allowed[field] {
		return fmt.Errorf("field %s not updatable", field)
	}
	_, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE audit_findings SET %s = $2, updated_at = now() WHERE id = $1 AND organization_id = $3 AND deleted_at IS NULL`, field), id, value, orgID)
	return err
}

// --- Review / approval Tx variants ---

// AddApprovalTx inserts an approval record within an existing transaction.
func AddApprovalTx(ctx context.Context, tx pgx.Tx, orgID int, a *Approval) error {
	a.OrganizationID = orgID
	return tx.QueryRow(ctx, `
		INSERT INTO approvals (organization_id, review_id, document_id, version, round, decision, approved_by, approved_by_user_id, comment)
		VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT id FROM users WHERE email = $7), $8)
		RETURNING id, created_at
	`, orgID, a.ReviewID, a.DocumentID, a.Version, a.Round, a.Decision, a.ApprovedBy, a.Comment,
	).Scan(&a.ID, &a.CreatedAt)
}

// CreateDecisionRecordTx inserts an immutable decision record within an existing transaction.
func CreateDecisionRecordTx(ctx context.Context, tx pgx.Tx, orgID int, rec *DecisionRecord) error {
	rec.OrganizationID = orgID
	return tx.QueryRow(ctx, `
		INSERT INTO decision_log (organization_id, review_id, document_id, decision, decided_by, decided_by_id, commit_ref, version, comment, content_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`, orgID, rec.ReviewID, rec.DocumentID, rec.Decision, rec.DecidedBy, rec.DecidedByID, rec.CommitRef, rec.Version, rec.Comment, rec.ContentHash,
	).Scan(&rec.ID, &rec.CreatedAt)
}

// CreateEntityReadingTx creates an entity reading within an existing transaction.
func CreateEntityReadingTx(ctx context.Context, tx pgx.Tx, orgID int, r *EntityReading) error {
	r.OrganizationID = orgID
	return tx.QueryRow(ctx, `
		INSERT INTO entity_readings (organization_id, entity_type, entity_id,
			current_likelihood, current_impact, confidentiality, integrity, availability,
			status, treatment, notes,
			assessed_by, assessed_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, (SELECT id FROM users WHERE email = $12))
		RETURNING id, created_at
	`, orgID, r.EntityType, r.EntityID,
		r.CurrentLikelihood, r.CurrentImpact, r.Confidentiality, r.Integrity, r.Availability,
		nilIfEmpty(r.Status), nilIfEmpty(r.Treatment), nilIfEmpty(r.Notes),
		r.AssessedBy,
	).Scan(&r.ID, &r.CreatedAt)
}

// UpdateAssignmentStatusTx updates a review assignment status within an existing transaction.
func UpdateAssignmentStatusTx(ctx context.Context, tx pgx.Tx, orgID int, id int, status string) error {
	_, err := tx.Exec(ctx, `
		UPDATE review_assignments SET status = $2, reviewed_at = now() WHERE id = $1 AND organization_id = $3
	`, id, status, orgID)
	return err
}

// SetMergeCommitTx stores the merge commit hash within an existing transaction.
func SetMergeCommitTx(ctx context.Context, tx pgx.Tx, orgID int, id int, mergeCommit string) error {
	_, err := tx.Exec(ctx, `UPDATE reviews SET merge_commit = $2, updated_at = now() WHERE id = $1 AND organization_id = $3`, id, mergeCommit, orgID)
	return err
}

// UpdateReviewStatusTx updates a review status within an existing transaction.
func UpdateReviewStatusTx(ctx context.Context, tx pgx.Tx, orgID int, id int, status string) error {
	_, err := tx.Exec(ctx, `UPDATE reviews SET status = $2, updated_at = now() WHERE id = $1 AND organization_id = $3`, id, status, orgID)
	return err
}

// ListAssignmentsForReviewTx lists review assignments within an existing transaction.
func ListAssignmentsForReviewTx(ctx context.Context, tx pgx.Tx, orgID int, reviewID int) ([]ReviewAssignment, error) {
	rows, err := tx.Query(ctx, `
		SELECT ra.id, ra.organization_id, ra.review_id, u.email, ra.status, ra.reviewed_at, ra.created_at
		FROM review_assignments ra JOIN users u ON u.id = ra.reviewer_id
		WHERE ra.organization_id = $1 AND ra.review_id = $2
		ORDER BY ra.created_at
	`, orgID, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []ReviewAssignment
	for rows.Next() {
		var a ReviewAssignment
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.ReviewID, &a.Reviewer, &a.Status, &a.ReviewedAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// MarkCommentsOutdatedTx marks all non-outdated comments as outdated for a review.
func MarkCommentsOutdatedTx(ctx context.Context, tx pgx.Tx, orgID int, reviewID int) error {
	_, err := tx.Exec(ctx, `UPDATE comments SET is_outdated = true WHERE review_id = $1 AND organization_id = $2 AND is_outdated = false`, reviewID, orgID)
	return err
}

// ResubmitReviewTx updates a review for resubmission: new sent_head, status back to open, round++.
func ResubmitReviewTx(ctx context.Context, tx pgx.Tx, orgID int, reviewID int, sentHead string) error {
	_, err := tx.Exec(ctx, `UPDATE reviews SET sent_head = $2, status = 'open', round = round + 1, updated_at = now() WHERE id = $1 AND organization_id = $3`, reviewID, sentHead, orgID)
	return err
}

// ResetAssignmentsTx resets all review assignments back to pending.
func ResetAssignmentsTx(ctx context.Context, tx pgx.Tx, orgID int, reviewID int) error {
	_, err := tx.Exec(ctx, `UPDATE review_assignments SET status = 'pending', reviewed_at = NULL WHERE review_id = $1 AND organization_id = $2`, reviewID, orgID)
	return err
}

// AddReviewAssignmentTx adds a reviewer assignment within an existing transaction.
// Idempotent: duplicate assignments are silently ignored.
func AddReviewAssignmentTx(ctx context.Context, tx pgx.Tx, orgID int, reviewID int, reviewerEmail string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO review_assignments (organization_id, review_id, reviewer_id, status)
		VALUES ($1, $2, (SELECT id FROM users WHERE email = $3), $4)
		ON CONFLICT (review_id, reviewer_id) DO NOTHING
	`, orgID, reviewID, reviewerEmail, "pending")
	return err
}

// CreateReviewTx creates a review record within an existing transaction.
func CreateReviewTx(ctx context.Context, tx pgx.Tx, orgID int, r *Review) error {
	r.OrganizationID = orgID
	return tx.QueryRow(ctx, `
		INSERT INTO reviews (organization_id, document_id, document_type, title, version, commit_hash, sent_head, requested_by_id, message, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT id FROM users WHERE email = $8), $9, $10)
		RETURNING id, round, created_at, updated_at
	`, orgID, r.DocumentID, r.DocumentType, r.Title, r.Version, r.CommitHash, r.SentHead,
		r.RequestedBy, r.Message, r.Status,
	).Scan(&r.ID, &r.Round, &r.CreatedAt, &r.UpdatedAt)
}

// RecordVersionTx inserts a document version snapshot within an existing transaction.
func RecordVersionTx(ctx context.Context, tx pgx.Tx, orgID int, v *DocumentVersion) error {
	v.OrganizationID = orgID
	return tx.QueryRow(ctx, `
		INSERT INTO document_versions (organization_id, document_id, version, commit_hash, file_path, content_hash, message, owner, review_cycle_months, created_by, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, (SELECT id FROM users WHERE email = $10))
		ON CONFLICT (organization_id, document_id, version) DO UPDATE SET
			commit_hash = EXCLUDED.commit_hash,
			file_path = EXCLUDED.file_path,
			content_hash = EXCLUDED.content_hash,
			message = EXCLUDED.message,
			owner = EXCLUDED.owner,
			review_cycle_months = EXCLUDED.review_cycle_months,
			created_by = EXCLUDED.created_by,
			created_by_user_id = EXCLUDED.created_by_user_id
		RETURNING id, created_at
	`, orgID, v.DocumentID, v.Version, v.CommitHash, v.FilePath, v.ContentHash, v.Message,
		nilIfEmpty(v.Owner), v.ReviewCycleMonths, v.CreatedBy,
	).Scan(&v.ID, &v.CreatedAt)
}
