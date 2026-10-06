package api

import (
	"context"
	"fmt"

	"isms.sh/internal/isms/db"
)

// Shared create-time defaults (#26, slice A: create consistency). Each helper
// encodes the canonical server-side defaults for a new entity so that a record
// created via suggestion-apply lands in the SAME state as one created over HTTP —
// both the HTTP create handler and the applyXCreate suggestion handler call these,
// instead of each keeping its own (previously divergent) inline defaults.
//
// actor is the creating user's email, used for the owner fallback.

func applyAssetDefaults(a *db.Asset, actor string) {
	if a.Status == "" {
		a.Status = "open"
	}
	if a.AssetType == "" {
		a.AssetType = "other"
	}
	if a.Owner == "" {
		a.Owner = actor
	}
}

func applySupplierDefaults(sup *db.Supplier, actor string) {
	if sup.Status == "" {
		sup.Status = "active"
	}
	if sup.SupplierType == "" {
		sup.SupplierType = "other"
	}
	if sup.Criticality == "" {
		sup.Criticality = "medium"
	}
	if sup.Owner == "" {
		sup.Owner = actor
	}
	if sup.Notes == "" {
		sup.Notes = "## Services\n\n"
	}
}

func applyLegalDefaults(lr *db.LegalRequirement, actor string) {
	if lr.Jurisdiction == "" {
		lr.Jurisdiction = "EU"
	}
	if lr.Category == "" {
		lr.Category = "privacy"
	}
	if lr.Status == "" {
		lr.Status = "open"
	}
	if lr.Owner == "" {
		lr.Owner = actor
	}
}

func applySystemDefaults(sys *db.System, actor string) {
	if sys.Status == "" {
		sys.Status = "active"
	}
	if sys.Classification == "" {
		sys.Classification = "internal"
	}
	if sys.Criticality == "" {
		sys.Criticality = "medium"
	}
	if sys.Owner == "" {
		sys.Owner = actor
	}
	if sys.Description == "" {
		sys.Description = "## Purpose\n\n"
	}
	if sys.Notes == "" {
		sys.Notes = "## Access control\n\n"
	}
}

func applyObjectiveDefaults(o *db.Objective, actor string) {
	if o.Status == "" {
		o.Status = "draft"
	}
	if o.Owner == "" {
		o.Owner = actor
	}
}

// Shared create-time enum validation (#26): both the HTTP create handler and the
// applyXCreate suggestion handler call these (after applyXDefaults), so an invalid
// value fails the same way on both paths — a clean 400 with the allowed list,
// rather than HTTP 400 vs a raw CHECK-violation 500 on apply. Mirrors exactly the
// validateEnum calls the HTTP handlers used inline.

func validateAssetCreate(a *db.Asset) error {
	if err := validateEnum("status", a.Status, db.AssetStatuses); err != nil {
		return err
	}
	return validateEnum("asset_type", a.AssetType, db.AssetTypes)
}

func validateSupplierCreate(sup *db.Supplier) error {
	if err := validateEnum("status", sup.Status, db.SupplierStatuses); err != nil {
		return err
	}
	if err := validateEnum("supplier_type", sup.SupplierType, db.SupplierTypes); err != nil {
		return err
	}
	return validateEnum("criticality", sup.Criticality, db.CriticalityLevels)
}

func validateLegalCreate(lr *db.LegalRequirement) error {
	if err := validateEnum("status", lr.Status, db.LegalStatuses); err != nil {
		return err
	}
	if err := validateEnum("treatment", lr.Treatment, db.LegalTreatments); err != nil {
		return err
	}
	return validateEnum("category", lr.Category, db.LegalCategories)
}

func validateSystemCreate(sys *db.System) error {
	if err := validateEnum("status", sys.Status, db.SystemStatuses); err != nil {
		return err
	}
	if err := validateEnum("criticality", sys.Criticality, db.SystemCriticalities); err != nil {
		return err
	}
	return validateEnum("classification", sys.Classification, db.SystemClassifications)
}

func validateObjectiveCreate(o *db.Objective) error {
	if err := validateEnum("status", o.Status, db.ObjectiveStatuses); err != nil {
		return err
	}
	return validateEnum("target_operator", o.TargetOperator, db.ObjectiveTargetOperators)
}

// applyRiskDefaults fills what the light create form leaves out, so a risk
// created by suggestion lands in the same state as one created over HTTP (#200).
func applyRiskDefaults(r *db.Risk, actor string) {
	if r.Owner == "" {
		r.Owner = actor
	}
	// Sensible defaults so the light create form (title + category) just works.
	// User refines via the edit modal if these aren't right.
	if r.Status == "" {
		r.Status = "open"
	}
	if r.RiskType == "" {
		r.RiskType = "threat"
	}
	if r.Origin == "" {
		r.Origin = "internal"
	}
	// Seed description with section headings when empty, so the user has clear
	// places to fill in both the risk description and its potential consequences.
	if r.Description == "" {
		r.Description = "## Description\n\n\n\n## Potential consequences\n\n"
	}
}

// validateRiskCreate checks the enum fields of a new risk (after defaults).
func validateRiskCreate(r *db.Risk, categories []string) error {
	if err := validateEnum("status", r.Status, db.RiskStatuses); err != nil {
		return err
	}
	if err := validateEnum("risk_type", r.RiskType, db.RiskTypes); err != nil {
		return err
	}
	if err := validateEnum("origin", r.Origin, db.RiskOrigins); err != nil {
		return err
	}
	if err := validateEnum("category", r.Category, categories); err != nil {
		return err
	}
	return validateEnum("treatment", r.Treatment, db.TreatmentOptions)
}

// applyIncidentDefaults fills the system-managed and defaulted fields of a new
// incident. Shared by handleCreateIncident and applyIncidentCreate (#200).
func (s *Server) applyIncidentDefaults(ctx context.Context, orgID int, inc *db.Incident, actor string) {
	// Server-side overwrites for system-managed fields.
	if inc.Reporter == "" {
		inc.Reporter = actor
	}
	if inc.Assignee == "" {
		inc.Assignee = inc.Reporter
	}
	if inc.Status == "" {
		inc.Status = "open"
	}
	if inc.Severity == "" {
		inc.Severity = "medium"
	}
	if inc.IncidentType == "" {
		inc.IncidentType = "event"
	}
	if inc.Source == "" {
		inc.Source = "internal"
	}
	if inc.AuthorityNotified == "" {
		inc.AuthorityNotified = "not_required"
	}
	if inc.SubjectsNotified == "" {
		inc.SubjectsNotified = "not_required"
	}
	if inc.DetectedAt.IsZero() {
		inc.DetectedAt = db.EpochNow()
	}

	// Seed Notes with timeline template if empty.
	if inc.Notes == "" {
		displayName := inc.Reporter
		if u, err := s.db.GetUserByEmail(ctx, inc.Reporter); err == nil && u != nil && u.Name != "" {
			displayName = u.Name
		}
		ts := inc.DetectedAt.Format("2006-01-02 15:04")
		inc.Notes = fmt.Sprintf("## Timeline\n\n- %s — Incident raised by %s\n", ts, displayName)
	}
}

// validateIncidentCreate checks the enum fields of a new incident (after defaults).
func validateIncidentCreate(inc *db.Incident) error {
	if err := validateEnum("status", inc.Status, db.IncidentStatuses); err != nil {
		return err
	}
	if err := validateEnum("severity", inc.Severity, db.IncidentSeverities); err != nil {
		return err
	}
	if err := validateEnum("incident_type", inc.IncidentType, db.IncidentTypes); err != nil {
		return err
	}
	if err := validateEnum("source", inc.Source, db.IncidentSources); err != nil {
		return err
	}
	if err := validateEnum("gdpr_role", inc.GDPRRole, db.GDPRRoles); err != nil {
		return err
	}
	if err := validateEnum("authority_notified", inc.AuthorityNotified, db.AuthorityNotifyVals); err != nil {
		return err
	}
	if err := validateEnum("subjects_notified", inc.SubjectsNotified, db.AuthorityNotifyVals); err != nil {
		return err
	}
	return nil
}

// applyChangeDefaults fills the defaults of a new change request; the caller sets
// RequestedBy first. Shared by handleCreateChange and applyChangeCreate (#200).
func applyChangeDefaults(cr *db.ChangeRequest) {
	if cr.AssignedTo == "" {
		cr.AssignedTo = cr.RequestedBy
	}
	if cr.Type == "" {
		cr.Type = "change"
	}
	if cr.Status == "" {
		cr.Status = "proposed"
	}
	if cr.Priority == "" {
		cr.Priority = "medium"
	}
	if cr.Category == "" {
		cr.Category = "process"
	}
	if cr.RiskLevel == "" {
		cr.RiskLevel = "low"
	}
}

// validateChangeCreate checks the enum fields of a new change request (after defaults).
func validateChangeCreate(cr *db.ChangeRequest) error {
	if err := validateEnum("type", cr.Type, db.ChangeTypes); err != nil {
		return err
	}
	if err := validateEnum("status", cr.Status, db.ChangeStatuses); err != nil {
		return err
	}
	if err := validateEnum("priority", cr.Priority, db.ChangePriorities); err != nil {
		return err
	}
	if err := validateEnum("category", cr.Category, db.ChangeCategories); err != nil {
		return err
	}
	if err := validateEnum("risk_level", cr.RiskLevel, db.ChangeRiskLevels); err != nil {
		return err
	}
	return nil
}

// applyTaskDefaults fills the defaults of a new task. private is the explicit
// flag from the request, or nil to fall back to the org default. Shared by
// handleCreateTask and applyTaskCreate (#200).
func (s *Server) applyTaskDefaults(ctx context.Context, orgID int, t *db.Task, private *bool, actor string) {
	t.CreatedBy = actor // always use authenticated user
	if t.Assignee == "" {
		t.Assignee = t.CreatedBy // default: assign to yourself
	}
	if t.Status == "" {
		t.Status = "open"
	}
	if t.Priority == "" {
		t.Priority = "medium"
	}
	if t.TaskType == "" {
		t.TaskType = "general"
	}
	// due_date stays optional — no auto-default. Users can leave it empty.
	// Visibility: an explicit flag wins; otherwise fall back to the org default
	// (public unless the org has opted into task_default_private).
	if private != nil {
		t.Private = *private
	} else if v, _ := s.db.GetOrgSetting(ctx, orgID, "task_default_private"); v == "true" {
		t.Private = true
	}
}

// validateTaskCreate checks the enum fields of a new task (after defaults).
func validateTaskCreate(t *db.Task) error {
	if err := validateEnum("status", t.Status, db.TaskStatuses); err != nil {
		return err
	}
	if err := validateEnum("priority", t.Priority, db.TaskPriorities); err != nil {
		return err
	}
	return validateEnum("task_type", t.TaskType, db.TaskTypes)
}
