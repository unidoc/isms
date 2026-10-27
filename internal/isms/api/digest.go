package api

import (
	"context"
	"fmt"
	"strings"

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
