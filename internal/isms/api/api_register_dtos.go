package api

import "isms.sh/internal/isms/db"

// Request DTOs for the four registers (risks, assets, suppliers, systems).
// These replace the previous pattern of binding directly to the db.X structs,
// which made it impossible to distinguish "not set" from "empty string". Update
// DTOs use *string for text and Optional[T] for nullable dates, links and
// numbers, so absent = leave alone, null = clear and a value = apply (#381).
// The one exception is next_review on risks, systems and suppliers (and legal
// requirements): there null means "recalculate from the review cycle".

// --- Assets ---

type assetCreateRequest struct {
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	AssetType       string           `json:"asset_type"`
	Status          string           `json:"status"`
	Owner           string           `json:"owner"`
	PrimaryLocation string           `json:"primary_location"`
	Confidentiality *int             `json:"confidentiality"`
	Integrity       *int             `json:"integrity"`
	Availability    *int             `json:"availability"`
	LastReview      *db.Epoch        `json:"last_review"`
	NextReview      *db.Epoch        `json:"next_review"`
	Notes           string           `json:"notes"`
	ExternalID      string           `json:"external_id"`
	References      []ReferenceInput `json:"references"`
}

type assetUpdateRequest struct {
	Name            *string            `json:"name"`
	Description     *string            `json:"description"`
	AssetType       *string            `json:"asset_type"`
	Status          *string            `json:"status"`
	Owner           *string            `json:"owner"`
	PrimaryLocation *string            `json:"primary_location"`
	Confidentiality Optional[int]      `json:"confidentiality"`
	Integrity       Optional[int]      `json:"integrity"`
	Availability    Optional[int]      `json:"availability"`
	LastReview      Optional[db.Epoch] `json:"last_review"`
	NextReview      Optional[db.Epoch] `json:"next_review"`
	Notes           *string            `json:"notes"`
	ExternalID      *string            `json:"external_id"`
}

// --- Suppliers ---

type supplierCreateRequest struct {
	Name            string           `json:"name"`
	SupplierType    string           `json:"supplier_type"`
	Criticality     string           `json:"criticality"`
	DataAccess      bool             `json:"data_access"`
	Contact         string           `json:"contact"`
	ContractRef     string           `json:"contract_ref"`
	Status          string           `json:"status"`
	Owner           string           `json:"owner"`
	ContractExpiry  *db.Epoch        `json:"contract_expiry"`
	Confidentiality *int             `json:"confidentiality"`
	Integrity       *int             `json:"integrity"`
	Availability    *int             `json:"availability"`
	LastReview      *db.Epoch        `json:"last_review"`
	NextReview      *db.Epoch        `json:"next_review"`
	Notes           string           `json:"notes"`
	ExternalID      string           `json:"external_id"`
	References      []ReferenceInput `json:"references"`
}

type supplierUpdateRequest struct {
	Name            *string            `json:"name"`
	SupplierType    *string            `json:"supplier_type"`
	Criticality     *string            `json:"criticality"`
	DataAccess      *bool              `json:"data_access"`
	Contact         *string            `json:"contact"`
	ContractRef     *string            `json:"contract_ref"`
	Status          *string            `json:"status"`
	Owner           *string            `json:"owner"`
	ContractExpiry  Optional[db.Epoch] `json:"contract_expiry"`
	Confidentiality Optional[int]      `json:"confidentiality"`
	Integrity       Optional[int]      `json:"integrity"`
	Availability    Optional[int]      `json:"availability"`
	LastReview      Optional[db.Epoch] `json:"last_review"`
	NextReview      Optional[db.Epoch] `json:"next_review"`
	Notes           *string            `json:"notes"`
	ExternalID      *string            `json:"external_id"`
}

// --- Systems ---

type systemCreateRequest struct {
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	SupplierID      *int64           `json:"supplier_id"`
	Department      string           `json:"department"`
	Classification  string           `json:"classification"`
	Criticality     string           `json:"criticality"`
	Status          string           `json:"status"`
	RPOHours        int              `json:"rpo_hours"`
	RTOHours        int              `json:"rto_hours"`
	Confidentiality *int             `json:"confidentiality"`
	Integrity       *int             `json:"integrity"`
	Availability    *int             `json:"availability"`
	LastReview      *db.Epoch        `json:"last_review"`
	NextReview      *db.Epoch        `json:"next_review"`
	Owner           string           `json:"owner"`
	Notes           string           `json:"notes"`
	ExternalID      string           `json:"external_id"`
	References      []ReferenceInput `json:"references"`
}

type systemUpdateRequest struct {
	Name            *string            `json:"name"`
	Description     *string            `json:"description"`
	SupplierID      Optional[int64]    `json:"supplier_id"`
	Department      *string            `json:"department"`
	Classification  *string            `json:"classification"`
	Criticality     *string            `json:"criticality"`
	Status          *string            `json:"status"`
	RPOHours        *int               `json:"rpo_hours"`
	RTOHours        *int               `json:"rto_hours"`
	Confidentiality Optional[int]      `json:"confidentiality"`
	Integrity       Optional[int]      `json:"integrity"`
	Availability    Optional[int]      `json:"availability"`
	LastReview      Optional[db.Epoch] `json:"last_review"`
	NextReview      Optional[db.Epoch] `json:"next_review"`
	Owner           *string            `json:"owner"`
	Notes           *string            `json:"notes"`
	ExternalID      *string            `json:"external_id"`
}

// --- Risks ---

type riskCreateRequest struct {
	Title                         string           `json:"title"`
	Description                   string           `json:"description"`
	RiskType                      string           `json:"risk_type"`
	Origin                        string           `json:"origin"`
	Category                      string           `json:"category"`
	CustomFields                  map[string]any   `json:"custom_fields"`
	CurrentLikelihood             *int             `json:"current_likelihood"`
	CurrentImpact                 *int             `json:"current_impact"`
	ConfidentialityImpact         *int             `json:"confidentiality_impact"`
	IntegrityImpact               *int             `json:"integrity_impact"`
	AvailabilityImpact            *int             `json:"availability_impact"`
	InherentLikelihood            *int             `json:"inherent_likelihood"`
	InherentImpact                *int             `json:"inherent_impact"`
	InherentConfidentialityImpact *int             `json:"inherent_confidentiality_impact"`
	InherentIntegrityImpact       *int             `json:"inherent_integrity_impact"`
	InherentAvailabilityImpact    *int             `json:"inherent_availability_impact"`
	TargetLikelihood              *int             `json:"target_likelihood"`
	TargetImpact                  *int             `json:"target_impact"`
	Treatment                     string           `json:"treatment"`
	TreatmentPlan                 string           `json:"treatment_plan"`
	TreatmentDueDate              *db.Epoch        `json:"treatment_due_date"`
	Owner                         string           `json:"owner"`
	Status                        string           `json:"status"`
	LastReview                    *db.Epoch        `json:"last_review"`
	NextReview                    *db.Epoch        `json:"next_review"`
	Notes                         string           `json:"notes"`
	ExternalID                    string           `json:"external_id"`
	References                    []ReferenceInput `json:"references"`
}

type riskUpdateRequest struct {
	Title                         *string            `json:"title"`
	Description                   *string            `json:"description"`
	RiskType                      *string            `json:"risk_type"`
	Origin                        *string            `json:"origin"`
	Category                      *string            `json:"category"`
	CustomFields                  *map[string]any    `json:"custom_fields"`
	CurrentLikelihood             Optional[int]      `json:"current_likelihood"`
	CurrentImpact                 Optional[int]      `json:"current_impact"`
	ConfidentialityImpact         Optional[int]      `json:"confidentiality_impact"`
	IntegrityImpact               Optional[int]      `json:"integrity_impact"`
	AvailabilityImpact            Optional[int]      `json:"availability_impact"`
	InherentLikelihood            Optional[int]      `json:"inherent_likelihood"`
	InherentImpact                Optional[int]      `json:"inherent_impact"`
	InherentConfidentialityImpact Optional[int]      `json:"inherent_confidentiality_impact"`
	InherentIntegrityImpact       Optional[int]      `json:"inherent_integrity_impact"`
	InherentAvailabilityImpact    Optional[int]      `json:"inherent_availability_impact"`
	TargetLikelihood              Optional[int]      `json:"target_likelihood"`
	TargetImpact                  Optional[int]      `json:"target_impact"`
	Treatment                     *string            `json:"treatment"`
	TreatmentPlan                 *string            `json:"treatment_plan"`
	TreatmentDueDate              Optional[db.Epoch] `json:"treatment_due_date"`
	Owner                         *string            `json:"owner"`
	Status                        *string            `json:"status"`
	LastReview                    Optional[db.Epoch] `json:"last_review"`
	NextReview                    Optional[db.Epoch] `json:"next_review"`
	Notes                         *string            `json:"notes"`
	ExternalID                    *string            `json:"external_id"`
}

// requestedNextReview returns the next_review an update request asks for, or
// nil when the date should be recalculated: the key is absent, null, or names
// the same calendar day as the stored date. The last case matters because a
// client that GETs a record, edits it and PUTs the whole body back echoes the
// stored next_review; treating that echo as a choice would pin a stale date
// and stop it following a change of level or criticality (#202).
func requestedNextReview(sent Optional[db.Epoch], stored *db.Epoch) *db.Epoch {
	if !sent.Set || sent.Value == nil {
		return nil
	}
	if stored != nil && sent.Value.Time.UTC().Format("2006-01-02") == stored.Time.UTC().Format("2006-01-02") {
		return nil
	}
	return sent.Value
}
