package db

import (
	"context"
	"time"
)

// OverdueItem represents something that needs review attention.
type OverdueItem struct {
	EntityType  string `json:"entity_type"` // risk, supplier, system, legal, policy
	EntityID    string `json:"entity_id"`   // identifier (RISK-1, SUPPLIER-1, etc.)
	Title       string `json:"title"`
	Owner       string `json:"owner,omitempty"`
	NextReview  *Epoch `json:"next_review"`
	DaysLate    int    `json:"days_late"`
	Criticality string `json:"criticality,omitempty"` // risk level or criticality
}

// OverdueSummary is the aggregate overdue status across all entity types.
type OverdueSummary struct {
	Risks      []OverdueItem `json:"risks"`
	Suppliers  []OverdueItem `json:"suppliers"`
	Systems    []OverdueItem `json:"systems"`
	Legal      []OverdueItem `json:"legal"`
	Tasks      []OverdueItem `json:"tasks"`
	TotalCount int           `json:"total_count"`
}

// GetOverdueSummary returns all overdue review items across entity types.
func (d *DB) GetOverdueSummary(ctx context.Context, orgID int) (*OverdueSummary, error) {
	now := time.Now()
	summary := &OverdueSummary{}

	// Overdue risks
	rows, err := d.pool.Query(ctx, `
		SELECT identifier, title,
			COALESCE((SELECT email FROM users WHERE id = risks.owner_id), ''),
			next_review, COALESCE(current_level, '')
		FROM risks
		WHERE organization_id = $1
			AND status NOT IN ('closed')
			AND next_review IS NOT NULL
			AND next_review < $2
			AND deleted_at IS NULL
		ORDER BY next_review ASC
	`, orgID, now)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item OverdueItem
			var reviewDate Epoch
			if err := rows.Scan(&item.EntityID, &item.Title, &item.Owner, &reviewDate, &item.Criticality); err != nil {
				break
			}
			item.EntityType = "risk"
			item.NextReview = &reviewDate
			item.DaysLate = int(now.Sub(reviewDate.Time).Hours() / 24)
			summary.Risks = append(summary.Risks, item)
		}
		rows.Close()
	}

	// Overdue suppliers
	rows2, err := d.pool.Query(ctx, `
		SELECT identifier, name, criticality, next_review
		FROM suppliers
		WHERE organization_id = $1
			AND next_review IS NOT NULL
			AND next_review < $2
			AND deleted_at IS NULL
		ORDER BY next_review ASC
	`, orgID, now)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var item OverdueItem
			var reviewDate Epoch
			if err := rows2.Scan(&item.EntityID, &item.Title, &item.Criticality, &reviewDate); err != nil {
				break
			}
			item.EntityType = "supplier"
			item.NextReview = &reviewDate
			item.DaysLate = int(now.Sub(reviewDate.Time).Hours() / 24)
			summary.Suppliers = append(summary.Suppliers, item)
		}
		rows2.Close()
	}

	// Overdue systems (access reviews)
	rows3, err := d.pool.Query(ctx, `
		SELECT identifier, name, criticality, next_review
		FROM systems
		WHERE organization_id = $1
			AND next_review IS NOT NULL
			AND next_review < $2
			AND deleted_at IS NULL
		ORDER BY next_review ASC
	`, orgID, now)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var item OverdueItem
			var reviewDate Epoch
			if err := rows3.Scan(&item.EntityID, &item.Title, &item.Criticality, &reviewDate); err != nil {
				break
			}
			item.EntityType = "system"
			item.NextReview = &reviewDate
			item.DaysLate = int(now.Sub(reviewDate.Time).Hours() / 24)
			summary.Systems = append(summary.Systems, item)
		}
		rows3.Close()
	}

	// Overdue legal requirements
	rows4, err := d.pool.Query(ctx, `
		SELECT identifier, title,
			COALESCE((SELECT email FROM users WHERE id = legal_requirements.owner_id), ''),
			next_review, COALESCE(current_level, '')
		FROM legal_requirements
		WHERE organization_id = $1
			AND next_review IS NOT NULL
			AND next_review < $2
			AND deleted_at IS NULL
		ORDER BY next_review ASC
	`, orgID, now)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var item OverdueItem
			var reviewDate Epoch
			if err := rows4.Scan(&item.EntityID, &item.Title, &item.Owner, &reviewDate, &item.Criticality); err != nil {
				break
			}
			item.EntityType = "legal"
			item.NextReview = &reviewDate
			item.DaysLate = int(now.Sub(reviewDate.Time).Hours() / 24)
			summary.Legal = append(summary.Legal, item)
		}
		rows4.Close()
	}

	// Overdue tasks
	rows5, err := d.pool.Query(ctx, `
		SELECT t.id::text, t.title,
			COALESCE((SELECT email FROM users WHERE id = t.assignee_id), ''),
			t.due_date, t.priority
		FROM tasks t
		WHERE t.organization_id = $1
			AND t.status NOT IN ('done', 'cancelled')
			AND t.due_date IS NOT NULL
			AND t.due_date < $2
			AND t.deleted_at IS NULL
		ORDER BY t.due_date ASC
	`, orgID, now)
	if err == nil {
		defer rows5.Close()
		for rows5.Next() {
			var item OverdueItem
			var dueDate Epoch
			if err := rows5.Scan(&item.EntityID, &item.Title, &item.Owner, &dueDate, &item.Criticality); err != nil {
				break
			}
			item.EntityType = "task"
			item.NextReview = &dueDate
			item.DaysLate = int(now.Sub(dueDate.Time).Hours() / 24)
			summary.Tasks = append(summary.Tasks, item)
		}
		rows5.Close()
	}

	summary.TotalCount = len(summary.Risks) + len(summary.Suppliers) +
		len(summary.Systems) + len(summary.Legal) + len(summary.Tasks)

	return summary, nil
}

// CreatedReviewTasks is the result of auto-creating review tasks.
type CreatedReviewTasks struct {
	Created []Task `json:"created"`
	Skipped int    `json:"skipped"` // already had open task
	Total   int    `json:"total"`
}

// CreateOverdueReviewTasks creates tasks for all overdue items.
// Deduplicates: skips if an open task with matching task_type and title substring already exists.
// OverdueDocumentReviews returns documents where the last approved version is older than review_cycle months.
// Joins document_versions with reviews to get owner and title.
// Uses owner and review_cycle_months from document_versions (snapshotted from frontmatter at approval time).
// Falls back to review requester and 12-month default for older versions without snapshots.
func (d *DB) OverdueDocumentReviews(ctx context.Context, orgID int) ([]OverdueItem, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT DISTINCT ON (dv.document_id)
			dv.document_id, dv.version, dv.created_by, dv.created_at,
			COALESCE(r.title, dv.document_id),
			COALESCE(NULLIF(dv.owner, ''), (SELECT email FROM users WHERE id = r.requested_by_id), dv.created_by),
			COALESCE(dv.review_cycle_months, 12)
		FROM document_versions dv
		LEFT JOIN reviews r ON r.organization_id = dv.organization_id
			AND r.document_id = dv.document_id AND r.status = 'merged'
		WHERE dv.organization_id = $1
		ORDER BY dv.document_id, dv.created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	var items []OverdueItem
	for rows.Next() {
		var docID, version, createdBy, title, owner string
		var createdAt Epoch
		var reviewCycleMonths int
		if err := rows.Scan(&docID, &version, &createdBy, &createdAt, &title, &owner, &reviewCycleMonths); err != nil {
			continue
		}
		if reviewCycleMonths <= 0 {
			reviewCycleMonths = 12
		}
		nextReview := createdAt.AddDate(0, reviewCycleMonths, 0)
		if nextReview.After(now) {
			continue // not overdue
		}
		daysLate := int(now.Sub(nextReview).Hours() / 24)
		due := Epoch{Time: nextReview}
		items = append(items, OverdueItem{
			EntityType: "document",
			EntityID:   docID,
			Title:      title,
			Owner:      owner,
			NextReview: &due,
			DaysLate:   daysLate,
		})
	}
	return items, nil
}

// OverdueObjectiveCheckins returns active objectives where the last checkin is older than checkin_cycle months.
// For objectives with no checkins yet, uses started_at (or created_at) as baseline.
func (d *DB) OverdueObjectiveCheckins(ctx context.Context, orgID int) ([]OverdueItem, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT o.id, o.display_id, o.title,
			COALESCE((SELECT email FROM users WHERE id = o.owner_id), ''),
			COALESCE(o.checkin_cycle, 12),
			(SELECT MAX(c.occurred_at) FROM checkins c WHERE c.objective_id = o.id),
			COALESCE(o.started_at, o.created_at)
		FROM objectives o
		WHERE o.organization_id = $1 AND o.status = 'active' AND o.deleted_at IS NULL
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	var items []OverdueItem
	for rows.Next() {
		var id int64
		var displayID, title, owner string
		var cycle int
		var lastCheckin *Epoch
		var baseline Epoch
		if err := rows.Scan(&id, &displayID, &title, &owner, &cycle, &lastCheckin, &baseline); err != nil {
			continue
		}
		if cycle <= 0 {
			cycle = 12
		}
		// Use last checkin if available, otherwise started_at/created_at
		var nextDue time.Time
		if lastCheckin != nil && !lastCheckin.IsZero() {
			nextDue = lastCheckin.AddDate(0, cycle, 0)
		} else {
			nextDue = baseline.AddDate(0, cycle, 0)
		}
		if nextDue.After(now) {
			continue
		}
		daysLate := int(now.Sub(nextDue).Hours() / 24)
		due := Epoch{Time: nextDue}
		items = append(items, OverdueItem{
			EntityType: "objective",
			EntityID:   displayID,
			Title:      title,
			Owner:      owner,
			NextReview: &due,
			DaysLate:   daysLate,
		})
	}
	return items, nil
}

func (d *DB) CreateOverdueReviewTasks(ctx context.Context, orgID int, createdBy string) (*CreatedReviewTasks, error) {
	summary, err := d.GetOverdueSummary(ctx, orgID)
	if err != nil {
		return nil, err
	}

	// Load existing open review tasks to deduplicate.
	existingTasks, err := d.ListTasksWhere(ctx, orgID, TaskViewer{CanSeeAll: true}, "status NOT IN ('done','cancelled')", 1000)
	if err != nil {
		existingTasks = nil
	}
	existing := map[string]bool{}
	for _, t := range existingTasks {
		existing[t.TaskType+":"+t.Title] = true
	}

	result := &CreatedReviewTasks{}

	type pendingTask struct {
		title    string
		taskType string
		priority string
		dueDate  *Epoch
		owner    string
	}

	var pending []pendingTask

	for _, r := range summary.Risks {
		pending = append(pending, pendingTask{
			title:    "Risk review: " + r.Title + " (" + r.EntityID + ")",
			taskType: "risk_review",
			priority: levelToPriority(r.Criticality),
			dueDate:  r.NextReview,
			owner:    r.Owner,
		})
	}
	for _, s := range summary.Suppliers {
		pending = append(pending, pendingTask{
			title:    "Supplier review: " + s.Title + " (" + s.EntityID + ")",
			taskType: "supplier_review",
			priority: criticalityToPriority(s.Criticality),
			dueDate:  s.NextReview,
			owner:    s.Owner,
		})
	}
	for _, s := range summary.Systems {
		pending = append(pending, pendingTask{
			title:    "Access review: " + s.Title + " (" + s.EntityID + ")",
			taskType: "access_review",
			priority: criticalityToPriority(s.Criticality),
			dueDate:  s.NextReview,
			owner:    s.Owner,
		})
	}
	for _, l := range summary.Legal {
		pending = append(pending, pendingTask{
			title:    "Legal review: " + l.Title + " (" + l.EntityID + ")",
			taskType: "legal_review",
			priority: levelToPriority(l.Criticality),
			dueDate:  l.NextReview,
			owner:    l.Owner,
		})
	}

	// Document reviews — from document_versions table (most recent per doc)
	docVersions, _ := d.OverdueDocumentReviews(ctx, orgID)
	for _, dv := range docVersions {
		pending = append(pending, pendingTask{
			title:    "Document review: " + dv.Title + " (" + dv.EntityID + ")",
			taskType: "document_review",
			priority: "medium",
			dueDate:  dv.NextReview,
			owner:    dv.Owner,
		})
	}

	// Objective check-ins overdue
	objCheckins, _ := d.OverdueObjectiveCheckins(ctx, orgID)
	for _, oc := range objCheckins {
		pending = append(pending, pendingTask{
			title:    "Objective check-in: " + oc.Title + " (" + oc.EntityID + ")",
			taskType: "objective_checkin",
			priority: "medium",
			dueDate:  oc.NextReview,
			owner:    oc.Owner,
		})
	}

	result.Total = len(pending)

	for _, p := range pending {
		key := p.taskType + ":" + p.title
		if existing[key] {
			result.Skipped++
			continue
		}
		assignee := p.owner
		if assignee == "" {
			assignee = createdBy
		}
		task := Task{
			Title:     p.title,
			TaskType:  p.taskType,
			Assignee:  assignee,
			CreatedBy: createdBy,
			Status:    "open",
			Priority:  p.priority,
			DueDate:   p.dueDate,
		}
		if err := d.CreateTask(ctx, orgID, &task); err != nil {
			continue
		}
		result.Created = append(result.Created, task)
		existing[key] = true
	}

	return result, nil
}

func levelToPriority(level string) string {
	switch level {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func criticalityToPriority(crit string) string {
	switch crit {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}
