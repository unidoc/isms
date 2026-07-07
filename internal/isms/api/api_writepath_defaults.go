package api

import "isms.sh/internal/isms/db"

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
