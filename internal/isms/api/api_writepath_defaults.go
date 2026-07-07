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
