package api

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/mail"
)

// Digest is the content of one recipient's weekly email (see mail.Digest).
type Digest = mail.Digest

// Section headings, also the keys the tests look sections up by.
const (
	digestReviews           = "Reviews"
	digestComments          = "Comments awaiting your reply"
	digestTasks             = "Tasks"
	digestChanges           = "Changes"
	digestIncidents         = "Incidents"
	digestCorrectiveActions = "Corrective actions"
	digestSuggestions       = "Suggestions"
)

// digestInboxLimit is the page size the web Inbox asks for (MINE.limit).
const digestInboxLimit = 200

// BuildDigest collects what is waiting on the user (email, with their org
// role) plus a count of what is overdue across the org.
//
// The personal section is the web Inbox's "needs action" set, read through the
// same scoped queries handleInbox uses, so the email and the Inbox cannot
// disagree about what is the user's. It is deliberately not built on
// handleAgentPendingActions, which covers fewer item types.
//
// It is a free function over *db.DB, not a Server method, because its caller
// is the manager cron, which has no Server. Any failing query returns an error
// and no digest: a partial one would claim "nothing pending" about a section
// it never read.
func BuildDigest(ctx context.Context, d *db.DB, orgID int, email, role string) (*Digest, error) {
	canAct := role == "manager" || role == "admin"
	viewer := db.TaskViewer{Email: email, CanSeeAll: canAct}
	var out Digest
	add := func(title string, items []mail.DigestItem) {
		if len(items) > 0 {
			out.Sections = append(out.Sections, mail.DigestSection{Title: title, Items: items})
		}
	}
	fail := func(what string, err error) (*Digest, error) {
		return nil, fmt.Errorf("digest: %s: %w", what, err)
	}

	reviews, _, err := d.PaginatedReviews(ctx, orgID, db.ReviewListParams{Involving: email, Limit: digestInboxLimit})
	if err != nil {
		return fail("reviews", err)
	}
	var items []mail.DigestItem
	for _, r := range reviews {
		if !r.NeedsAction {
			continue
		}
		detail := "Waiting for your review, requested by " + r.RequestedBy
		switch {
		case r.InboxGroup == "sent" && r.Status == "approved":
			detail = "Approved, ready to publish"
		case r.InboxGroup == "sent":
			detail = "Changes requested, ready to resubmit"
		}
		items = append(items, mail.DigestItem{Ref: r.DocumentID, Title: r.Title, Detail: detail, Path: fmt.Sprintf("/reviews/%d", r.ID)})
	}
	add(digestReviews, items)

	comments, err := d.OpenCommentsInvolving(ctx, orgID, email)
	if err != nil {
		return fail("comments", err)
	}
	items = nil
	for _, c := range comments {
		if !c.NeedsAction {
			continue
		}
		path := docLink(c.DocumentID)
		if c.ReviewID != nil {
			path = fmt.Sprintf("/reviews/%d", *c.ReviewID)
		}
		items = append(items, mail.DigestItem{Ref: c.DocumentID, Title: digestSnippet(c.Body), Detail: "From " + c.Author, Path: path})
	}
	add(digestComments, items)

	tasks, _, err := d.PaginatedTasks(ctx, orgID, viewer, db.TaskListParams{Involving: email, Limit: digestInboxLimit})
	if err != nil {
		return fail("tasks", err)
	}
	items = nil
	for _, t := range tasks {
		if !t.NeedsAction {
			continue
		}
		detail := "Priority " + t.Priority
		if t.DueDate != nil {
			detail += ", due " + t.DueDate.Format("2 Jan 2006")
		}
		items = append(items, mail.DigestItem{Ref: t.Identifier, Title: t.Title, Detail: detail, Path: fmt.Sprintf("/tasks/%d", t.ID)})
	}
	add(digestTasks, items)

	changes, _, err := d.PaginatedChangeRequests(ctx, orgID,
		db.ChangeRequestListParams{Involving: email, InvolvingCanApprove: canAct, Limit: digestInboxLimit})
	if err != nil {
		return fail("changes", err)
	}
	items = nil
	for _, c := range changes {
		if !c.NeedsAction {
			continue
		}
		detail := "Status " + strings.ReplaceAll(c.Status, "_", " ")
		if c.Status == "proposed" {
			detail = "Proposed, waiting for your approval"
		}
		items = append(items, mail.DigestItem{Ref: c.Identifier, Title: c.Title, Detail: detail, Path: entityLink("change_request", c.Identifier)})
	}
	add(digestChanges, items)

	// The Inbox lists incidents and corrective actions by assignee, so every
	// open one is the user's; the status filter is the Inbox page's own.
	incidents, err := digestPages(func(page int) ([]db.Incident, int, error) {
		return d.PaginatedIncidents(ctx, orgID, db.IncidentListParams{Assignee: email, Page: page, Limit: digestInboxLimit})
	})
	if err != nil {
		return fail("incidents", err)
	}
	items = nil
	for _, i := range incidents {
		if i.Status == "closed" || i.Status == "resolved" {
			continue
		}
		items = append(items, mail.DigestItem{Ref: i.Identifier, Title: i.Title,
			Detail: fmt.Sprintf("Severity %s, status %s", i.Severity, i.Status), Path: entityLink("incident", i.Identifier)})
	}
	add(digestIncidents, items)

	cas, err := digestPages(func(page int) ([]db.CorrectiveAction, int, error) {
		return d.PaginatedCorrectiveActions(ctx, orgID, db.CorrectiveActionListParams{Assignee: email, Page: page, Limit: digestInboxLimit})
	})
	if err != nil {
		return fail("corrective actions", err)
	}
	items = nil
	for _, c := range cas {
		if c.Status == "resolved" {
			continue
		}
		detail := "Status " + strings.ReplaceAll(c.Status, "_", " ")
		if c.DueDate != nil {
			detail += ", due " + c.DueDate.Format("2 Jan 2006")
		}
		items = append(items, mail.DigestItem{Ref: c.Identifier, Title: c.Title, Detail: detail, Path: entityLink("corrective_action", c.Identifier)})
	}
	add(digestCorrectiveActions, items)

	suggestions, err := d.ListSuggestions(ctx, orgID, db.SuggestionFilters{Involving: email, InvolvingCanReview: canAct, Limit: digestInboxLimit})
	if err != nil {
		return fail("suggestions", err)
	}
	items = nil
	for _, s := range suggestions {
		if !s.NeedsAction {
			continue
		}
		items = append(items, mail.DigestItem{Ref: s.EntityType, Title: s.Title, Detail: "Suggested by " + s.SuggestedBy, Path: "/inbox/suggestions"})
	}
	add(digestSuggestions, items)

	overdue, err := d.GetOverdueSummary(ctx, orgID, viewer)
	if err != nil {
		return fail("overdue summary", err)
	}
	out.Overdue = []mail.DigestCount{
		{Label: "Risks", Count: len(overdue.Risks)},
		{Label: "Suppliers", Count: len(overdue.Suppliers)},
		{Label: "Supplier contracts", Count: len(overdue.SupplierContracts)},
		{Label: "Systems", Count: len(overdue.Systems)},
		{Label: "Legal requirements", Count: len(overdue.Legal)},
		{Label: "Tasks", Count: len(overdue.Tasks)},
	}
	out.OverdueTotal = overdue.TotalCount
	return &out, nil
}

// digestPages reads every page of a paginated list, so an assignee with more
// rows than one page loses none of the open ones behind closed ones.
func digestPages[T any](fetch func(page int) ([]T, int, error)) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		got, total, err := fetch(page)
		if err != nil {
			return nil, err
		}
		all = append(all, got...)
		if len(got) == 0 || len(all) >= total {
			return all, nil
		}
	}
}

// digestSnippet shortens a comment body to one readable line.
func digestSnippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 140 {
		return string(r[:140]) + "..."
	}
	return s
}

// DigestRunResult counts what one SendDigests run did.
type DigestRunResult struct {
	Sent    int
	Empty   int
	Skipped int
	Failed  int
	NotDue  bool
	// Disabled is why nothing was attempted (no mailer, no base URL); empty
	// when the run went ahead.
	Disabled string
}

// digestPeriod returns the ISO-week key ("2026-W44") of now in UTC and whether
// the week's digest is due: from Monday 08:00 UTC of that week on. Both come
// from the same UTC clock, so the due check and the claim key cannot disagree.
func digestPeriod(now time.Time) (period string, due bool) {
	now = now.UTC()
	year, week := now.ISOWeek()
	sinceMonday := (int(now.Weekday()) + 6) % 7
	monday := time.Date(now.Year(), now.Month(), now.Day()-sinceMonday, 8, 0, 0, 0, time.UTC)
	return fmt.Sprintf("%d-W%02d", year, week), !now.Before(monday)
}

// SendDigests sends this ISO week's digest to every eligible member of every
// organization. It is idempotent per (organization, member, week): a member
// is handled once, so an hourly or daily cron, or two replicas, cannot send a
// week twice. Nothing is attempted, and nothing is claimed, without a mailer
// or a base URL (a digest without links is not worth sending); the result says
// why. An error is returned only when the organizations cannot be listed;
// per-member problems are logged and counted in Failed.
func SendDigests(ctx context.Context, d *db.DB, m *mail.Mailer, baseURL string, subdomainRouting bool, now time.Time) (DigestRunResult, error) {
	if reason := digestDisabledReason(m, baseURL); reason != "" {
		return DigestRunResult{Disabled: reason}, nil
	}
	if _, due := digestPeriod(now); !due {
		return DigestRunResult{NotDue: true}, nil
	}
	orgs, err := d.ListOrganizations(ctx)
	if err != nil {
		return DigestRunResult{}, fmt.Errorf("digest: listing organizations: %w", err)
	}
	return runDigests(ctx, d, orgs, m, baseURL, subdomainRouting, now), nil
}

func digestDisabledReason(m *mail.Mailer, baseURL string) string {
	switch {
	case !m.Enabled():
		return "email is not configured (SMTP_HOST / SMTP_FROM)"
	case strings.TrimSpace(baseURL) == "":
		return "ISMS_BASE_URL is not set, so the digest would have no links"
	}
	return ""
}

// runDigests is SendDigests over the given organizations.
//
// Per member: skip agents, inactive users, unverified emails and members who
// turned the digest off; skip those already handled this week; build the
// digest (an error is counted and NOT claimed, so the next run retries); an
// empty digest only records the week as handled, so an item arriving later
// the same week does not trigger a mid-week mail; otherwise claim the week
// and send, releasing the claim if the send fails.
func runDigests(ctx context.Context, d *db.DB, orgs []db.Organization, m *mail.Mailer, baseURL string, subdomainRouting bool, now time.Time) DigestRunResult {
	var res DigestRunResult
	if res.Disabled = digestDisabledReason(m, baseURL); res.Disabled != "" {
		return res
	}
	period, due := digestPeriod(now)
	if !due {
		res.NotDue = true
		return res
	}
	for _, org := range orgs {
		members, err := d.ListOrgUsers(ctx, org.ID)
		if err != nil {
			log.Printf("digest: org %s: listing members: %v", org.Slug, err)
			res.Failed++
			continue
		}
		optOut, err := d.ListDigestOptOuts(ctx, org.ID)
		if err != nil {
			log.Printf("digest: org %s: reading preferences: %v", org.Slug, err)
			res.Failed++
			continue
		}
		claimed, err := d.ListDigestClaims(ctx, org.ID, period)
		if err != nil {
			log.Printf("digest: org %s: reading sent digests: %v", org.Slug, err)
			res.Failed++
			continue
		}
		var om orgMailCtx
		for _, u := range members {
			if u.IsAgent || !u.Active || optOut[u.ID] || claimed[u.ID] {
				res.Skipped++
				continue
			}
			dg, err := BuildDigest(ctx, d, org.ID, u.Email, u.Role)
			if err != nil {
				log.Printf("digest: org %s: %s: %v", org.Slug, u.Email, err)
				res.Failed++
				continue
			}
			if dg.Empty() {
				if won, err := d.ClaimDigest(ctx, org.ID, u.ID, period, "empty"); err != nil {
					log.Printf("digest: org %s: %s: recording empty week: %v", org.Slug, u.Email, err)
					res.Failed++
				} else if won {
					res.Empty++
				} else {
					res.Skipped++
				}
				continue
			}
			won, err := d.ClaimDigest(ctx, org.ID, u.ID, period, "sent")
			if err != nil {
				log.Printf("digest: org %s: %s: claiming: %v", org.Slug, u.Email, err)
				res.Failed++
				continue
			}
			if !won {
				res.Skipped++ // another run or replica holds it
				continue
			}
			if om.AppURL == "" {
				om = orgMailFor(ctx, d, org.ID, baseURL, subdomainRouting)
			}
			name := u.Name
			if name == "" {
				name = u.Email
			}
			if err := m.SendDigestBranded(u.Email, name, om.AppURL, dg, om.Branding); err != nil {
				log.Printf("digest: org %s: %s: sending: %v", org.Slug, u.Email, err)
				if rerr := d.ReleaseDigestClaim(ctx, org.ID, u.ID, period); rerr != nil {
					log.Printf("digest: org %s: %s: releasing claim: %v", org.Slug, u.Email, rerr)
				}
				res.Failed++
				continue
			}
			res.Sent++
		}
	}
	return res
}
