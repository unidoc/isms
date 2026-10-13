package refs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/jackc/pgx/v5"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// CheckOptions controls Check.
type CheckOptions struct {
	// OrgSlug limits the check to one organization; empty checks them all.
	OrgSlug string
	// Fix deletes every broken pair after reporting. Unchecked pairs are never
	// deleted.
	Fix bool
	// OpenStore opens the org's git repo. It is called at most once per org, and
	// only when the org has a document reference.
	OpenStore func(org db.Organization) (*store.Store, error)
}

// CheckResult counts what Check found and did. A pair is the two directions of
// one link; rows are the entity_references rows behind it (legacy data may hold
// one direction only).
type CheckResult struct {
	Broken      int // pairs with an end that does not resolve
	BrokenRows  int
	Unchecked   int // pairs whose lookup failed, so nothing is known
	Deleted     int // broken pairs removed by Fix
	DeletedRows int
}

type pairEnd struct{ Type, ID string }

func (e pairEnd) String() string { return e.Type + " " + e.ID }

type finding struct {
	source, target pairEnd
	reason         string
	rowIDs         []int64
	broken         bool // false: unchecked
}

// endState is the verdict on one end of a pair.
type endState struct {
	ok     bool
	reason string // set when broken
	err    error  // set when the lookup failed
}

// Check scans the stored references of one or all organizations with the same
// rule the API enforces (Resolver.Canonical) and writes a report to w. A pair is
// broken when either end fails to resolve. A pair whose lookup failed (repo not
// openable, database error) is unchecked, never broken, and is never deleted.
// With opts.Fix, each org's broken pairs are removed in one transaction.
func Check(ctx context.Context, d *db.DB, w io.Writer, opts CheckOptions) (CheckResult, error) {
	var res CheckResult
	var orgs []db.Organization
	if opts.OrgSlug != "" {
		o, err := d.GetOrganizationBySlug(ctx, opts.OrgSlug)
		if errors.Is(err, pgx.ErrNoRows) {
			return res, fmt.Errorf("organization %q not found", opts.OrgSlug)
		}
		if err != nil {
			return res, err
		}
		orgs = []db.Organization{*o}
	} else {
		var err error
		if orgs, err = d.ListOrganizations(ctx); err != nil {
			return res, err
		}
	}

	type orgFindings struct {
		org      db.Organization
		findings []finding
		broken   int
		rows     int
		unchk    int
	}
	var all []orgFindings
	for _, org := range orgs {
		fs, err := scanOrg(ctx, d, org, opts)
		if err != nil {
			return res, fmt.Errorf("checking %s: %w", org.Slug, err)
		}
		of := orgFindings{org: org, findings: fs}
		for _, f := range fs {
			if f.broken {
				of.broken++
				of.rows += len(f.rowIDs)
			} else {
				of.unchk++
			}
		}
		res.Broken += of.broken
		res.BrokenRows += of.rows
		res.Unchecked += of.unchk
		all = append(all, of)
	}

	if res.Broken == 0 && res.Unchecked == 0 {
		fmt.Fprintln(w, "0 broken pairs (0 rows): every reference resolves")
		return res, nil
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ORG\tSOURCE\tTARGET\tREASON")
	for _, of := range all {
		for _, f := range of.findings {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", of.org.Slug, f.source, f.target, f.reason)
		}
	}
	if err := tw.Flush(); err != nil {
		return res, err
	}
	summary := func(label string, broken, rows, unchecked int) {
		line := fmt.Sprintf("%s: %s (%s)", label, plural(broken, "broken pair"), plural(rows, "row"))
		if unchecked > 0 {
			line += fmt.Sprintf(", %d unchecked", unchecked)
		}
		fmt.Fprintln(w, line)
	}
	for _, of := range all {
		if of.broken > 0 || of.unchk > 0 {
			summary(of.org.Slug, of.broken, of.rows, of.unchk)
		}
	}
	summary("total", res.Broken, res.BrokenRows, res.Unchecked)

	if !opts.Fix {
		return res, nil
	}
	for _, of := range all {
		if of.broken == 0 {
			continue
		}
		var pairs, rows int
		var lines []string
		err := d.WithOrgTx(ctx, of.org.ID, func(ctx context.Context, tx pgx.Tx) error {
			pairs, rows, lines = 0, 0, nil
			for _, f := range of.findings {
				if !f.broken {
					continue
				}
				n, err := db.DeleteReferencesTx(ctx, tx, of.org.ID, f.rowIDs)
				if err != nil {
					return err
				}
				pairs++
				rows += int(n)
				lines = append(lines, fmt.Sprintf("deleted %s %s -> %s (%s)", of.org.Slug, f.source, f.target, plural(int(n), "row")))
			}
			return nil
		})
		if err != nil {
			return res, fmt.Errorf("deleting broken references of %s: %w", of.org.Slug, err)
		}
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
		fmt.Fprintf(w, "%s: deleted %s (%s)\n", of.org.Slug, plural(pairs, "broken pair"), plural(rows, "row"))
		res.Deleted += pairs
		res.DeletedRows += rows
	}
	fmt.Fprintf(w, "total: deleted %s (%s)\n", plural(res.Deleted, "broken pair"), plural(res.DeletedRows, "row"))
	return res, nil
}

// scanOrg classifies every reference pair of one org and returns the broken
// and unchecked ones.
func scanOrg(ctx context.Context, d *db.DB, org db.Organization, opts CheckOptions) ([]finding, error) {
	var rows []db.EntityReference
	err := d.WithOrgTx(ctx, org.ID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = db.ListOrgReferencesTx(ctx, tx, org.ID)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Group the rows into unordered pairs. Rows come ordered by id, so the first
	// row of a pair is its oldest and gives the orientation to print.
	type pair struct {
		rowIDs    []int64
		sourceEnd pairEnd
		targetEnd pairEnd
	}
	pairs := map[[2]pairEnd]*pair{}
	var order []*pair
	for _, r := range rows {
		a, b := pairEnd{r.SourceType, r.SourceID}, pairEnd{r.TargetType, r.TargetID}
		key := [2]pairEnd{a, b}
		if a.String() > b.String() {
			key = [2]pairEnd{b, a}
		}
		p := pairs[key]
		if p == nil {
			p = &pair{sourceEnd: a, targetEnd: b}
			pairs[key] = p
			order = append(order, p)
		}
		p.rowIDs = append(p.rowIDs, r.ID)
	}

	// One store per org, opened lazily and once; a failure is remembered so every
	// document end reports the same error without reopening.
	var st *store.Store
	var stErr error
	opened := false
	resolver := &Resolver{DB: d, Store: func(context.Context, int) (*store.Store, error) {
		if !opened {
			opened = true
			st, stErr = openOrgStore(org, opts.OpenStore)
		}
		return st, stErr
	}}
	viewer := db.TaskViewer{CanSeeAll: true}
	cache := map[pairEnd]endState{}
	classify := func(e pairEnd) endState {
		if s, ok := cache[e]; ok {
			return s
		}
		s := classifyEnd(ctx, d, resolver, org.ID, viewer, e)
		cache[e] = s
		return s
	}

	var out []finding
	for _, p := range order {
		src, tgt := classify(p.sourceEnd), classify(p.targetEnd)
		f := finding{source: p.sourceEnd, target: p.targetEnd, rowIDs: p.rowIDs}
		switch {
		case !src.ok && src.err == nil && !tgt.ok && tgt.err == nil:
			// Both ends broken: keep the stored orientation of the oldest row.
			f.broken, f.reason = true, src.reason+"; "+tgt.reason
		case !src.ok && src.err == nil:
			// Healthy (or unknown) end first, the broken one second.
			f.source, f.target = p.targetEnd, p.sourceEnd
			f.broken, f.reason = true, src.reason
		case !tgt.ok && tgt.err == nil:
			f.broken, f.reason = true, tgt.reason
		case src.err != nil || tgt.err != nil:
			err := src.err
			if err == nil {
				err = tgt.err
			}
			f.reason = "unchecked: " + err.Error()
			// Show the end that could be checked first.
			if src.err != nil && tgt.err == nil {
				f.source, f.target = p.targetEnd, p.sourceEnd
			}
		default:
			continue
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].rowIDs[0] < out[j].rowIDs[0] })
	return out, nil
}

// openOrgStore opens the org's repo and makes sure it is readable. A repo with
// no commits yet is fine (it has no documents); any other trouble with HEAD is
// an error, so a damaged repo is reported as unchecked, never as missing
// documents.
func openOrgStore(org db.Organization, open func(db.Organization) (*store.Store, error)) (*store.Store, error) {
	st, err := open(org)
	if err != nil {
		return nil, err
	}
	if _, err := st.HeadHash(); err != nil && !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, err
	}
	return st, nil
}

func classifyEnd(ctx context.Context, d *db.DB, r *Resolver, orgID int, viewer db.TaskViewer, e pairEnd) endState {
	_, found, err := r.Canonical(ctx, orgID, viewer, e.Type, e.ID)
	if err != nil {
		return endState{err: err}
	}
	if found {
		return endState{ok: true}
	}
	if SequenceIdentifierTypes[e.Type] {
		if allDigits(e.ID) {
			return endState{reason: "bare number, not an identifier"}
		}
		if !HasIdentifierShape(e.ID) {
			return endState{reason: "not an identifier"}
		}
	}
	deleted, err := d.SoftDeletedEntityExists(ctx, orgID, e.Type, e.ID)
	if err != nil {
		return endState{err: err}
	}
	if deleted {
		return endState{reason: "record deleted"}
	}
	return endState{reason: "not found"}
}

// plural renders a count with its noun, singular for exactly one.
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func allDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}
