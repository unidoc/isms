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
	// Fix deletes every broken pair after reporting, and rewrites every
	// non-canonical one to its canonical id (merging it into the canonical row
	// when that exists). Unchecked pairs are never touched.
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

	// NonCanonical counts pairs whose two ends resolve but are stored under a
	// non-canonical id (a program by row id, a bare audit number, ...).
	// NonCanonicalRows are the rows behind them and Duplicates the pairs whose
	// canonical twin already exists (or is made by an earlier pair), so Fix merges
	// them instead of rewriting.
	NonCanonical     int
	NonCanonicalRows int
	Duplicates       int
	// NonCanonicalComments counts comment_references rows stored under a
	// non-canonical id.
	NonCanonicalComments int
	// Fixed pairs, FixedRows (rows updated or deleted) and FixedComments are
	// what Fix rewrote or merged.
	Fixed         int
	FixedRows     int
	FixedComments int
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
	ok        bool
	canonical string // set when ok: the id every writer stores for this record
	reason    string // set when broken
	err       error  // set when the lookup failed
}

// canonRow is one entity_references row that resolves but is stored under a
// non-canonical id: its stored ends and the ends it should have.
type canonRow struct {
	id                       int64
	source, target           pairEnd
	canonSource, canonTarget pairEnd
}

// canonFinding is a stored pair (both directions) with non-canonical ids,
// shown in the orientation of its oldest row.
type canonFinding struct {
	source, target           pairEnd
	canonSource, canonTarget pairEnd
	rows                     []canonRow
	duplicate                bool // the canonical pair already exists, or an earlier finding makes it
}

func (f canonFinding) stored() string { return f.source.String() + " -> " + f.target.String() }
func (f canonFinding) canonical() string {
	return f.canonSource.String() + " -> " + f.canonTarget.String()
}

// commentFinding is a comment_references row stored under a non-canonical id.
type commentFinding struct {
	id                       int64
	canonSource, canonTarget string
}

// orgScan is everything Check learned about one org.
type orgScan struct {
	broken   []finding
	canon    []canonFinding
	comments []commentFinding
}

// Check scans the stored references of one or all organizations with the same
// rule the API enforces (Resolver.Canonical) and writes a report to w. A pair is
// broken when either end fails to resolve. A pair whose lookup failed (repo not
// openable, database error) is unchecked, never broken, and is never deleted.
// A pair whose ends resolve but are stored under the other spelling of the same
// record (a program by row id, a bare audit number) is non-canonical; so is a
// comment_references row that does. With opts.Fix, each org's broken pairs are
// removed in one transaction, then its non-canonical rows are rewritten to the
// canonical id or, when that row exists, merged into it, in a second one.
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

		canon       []canonFinding
		canonRows   int
		duplicates  int
		commentRows []commentFinding
	}
	var all []orgFindings
	for _, org := range orgs {
		sc, err := scanOrg(ctx, d, org, opts)
		if err != nil {
			return res, fmt.Errorf("checking %s: %w", org.Slug, err)
		}
		of := orgFindings{org: org, findings: sc.broken, canon: sc.canon, commentRows: sc.comments}
		for _, f := range sc.broken {
			if f.broken {
				of.broken++
				of.rows += len(f.rowIDs)
			} else {
				of.unchk++
			}
		}
		for _, f := range sc.canon {
			of.canonRows += len(f.rows)
			if f.duplicate {
				of.duplicates++
			}
		}
		res.Broken += of.broken
		res.BrokenRows += of.rows
		res.Unchecked += of.unchk
		res.NonCanonical += len(of.canon)
		res.NonCanonicalRows += of.canonRows
		res.Duplicates += of.duplicates
		res.NonCanonicalComments += len(of.commentRows)
		all = append(all, of)
	}

	brokenClean := res.Broken == 0 && res.Unchecked == 0
	if brokenClean {
		fmt.Fprintln(w, "0 broken pairs (0 rows): every reference resolves")
	} else {
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
	}

	canonClean := res.NonCanonical == 0 && res.NonCanonicalComments == 0
	if canonClean {
		fmt.Fprintln(w, "0 non-canonical pairs (0 rows): every reference uses its canonical id")
	} else {
		if res.NonCanonical > 0 {
			tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ORG\tSTORED\tCANONICAL\tACTION")
			for _, of := range all {
				for _, f := range of.canon {
					action := "rewrite"
					if f.duplicate {
						action = "merge (duplicate)"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", of.org.Slug, f.stored(), f.canonical(), action)
				}
			}
			if err := tw.Flush(); err != nil {
				return res, err
			}
		}
		summary := func(label string, pairs, rows, dups int) {
			line := fmt.Sprintf("%s: %s (%s)", label, plural(pairs, "non-canonical pair"), plural(rows, "row"))
			if dups > 0 {
				line += ", " + plural(dups, "duplicate")
			}
			fmt.Fprintln(w, line)
		}
		for _, of := range all {
			if len(of.canon) > 0 {
				summary(of.org.Slug, len(of.canon), of.canonRows, of.duplicates)
			}
			if len(of.commentRows) > 0 {
				fmt.Fprintf(w, "%s: %s a non-canonical id\n", of.org.Slug, useVerb(len(of.commentRows), "comment mention row"))
			}
		}
		summary("total", res.NonCanonical, res.NonCanonicalRows, res.Duplicates)
		if res.NonCanonicalComments > 0 {
			fmt.Fprintf(w, "total: %s a non-canonical id\n", useVerb(res.NonCanonicalComments, "comment mention row"))
		}
	}

	if !opts.Fix {
		return res, nil
	}
	if !brokenClean {
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
	}
	if canonClean {
		return res, nil
	}

	for _, of := range all {
		if len(of.canon) == 0 && len(of.commentRows) == 0 {
			continue
		}
		var pairs, rows, comments int
		var lines []string
		err := d.WithOrgTx(ctx, of.org.ID, func(ctx context.Context, tx pgx.Tx) error {
			pairs, rows, comments, lines = 0, 0, 0, nil
			for _, f := range of.canon {
				var touched int
				merged := false
				for _, r := range f.rows {
					fix, err := db.CanonicalizeReferenceTx(ctx, tx, of.org.ID, r.id, r.canonSource.Type, r.canonSource.ID, r.canonTarget.Type, r.canonTarget.ID)
					if err != nil {
						return err
					}
					if fix != db.ReferenceGone {
						touched++
					}
					merged = merged || fix == db.ReferenceMerged
				}
				if touched == 0 {
					continue
				}
				pairs++
				rows += touched
				if merged {
					lines = append(lines, fmt.Sprintf("merged %s %s into %s (%s)", of.org.Slug, f.stored(), f.canonical(), plural(touched, "row")))
				} else {
					lines = append(lines, fmt.Sprintf("rewrote %s %s as %s (%s)", of.org.Slug, f.stored(), f.canonical(), plural(touched, "row")))
				}
			}
			for _, c := range of.commentRows {
				fix, err := db.CanonicalizeCommentReferenceTx(ctx, tx, of.org.ID, c.id, c.canonSource, c.canonTarget)
				if err != nil {
					return err
				}
				if fix != db.ReferenceGone {
					comments++
				}
			}
			return nil
		})
		if err != nil {
			return res, fmt.Errorf("canonicalising references of %s: %w", of.org.Slug, err)
		}
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
		if len(of.canon) > 0 {
			fmt.Fprintf(w, "%s: fixed %s (%s)\n", of.org.Slug, plural(pairs, "non-canonical pair"), plural(rows, "row"))
		}
		if comments > 0 {
			fmt.Fprintf(w, "%s: fixed %s\n", of.org.Slug, plural(comments, "comment mention row"))
		}
		res.Fixed += pairs
		res.FixedRows += rows
		res.FixedComments += comments
	}
	fmt.Fprintf(w, "total: fixed %s (%s)\n", plural(res.Fixed, "non-canonical pair"), plural(res.FixedRows, "row"))
	if res.FixedComments > 0 {
		fmt.Fprintf(w, "total: fixed %s\n", plural(res.FixedComments, "comment mention row"))
	}
	return res, nil
}

// scanOrg classifies every reference pair of one org: the broken and unchecked
// ones, and, among the pairs that resolve, the ones stored under a non-canonical
// id. Both come from one Canonical call per end.
func scanOrg(ctx context.Context, d *db.DB, org db.Organization, opts CheckOptions) (orgScan, error) {
	var sc orgScan
	var rows []db.EntityReference
	var commentRows []db.CommentReferenceRow
	err := d.WithOrgTx(ctx, org.ID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if rows, err = db.ListOrgReferencesTx(ctx, tx, org.ID); err != nil {
			return err
		}
		commentRows, err = db.ListOrgCommentReferencesTx(ctx, tx, org.ID)
		return err
	})
	if err != nil {
		return sc, err
	}

	// Group the rows into unordered pairs. Rows come ordered by id, so the first
	// row of a pair is its oldest and gives the orientation to print.
	type pair struct {
		rowIDs    []int64
		rows      []db.EntityReference
		sourceEnd pairEnd
		targetEnd pairEnd
	}
	unordered := func(a, b pairEnd) [2]pairEnd {
		if a.String() > b.String() {
			return [2]pairEnd{b, a}
		}
		return [2]pairEnd{a, b}
	}
	pairs := map[[2]pairEnd]*pair{}
	var order []*pair
	for _, r := range rows {
		a, b := pairEnd{r.SourceType, r.SourceID}, pairEnd{r.TargetType, r.TargetID}
		key := unordered(a, b)
		p := pairs[key]
		if p == nil {
			p = &pair{sourceEnd: a, targetEnd: b}
			pairs[key] = p
			order = append(order, p)
		}
		p.rowIDs = append(p.rowIDs, r.ID)
		p.rows = append(p.rows, r)
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
	var canon []canonFinding
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
			// Both ends resolve. The pair is fine unless an end is stored under
			// another spelling than the one every writer uses.
			cs, ct := pairEnd{p.sourceEnd.Type, src.canonical}, pairEnd{p.targetEnd.Type, tgt.canonical}
			if cs != p.sourceEnd || ct != p.targetEnd {
				cf := canonFinding{source: p.sourceEnd, target: p.targetEnd, canonSource: cs, canonTarget: ct}
				for _, r := range p.rows {
					a, b := pairEnd{r.SourceType, r.SourceID}, pairEnd{r.TargetType, r.TargetID}
					cf.rows = append(cf.rows, canonRow{id: r.ID, source: a, target: b,
						canonSource: pairEnd{a.Type, classify(a).canonical}, canonTarget: pairEnd{b.Type, classify(b).canonical}})
				}
				canon = append(canon, cf)
			}
			continue
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].rowIDs[0] < out[j].rowIDs[0] })

	// A finding is a duplicate when its canonical pair is already stored, or an
	// earlier finding of the report maps to it (that one gets the rewrite).
	sort.SliceStable(canon, func(i, j int) bool { return canon[i].rows[0].id < canon[j].rows[0].id })
	made := map[[2]pairEnd]bool{}
	for i := range canon {
		key := unordered(canon[i].canonSource, canon[i].canonTarget)
		canon[i].duplicate = pairs[key] != nil || made[key]
		made[key] = true
	}

	// comment_references rows: a row whose ends resolve and differ is
	// non-canonical. Broken or unchecked ends are the broken check's business.
	var comments []commentFinding
	for _, c := range commentRows {
		src, tgt := classify(pairEnd{c.SourceType, c.SourceID}), classify(pairEnd{c.TargetType, c.TargetID})
		if !src.ok || !tgt.ok {
			continue
		}
		if src.canonical != c.SourceID || tgt.canonical != c.TargetID {
			comments = append(comments, commentFinding{id: c.ID, canonSource: src.canonical, canonTarget: tgt.canonical})
		}
	}
	return orgScan{broken: out, canon: canon, comments: comments}, nil
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
	canonical, found, err := r.Canonical(ctx, orgID, viewer, e.Type, e.ID)
	if err != nil {
		return endState{err: err}
	}
	if found {
		return endState{ok: true, canonical: canonical}
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

// useVerb renders a count with its noun and a verb that agrees with it.
func useVerb(n int, word string) string {
	if n == 1 {
		return plural(n, word) + " uses"
	}
	return plural(n, word) + " use"
}

func allDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}
