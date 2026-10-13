// Package refs holds the rule for what counts as a valid entity reference: which
// types exist, in which form an id must be written, and whether the record it
// names is there. The API enforces it when a reference is written or listed; the
// CLI applies the same rule to rows already stored, so both must call this one
// implementation.
package refs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// Resolver looks entities up for the reference rule. Store opens the git store
// of an org; it is only called for document references.
type Resolver struct {
	DB    *db.DB
	Store func(ctx context.Context, orgID int) (*store.Store, error)
}

// SequenceIdentifierTypes are the reference types whose display identifier comes
// from a per-org sequence. Its numeric suffix is not the primary key (#201), so a
// bare number is never a valid reference id for them — even where the underlying
// lookup would accept one as a row id.
var SequenceIdentifierTypes = map[string]bool{
	"risk": true, "legal_requirement": true, "asset": true, "supplier": true,
	"system": true, "incident": true, "corrective_action": true,
	"change_request": true, "task": true,
}

// HasIdentifierShape reports whether param could be a display identifier at all:
// a non-empty prefix, a dash, and a non-empty remainder. It is deliberately
// looser than a fixed-prefix check — asset identifiers exist as both ASSET- and
// AST-, and objective display IDs are program-key-scoped, so a hardcoded prefix
// would be wrong. It does not reject a numeric prefix either: a program key is
// only uppercased on create, never checked for digits, so "2026-3" is a
// legitimate objective display ID.
//
// Its only job is to separate "malformed param" from "well-formed identifier
// that does not exist". Deciding whether the identifier belongs to a given
// entity is the lookup's job, not this function's.
func HasIdentifierShape(param string) bool {
	prefix, rest, ok := strings.Cut(param, "-")
	return ok && prefix != "" && rest != ""
}

// Canonical resolves entityID to the form a reference must store it in, and
// reports whether it names a real entity of entityType in the org. For a program
// or objective this is the key / display id, even when entityID was the numeric
// row id (#350); for an audit, audit finding or audit programme it is
// AUDIT-<id> / FIND-<id> / AP-<id> (#366, #394); every other type returns
// entityID unchanged. Writers must store the returned id, never the input
// entityID. A bare number for a sequence type is found=false with no error and
// without touching the DB. err is non-nil only when the lookup itself failed, in
// which case found says nothing about the entity.
func (r *Resolver) Canonical(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) (canonicalID string, found bool, err error) {
	if SequenceIdentifierTypes[entityType] && !HasIdentifierShape(entityID) {
		return entityID, false, nil
	}
	_, canonicalID, found, err = r.Lookup(ctx, orgID, viewer, entityType, entityID)
	return canonicalID, found, err
}

// Lookup looks up an entity by type and ID, and returns its display title, its
// canonical reference ID, and whether it was found. For most types the canonical
// ID is entityID unchanged. Programs and objectives can be addressed by row id
// or by key / display id (#350); both forms resolve, but the canonical ID
// returned is always the key / display id, since that is the form the UI reads
// and writes and the form every reference must be stored in. Audits, audit
// findings and audit programmes likewise resolve by row id or by AUDIT-<id> /
// FIND-<id> / AP-<id>, and canonicalise to the prefixed form (#366, #394).
// Callers that write references (Canonical and its callers) must store the
// returned canonicalID, never the input entityID.
//
// A record that is not there (no row, no such document, an unknown type, an id
// that does not parse) is found=false with a nil error. Any other failure — the
// org's repo will not open, the database errors — is returned as err, so a
// caller that deletes on "not found" never acts on a failed lookup.
func (r *Resolver) Lookup(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) (title, canonicalID string, found bool, err error) {
	// notFound maps a lookup error to the result: pgx.ErrNoRows is a genuine
	// miss, anything else is a failure to look.
	notFound := func(err error) (string, string, bool, error) {
		if errors.Is(err, pgx.ErrNoRows) {
			return entityID, entityID, false, nil
		}
		return entityID, entityID, false, err
	}

	// References store per-org identifiers (e.g. "RISK-12", "INC-3") — both
	// the UI and createReferencesForEntity write that format. Resolve by
	// identifier, never by numeric row id: the numeric part of an identifier
	// and the row id are different sequences and diverge in multi-org DBs.
	switch entityType {
	case "risk":
		x, err := r.DB.GetRiskByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return notFound(err)
		}
		return x.Title, entityID, true, nil

	case "legal_requirement":
		l, err := r.DB.GetLegalRequirementByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return notFound(err)
		}
		return l.Title, entityID, true, nil

	case "asset":
		a, err := r.DB.GetAssetByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return notFound(err)
		}
		return a.Name, entityID, true, nil

	case "supplier":
		sup, err := r.DB.GetSupplierByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return notFound(err)
		}
		return sup.Name, entityID, true, nil

	case "system":
		sys, err := r.DB.GetSystemByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return notFound(err)
		}
		return sys.Name, entityID, true, nil

	case "incident":
		inc, err := r.DB.GetIncidentByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return notFound(err)
		}
		return inc.Title, entityID, true, nil

	case "corrective_action":
		ca, err := r.DB.GetCorrectiveActionByIdentifier(ctx, orgID, entityID)
		if err != nil {
			return notFound(err)
		}
		return ca.Title, entityID, true, nil

	case "objective":
		id, err := strconv.ParseInt(entityID, 10, 64)
		if err != nil {
			// Try by display_id (e.g. "ISMS-1")
			o, err := r.DB.GetObjectiveByDisplayID(ctx, orgID, entityID)
			if err != nil {
				return notFound(err)
			}
			return o.Title, o.DisplayID, true, nil
		}
		o, err := r.DB.GetObjective(ctx, orgID, id)
		if err != nil {
			return notFound(err)
		}
		return o.Title, o.DisplayID, true, nil

	case "program":
		id, err := strconv.ParseInt(entityID, 10, 64)
		if err != nil {
			// Try by key (e.g. "ISMS")
			p, err := r.DB.GetProgramByKey(ctx, orgID, entityID)
			if err != nil {
				return notFound(err)
			}
			return p.Title, p.Key, true, nil
		}
		p, err := r.DB.GetProgram(ctx, orgID, id)
		if err != nil {
			return notFound(err)
		}
		return p.Title, p.Key, true, nil

	case "document":
		st, err := r.Store(ctx, orgID)
		if err != nil {
			return entityID, entityID, false, err
		}
		docPath := st.FindDocumentByID(entityID)
		if docPath == "" {
			return entityID, entityID, false, nil
		}
		doc, err := st.LoadDocument(docPath)
		if err != nil {
			return entityID, entityID, false, err
		}
		if doc.Frontmatter.Title != "" {
			return doc.Frontmatter.Title, entityID, true, nil
		}
		return entityID, entityID, true, nil

	case "audit":
		// AUDIT-/FIND- are built from the row id (audits and audit_findings have
		// no identifier column), so stripping is correct here — see api_audit.go.
		// Both "5" and "AUDIT-5" resolve, but references are stored as AUDIT-<id>
		// only, or the two spellings are two links that never meet (#366).
		id, err := strconv.Atoi(stripPrefix(entityID, "AUDIT-"))
		if err != nil {
			return entityID, entityID, false, nil
		}
		a, err := r.DB.GetAudit(ctx, orgID, id)
		if err != nil {
			return notFound(err)
		}
		return a.Title, fmt.Sprintf("AUDIT-%d", a.ID), true, nil

	case "audit_finding":
		// FIND-<id> is the form db.SoftDeleteAuditFinding's linked-CA guard and
		// the create-from-finding flow use; a bare row id is canonicalised to it.
		id, err := strconv.Atoi(stripPrefix(entityID, "FIND-"))
		if err != nil {
			return entityID, entityID, false, nil
		}
		f, err := r.DB.GetAuditFinding(ctx, orgID, int64(id))
		if err != nil {
			return notFound(err)
		}
		return f.Title, fmt.Sprintf("FIND-%d", f.ID), true, nil

	case "audit_programme":
		// AP-<id> is built from the row id (audit_programmes has no identifier
		// column), so stripping is correct here, and is canonicalised like
		// AUDIT-/FIND- above (#394).
		id, err := strconv.Atoi(stripPrefix(entityID, "AP-"))
		if err != nil {
			return entityID, entityID, false, nil
		}
		p, err := r.DB.GetAuditProgramme(ctx, orgID, id)
		if err != nil {
			return notFound(err)
		}
		return p.Title, fmt.Sprintf("AP-%d", p.ID), true, nil

	case "change_request":
		// CR- identifiers come from the per-org sequence, so the suffix is not
		// the primary key — stripping it named a different change request's
		// title on the reference chip (#201, and the warning in db/changes.go).
		// A bare number is a row id here; Canonical rejects it before it gets
		// this far, but list and search titles still resolve legacy rows by it.
		var cr *db.ChangeRequest
		if n, perr := strconv.ParseInt(entityID, 10, 64); perr == nil {
			var err error
			if cr, err = r.DB.GetChangeRequest(ctx, orgID, int(n)); err != nil {
				return notFound(err)
			}
		} else {
			if !HasIdentifierShape(entityID) {
				return entityID, entityID, false, nil
			}
			var err error
			if cr, err = r.DB.GetChangeRequestByIdentifier(ctx, orgID, entityID); err != nil {
				return notFound(err)
			}
		}
		return cr.Title, entityID, true, nil

	case "task":
		// As with CR- above: TASK- is a per-org sequence, not the primary key.
		var t *db.Task
		if n, perr := strconv.ParseInt(entityID, 10, 64); perr == nil {
			var err error
			if t, err = r.DB.GetTask(ctx, orgID, n); err != nil {
				return notFound(err)
			}
		} else {
			if !strings.HasPrefix(entityID, "TASK-") {
				return entityID, entityID, false, nil
			}
			var err error
			if t, err = r.DB.GetTaskByIdentifier(ctx, orgID, entityID); err != nil {
				return notFound(err)
			}
		}
		// Don't leak a private task's title to someone who may not see it — fall
		// back to the identifier (mirrors db.TaskViewer's rule).
		if t.Private && !viewer.CanSeeAll && t.Assignee != viewer.Email && t.CreatedBy != viewer.Email {
			return entityID, entityID, true, nil
		}
		return t.Title, entityID, true, nil

	default:
		return entityID, entityID, false, nil
	}
}

// stripPrefix removes a prefix if present, otherwise returns the original string.
func stripPrefix(s, prefix string) string {
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
