package model

// DocumentFrontmatter is the YAML frontmatter for all ISMS documents.
// Not all fields apply to all document types — which fields are used
// depends on the template and document folder, not on hardcoded types.
type DocumentFrontmatter struct {
	DocumentID     string         `yaml:"document_id" json:"document_id"`
	Title          string         `yaml:"title" json:"title"`
	Type           string         `yaml:"type,omitempty" json:"type,omitempty"`                 // control, policy, procedure, clause, record, guideline — empty = document
	Version        string         `yaml:"version,omitempty" json:"version,omitempty"`
	Status         string         `yaml:"status" json:"status"`                                 // draft, in_review, approved, retired
	Author         string         `yaml:"author,omitempty" json:"author,omitempty"`
	Owner          string         `yaml:"owner,omitempty" json:"owner,omitempty"` // responsible for periodic review
	Reviewer       string         `yaml:"reviewer,omitempty" json:"reviewer,omitempty"`
	ApprovedBy     string         `yaml:"approved_by,omitempty" json:"approved_by,omitempty"`
	ApprovedDate   string         `yaml:"approved_date,omitempty" json:"approved_date,omitempty"`
	EffectiveDate  string         `yaml:"effective_date,omitempty" json:"effective_date,omitempty"`
	NextReview     string         `yaml:"next_review,omitempty" json:"next_review,omitempty"`
	ReviewCycle    int            `yaml:"review_cycle,omitempty" json:"review_cycle,omitempty"` // months
	Classification string         `yaml:"classification,omitempty" json:"classification,omitempty"`
	Changelog      []DocumentChange `yaml:"changelog,omitempty" json:"changelog,omitempty"`
}


// DocumentChange is a version history entry in the document changelog.
type DocumentChange struct {
	Version     string `yaml:"version" json:"version"`
	Date        string `yaml:"date" json:"date"`
	Author      string `yaml:"author" json:"author"`
	ApprovedBy  string `yaml:"approved_by,omitempty" json:"approved_by,omitempty"`
	Description string `yaml:"description" json:"description"`
}

// DocumentStatuses lists the valid document statuses.
var DocumentStatuses = []string{"draft", "in_review", "approved", "retired"}

