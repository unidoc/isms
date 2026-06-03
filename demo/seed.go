// Package demo is the SDK for seeding ISMS demonstration organisations.
//
// Run creates the org and applies a Content implementation that adds the
// entities, documents, reviews, and branding the demo should contain.
// Per-customer content lives in separate repositories (e.g. unidoc/isms-demo)
// — this package ships no organisation-specific content.
//
// Content packages talk to the platform through the Seeder methods so
// schema and API changes break them at compile time.
package demo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"isms.sh/internal/isms/blob"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/scaffold"
	"isms.sh/internal/isms/store"
)

// Re-exported types from internal packages. External content modules can't
// import `isms.sh/internal/...`, so the SDK is their single import surface
// for building entity values.
type (
	Risk             = db.Risk
	Supplier         = db.Supplier
	Asset            = db.Asset
	System           = db.System
	Incident         = db.Incident
	Task             = db.Task
	ChangeRequest    = db.ChangeRequest
	CorrectiveAction = db.CorrectiveAction
	Program          = db.Program
	Objective        = db.Objective
	LegalRequirement = db.LegalRequirement
	AuditProgramme   = db.AuditProgramme
	Audit            = db.Audit
	AuditFinding     = db.AuditFinding
	ApprovalPolicy   = db.ApprovalPolicy
	SupplierReview   = db.SupplierReview
	AssetReview      = db.AssetReview
	EntityReading    = db.EntityReading
	Review           = db.Review
	Comment          = db.Comment
	Suggestion       = db.Suggestion
	Epoch            = db.Epoch
	User             = db.User
	DB               = db.DB
)

// DefaultPassword is set on every user created via AddUser. Demo only.
const DefaultPassword = "demo"

func NewEpoch(t time.Time) Epoch { return db.NewEpoch(t) }

func Connect(ctx context.Context, connStr string) (*DB, error) {
	return db.New(ctx, connStr)
}

type Options struct {
	Slug   string
	Name   string
	Domain string
}

// Content is implemented by per-customer content packages. Apply is called
// once with a Seeder that has the organisation already created.
type Content interface {
	Apply(*Seeder) error
}

// Run creates the organisation defined by opts and applies the given
// content. Refuses to overwrite an existing org — callers wanting a fresh
// demo must delete the org first.
func Run(ctx context.Context, database *DB, opts Options, content Content) error {
	if opts.Slug == "" {
		return fmt.Errorf("demo.Options.Slug is required")
	}
	if opts.Name == "" {
		return fmt.Errorf("demo.Options.Name is required")
	}
	if content == nil {
		return fmt.Errorf("demo.Run: content is nil")
	}
	existing, err := database.GetOrganizationBySlug(ctx, opts.Slug)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("checking for existing org: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("organization %q already exists — delete it first to reseed", opts.Slug)
	}

	s := &Seeder{db: database, ctx: ctx, users: map[string]*db.User{}}
	if err := s.CreateOrg(opts.Name, opts.Slug, opts.Domain); err != nil {
		return err
	}
	if err := content.Apply(s); err != nil {
		// Best-effort cleanup so a subsequent reseed isn't blocked by a
		// half-populated org row left over from a failed Apply.
		_ = database.DeleteOrganization(ctx, s.org.ID)
		return fmt.Errorf("seeding content: %w", err)
	}
	return nil
}

// Seeder is the SDK handle content packages use to populate a demo org.
// Instantiated by Run; never directly by content.
type Seeder struct {
	db     *db.DB
	ctx    context.Context
	org    *db.Organization
	users  map[string]*db.User
	admins []*db.User // tracked in addition to users so git authorship can prefer admins
}

func (s *Seeder) Org() *db.Organization    { return s.org }
func (s *Seeder) DB() *db.DB               { return s.db }
func (s *Seeder) Context() context.Context { return s.ctx }
func (s *Seeder) User(tag string) *db.User { return s.users[tag] }

func (s *Seeder) CreateOrg(name, slug, domain string) error {
	dataDir := os.Getenv("ISMS_DATA_DIR")
	if dataDir == "" {
		return fmt.Errorf("ISMS_DATA_DIR is not set")
	}
	repoPath := filepath.Join(dataDir, "repos", slug+".git")
	org := &db.Organization{Name: name, Slug: slug, RepoPath: repoPath}
	if domain != "" {
		org.Domain = &domain
	}
	if err := s.db.CreateOrganization(s.ctx, org); err != nil {
		return fmt.Errorf("creating org: %w", err)
	}
	s.org = org

	if _, err := gogit.PlainOpen(repoPath); err != nil {
		if _, err := gogit.PlainInit(repoPath, true); err != nil {
			return fmt.Errorf("initializing repo at %s: %w", repoPath, err)
		}
	}
	return nil
}

// ScaffoldTemplate scaffolds a template from ISMS_TEMPLATE_PATH into the
// org's repo. Optional — content can omit this for an empty repo.
func (s *Seeder) ScaffoldTemplate(template string) error {
	if !scaffold.IsValidTemplate(template) {
		return fmt.Errorf("unknown template %q", template)
	}
	st, err := store.NewBare(s.org.RepoPath)
	if err != nil {
		return fmt.Errorf("opening bare repo: %w", err)
	}
	owner := s.firstAdminOrFallback()
	marker := fmt.Sprintf("# %s\n\nDemo ISMS repository (auto-seeded).\n", s.org.Name)
	if _, err := st.CommitFile("README.md", []byte(marker), owner.name, owner.email, "chore: initialize demo repository"); err != nil {
		return fmt.Errorf("initial commit: %w", err)
	}
	if err := scaffold.ScaffoldToRepo(st, template, owner.name, owner.email); err != nil {
		return fmt.Errorf("scaffolding template: %w", err)
	}
	return nil
}

// AddUser creates a verified user with DefaultPassword and adds them to the
// org with the given role (admin / manager / contributor / reader). A
// non-empty tag is recorded for User(tag) lookup from later steps.
func (s *Seeder) AddUser(email, name, role, tag string, isAgent bool) (*db.User, error) {
	usr := &db.User{Email: email, Name: name, IsAgent: isAgent, Active: true}
	if err := s.db.UpsertUser(s.ctx, usr); err != nil {
		return nil, fmt.Errorf("creating user %s: %w", email, err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(DefaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	if err := s.db.SetPassword(s.ctx, usr.ID, string(hash)); err != nil {
		return nil, fmt.Errorf("set password for %s: %w", email, err)
	}
	if err := s.db.SetEmailVerified(s.ctx, usr.ID); err != nil {
		return nil, fmt.Errorf("verifying %s: %w", email, err)
	}
	if err := s.db.AddOrgMember(s.ctx, s.org.ID, usr.ID, role); err != nil {
		return nil, fmt.Errorf("adding member %s: %w", email, err)
	}
	if tag != "" {
		s.users[tag] = usr
	}
	if role == "admin" {
		s.admins = append(s.admins, usr)
	}
	return usr, nil
}

func (s *Seeder) AddRisk(r *db.Risk) error          { return s.db.CreateRisk(s.ctx, s.org.ID, r) }
func (s *Seeder) AddSupplier(sp *db.Supplier) error { return s.db.CreateSupplier(s.ctx, s.org.ID, sp) }
func (s *Seeder) AddAsset(a *db.Asset) error        { return s.db.CreateAsset(s.ctx, s.org.ID, a) }
func (s *Seeder) AddSystem(sys *db.System) error    { return s.db.CreateSystem(s.ctx, s.org.ID, sys) }
func (s *Seeder) AddIncident(i *db.Incident) error  { return s.db.CreateIncident(s.ctx, s.org.ID, i) }
func (s *Seeder) AddTask(t *db.Task) error          { return s.db.CreateTask(s.ctx, s.org.ID, t) }
func (s *Seeder) AddChangeRequest(c *db.ChangeRequest) error {
	return s.db.CreateChangeRequest(s.ctx, s.org.ID, c)
}
func (s *Seeder) AddCorrectiveAction(ca *db.CorrectiveAction) error {
	return s.db.CreateCorrectiveAction(s.ctx, s.org.ID, ca)
}
func (s *Seeder) AddProgram(p *db.Program) error     { return s.db.CreateProgram(s.ctx, s.org.ID, p) }
func (s *Seeder) AddObjective(o *db.Objective) error { return s.db.CreateObjective(s.ctx, s.org.ID, o) }
func (s *Seeder) AddLegalRequirement(lr *db.LegalRequirement) error {
	return s.db.CreateLegalRequirement(s.ctx, s.org.ID, lr)
}
func (s *Seeder) AddAuditProgramme(p *db.AuditProgramme) error {
	return s.db.CreateAuditProgramme(s.ctx, s.org.ID, p)
}
func (s *Seeder) AddAudit(a *db.Audit) error { return s.db.CreateAudit(s.ctx, s.org.ID, a) }
func (s *Seeder) AddAuditFinding(f *db.AuditFinding) error {
	return s.db.AddAuditFinding(s.ctx, s.org.ID, f)
}
func (s *Seeder) AddApprovalPolicy(p *db.ApprovalPolicy) error {
	return s.db.CreateApprovalPolicy(s.ctx, s.org.ID, p)
}
func (s *Seeder) AddSupplierReview(sr *db.SupplierReview) error {
	return s.db.CreateSupplierReview(s.ctx, s.org.ID, sr)
}
func (s *Seeder) AddAssetReview(ar *db.AssetReview) error {
	return s.db.CreateAssetReview(s.ctx, s.org.ID, ar)
}
func (s *Seeder) AddEntityReading(r *db.EntityReading) error {
	return s.db.CreateEntityReading(s.ctx, s.org.ID, r)
}
func (s *Seeder) AddReference(sourceType, sourceID, targetType, targetID, createdBy string) error {
	return s.db.CreateReference(s.ctx, s.org.ID, &db.EntityReference{
		SourceType: sourceType,
		SourceID:   sourceID,
		TargetType: targetType,
		TargetID:   targetID,
		CreatedBy:  createdBy,
	})
}
func (s *Seeder) AddReview(r *db.Review) error { return s.db.CreateReview(s.ctx, s.org.ID, r) }
func (s *Seeder) AddReviewAssignment(reviewID int, reviewer, status string) error {
	return s.db.AddReviewAssignment(s.ctx, s.org.ID, &db.ReviewAssignment{
		ReviewID: reviewID,
		Reviewer: reviewer,
		Status:   status,
	})
}
func (s *Seeder) AddComment(c *db.Comment) error { return s.db.AddComment(s.ctx, s.org.ID, c) }
func (s *Seeder) AddSuggestion(sg *db.Suggestion) error {
	return s.db.CreateSuggestion(s.ctx, s.org.ID, sg)
}

// AddActivity inserts an activity-feed entry at an explicit timestamp —
// used to backdate historical events when seeding a believable timeline.
func (s *Seeder) AddActivity(actor, action, detail, documentID string, at time.Time) error {
	return s.db.LogActivityAt(s.ctx, s.org.ID, &db.Activity{
		Actor:      actor,
		Action:     action,
		Detail:     detail,
		DocumentID: documentID,
	}, at)
}

type DocFile struct {
	Path    string
	Content []byte
}

// CommitDocuments writes files to the bare repo as a single commit authored
// by the first added admin (or a synthetic identity if none yet).
func (s *Seeder) CommitDocuments(files []DocFile, message string) error {
	st, err := store.NewBare(s.org.RepoPath)
	if err != nil {
		return fmt.Errorf("opening bare repo: %w", err)
	}
	owner := s.firstAdminOrFallback()
	entries := make([]store.FileEntry, len(files))
	for i, f := range files {
		entries[i] = store.FileEntry{Path: f.Path, Content: f.Content}
	}
	_, err = st.CommitFiles(entries, owner.name, owner.email, message)
	return err
}

func (s *Seeder) SetOrgSetting(key, value string) error {
	return s.db.SetOrgSetting(s.ctx, s.org.ID, key, value)
}

// Branding configures the org's visual identity. Logo and favicon SVGs are
// uploaded to the blob backend the API server uses (ISMS_STORAGE_BACKEND).
// Nil bytes skip an asset.
type Branding struct {
	Name    string
	Color   string
	Footer  string
	LogoSVG []byte
	FavSVG  []byte
}

func (s *Seeder) SetBranding(b Branding) error {
	if len(b.LogoSVG) > 0 || len(b.FavSVG) > 0 {
		store, err := blob.NewFromEnv()
		if err != nil {
			return fmt.Errorf("blob store: %w", err)
		}
		if len(b.LogoSVG) > 0 {
			if err := store.Put(s.ctx, s.org.UUID, "branding/logo.svg", b.LogoSVG); err != nil {
				return fmt.Errorf("uploading logo: %w", err)
			}
		}
		if len(b.FavSVG) > 0 {
			if err := store.Put(s.ctx, s.org.UUID, "branding/favicon.svg", b.FavSVG); err != nil {
				return fmt.Errorf("uploading favicon: %w", err)
			}
		}
	}
	for k, v := range map[string]string{
		"branding_name":   b.Name,
		"branding_color":  b.Color,
		"branding_footer": b.Footer,
	} {
		if v == "" {
			continue
		}
		if err := s.SetOrgSetting(k, v); err != nil {
			return fmt.Errorf("setting %s: %w", k, err)
		}
	}
	return nil
}

type identity struct{ name, email string }

func (s *Seeder) firstAdminOrFallback() identity {
	if len(s.admins) > 0 {
		u := s.admins[0]
		return identity{name: u.Name, email: u.Email}
	}
	for _, u := range s.users {
		if u != nil {
			return identity{name: u.Name, email: u.Email}
		}
	}
	return identity{name: "isms-demo", email: "demo@" + s.org.Slug + ".invalid"}
}
