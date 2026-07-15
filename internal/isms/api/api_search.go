package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// SearchEntry is a single searchable item in the index.
type SearchEntry struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Search string `json:"-"` // lowercase concatenation of all searchable fields
}

// SearchIndex is a per-org in-memory search index that avoids 10+ DB queries per keystroke.
type SearchIndex struct {
	mu       sync.RWMutex
	entries  map[int][]SearchEntry // orgID -> entries
	built    map[int]time.Time     // orgID -> last build time
	building sync.Map              // orgID -> struct{} — prevents concurrent builds for same org
}

const searchIndexTTL = 5 * time.Minute

// NewSearchIndex creates an empty search index.
func NewSearchIndex() *SearchIndex {
	return &SearchIndex{
		entries: make(map[int][]SearchEntry),
		built:   make(map[int]time.Time),
	}
}

// Invalidate removes the cached index for an org, forcing a rebuild on next search.
func (idx *SearchIndex) Invalidate(orgID int) {
	idx.mu.Lock()
	delete(idx.entries, orgID)
	delete(idx.built, orgID)
	idx.mu.Unlock()
}

// Search returns matching entries for the query, rebuilding the index if stale or missing.
func (idx *SearchIndex) Search(orgID int, query string, build func()) []SearchEntry {
	query = strings.ToLower(strings.TrimSpace(query))

	idx.mu.RLock()
	entries, ok := idx.entries[orgID]
	builtAt := idx.built[orgID]
	idx.mu.RUnlock()

	stale := !ok || time.Since(builtAt) > searchIndexTTL

	if stale {
		// Trigger a rebuild. Use building map to prevent concurrent builds for the same org.
		if _, loaded := idx.building.LoadOrStore(orgID, struct{}{}); !loaded {
			if !ok {
				// No index at all — build synchronously so we have results.
				build()
				idx.building.Delete(orgID)

				idx.mu.RLock()
				entries = idx.entries[orgID]
				idx.mu.RUnlock()
			} else {
				// Stale but present — rebuild in background, serve stale results now.
				go func() {
					defer idx.building.Delete(orgID)
					build()
				}()
			}
		}
	}

	if query == "" {
		if len(entries) > 50 {
			return entries[:50]
		}
		return entries
	}

	var results []SearchEntry
	for _, e := range entries {
		if strings.Contains(e.Search, query) {
			results = append(results, e)
			if len(results) >= 50 {
				break
			}
		}
	}
	return results
}

// Set replaces the index entries for an org.
func (idx *SearchIndex) Set(orgID int, entries []SearchEntry) {
	idx.mu.Lock()
	idx.entries[orgID] = entries
	idx.built[orgID] = time.Now()
	idx.mu.Unlock()
}

// Upsert adds or updates a single entry in the index for an org.
// If the index doesn't exist yet, the entry is silently dropped (next search will rebuild).
func (idx *SearchIndex) Upsert(orgID int, entry SearchEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	entries, ok := idx.entries[orgID]
	if !ok {
		return // no index yet — next search will build it
	}

	// Update existing or append
	for i, e := range entries {
		if e.Type == entry.Type && e.ID == entry.ID {
			entries[i] = entry
			return
		}
	}
	idx.entries[orgID] = append(entries, entry)
}

// Remove deletes a single entry from the index for an org.
func (idx *SearchIndex) Remove(orgID int, entryType, entryID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	entries, ok := idx.entries[orgID]
	if !ok {
		return
	}

	for i, e := range entries {
		if e.Type == entryType && e.ID == entryID {
			idx.entries[orgID] = append(entries[:i], entries[i+1:]...)
			return
		}
	}
}

// buildSearchIndex loads all entity types for an org and populates the index.
func (s *Server) buildSearchIndex(orgID int) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		all []SearchEntry
	)

	collect := func(fn func() []SearchEntry) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entries := fn()
			if len(entries) > 0 {
				mu.Lock()
				all = append(all, entries...)
				mu.Unlock()
			}
		}()
	}

	// Documents — from git store
	collect(func() []SearchEntry {
		st, err := s.storeForOrg(ctx, orgID)
		if err != nil {
			return nil
		}
		var entries []SearchEntry
		for _, folder := range st.ListDocFolders() {
			docs, err := st.LoadDocumentsFromDir(folder)
			if err != nil {
				continue
			}
			for _, d := range docs {
				docType := d.Frontmatter.Type
				if docType == "" {
					docType = "document"
				}
				entries = append(entries, SearchEntry{
					Type:   docType,
					ID:     d.Frontmatter.DocumentID,
					Title:  d.Frontmatter.Title,
					Search: strings.ToLower(docType + " " + d.Frontmatter.DocumentID + " " + d.Frontmatter.Title),
				})
			}
		}
		return entries
	})

	// Risks
	collect(func() []SearchEntry {
		items, err := s.db.ListRisks(ctx, orgID)
		if err != nil {
			return nil
		}
		entries := make([]SearchEntry, 0, len(items))
		for _, r := range items {
			entries = append(entries, SearchEntry{
				Type:   "risk",
				ID:     r.Identifier,
				Title:  r.Title,
				Search: strings.ToLower(r.Identifier + " " + r.Title + " " + r.Description + " " + r.Category),
			})
		}
		return entries
	})

	// Legal requirements
	collect(func() []SearchEntry {
		items, err := s.db.ListLegalRequirements(ctx, orgID, "")
		if err != nil {
			return nil
		}
		entries := make([]SearchEntry, 0, len(items))
		for _, lr := range items {
			entries = append(entries, SearchEntry{
				Type:   "legal",
				ID:     lr.Identifier,
				Title:  lr.Title,
				Search: strings.ToLower(lr.Identifier + " " + lr.Title + " " + lr.Description + " " + lr.Jurisdiction),
			})
		}
		return entries
	})

	// Suppliers
	collect(func() []SearchEntry {
		items, err := s.db.ListSuppliers(ctx, orgID)
		if err != nil {
			return nil
		}
		entries := make([]SearchEntry, 0, len(items))
		for _, sup := range items {
			entries = append(entries, SearchEntry{
				Type:   "supplier",
				ID:     sup.Identifier,
				Title:  sup.Name,
				Search: strings.ToLower(sup.Identifier + " " + sup.Name + " " + sup.Notes),
			})
		}
		return entries
	})

	// Assets
	collect(func() []SearchEntry {
		items, err := s.db.ListAssets(ctx, orgID)
		if err != nil {
			return nil
		}
		entries := make([]SearchEntry, 0, len(items))
		for _, a := range items {
			entries = append(entries, SearchEntry{
				Type:   "asset",
				ID:     a.Identifier,
				Title:  a.Name,
				Search: strings.ToLower(a.Identifier + " " + a.Name + " " + a.Description),
			})
		}
		return entries
	})

	// Systems
	collect(func() []SearchEntry {
		items, err := s.db.ListSystems(ctx, orgID)
		if err != nil {
			return nil
		}
		entries := make([]SearchEntry, 0, len(items))
		for _, sys := range items {
			entries = append(entries, SearchEntry{
				Type:   "system",
				ID:     sys.Identifier,
				Title:  sys.Name,
				Search: strings.ToLower(sys.Identifier + " " + sys.Name + " " + sys.Description),
			})
		}
		return entries
	})

	// Incidents
	collect(func() []SearchEntry {
		items, err := s.db.ListIncidents(ctx, orgID, "", "", 0)
		if err != nil {
			return nil
		}
		entries := make([]SearchEntry, 0, len(items))
		for _, inc := range items {
			entries = append(entries, SearchEntry{
				Type:   "incident",
				ID:     inc.Identifier,
				Title:  inc.Title,
				Search: strings.ToLower(inc.Identifier + " " + inc.Title + " " + inc.Description),
			})
		}
		return entries
	})

	// Tasks
	collect(func() []SearchEntry {
		// The search index is an org-wide shared/cached structure, so it can't be
		// filtered per viewer at build time. Include ONLY public tasks (zero
		// TaskViewer → public-only) so a private task's title can't leak through
		// universal search. Trade-off: private tasks aren't universally searchable
		// (tracked as a follow-up — needs query-time visibility filtering).
		items, err := s.db.ListTasks(ctx, orgID, db.TaskViewer{}, "", "", 0)
		if err != nil {
			return nil
		}
		entries := make([]SearchEntry, 0, len(items))
		for _, t := range items {
			entries = append(entries, SearchEntry{
				Type:   "task",
				ID:     t.Identifier,
				Title:  t.Title,
				Search: strings.ToLower(t.Identifier + " " + t.Title + " " + t.Description),
			})
		}
		return entries
	})

	// Changes
	collect(func() []SearchEntry {
		items, err := s.db.ListChangeRequests(ctx, orgID, "", 0)
		if err != nil {
			return nil
		}
		entries := make([]SearchEntry, 0, len(items))
		for _, cr := range items {
			entries = append(entries, SearchEntry{
				Type:   "change",
				ID:     cr.Identifier,
				Title:  cr.Title,
				Search: strings.ToLower(cr.Identifier + " " + cr.Title + " " + cr.Description),
			})
		}
		return entries
	})

	// Corrective actions
	collect(func() []SearchEntry {
		items, err := s.db.ListCorrectiveActions(ctx, orgID, "", "", "", 0)
		if err != nil {
			return nil
		}
		entries := make([]SearchEntry, 0, len(items))
		for _, ca := range items {
			entries = append(entries, SearchEntry{
				Type:   "corrective_action",
				ID:     ca.Identifier,
				Title:  ca.Title,
				Search: strings.ToLower(ca.Identifier + " " + ca.Title + " " + ca.Description),
			})
		}
		return entries
	})

	wg.Wait()

	s.searchIndex.Set(orgID, all)
	log.Printf("search: rebuilt index for org %d (%d entries, %dms)", orgID, len(all), time.Since(start).Milliseconds())
}

// searchUpsert updates a single entry in the search index after a create or update.
func (s *Server) searchUpsert(orgID int, entryType, id, title, searchText string) {
	s.searchIndex.Upsert(orgID, SearchEntry{
		Type:   entryType,
		ID:     id,
		Title:  title,
		Search: strings.ToLower(searchText),
	})
}

// searchRemove removes a single entry from the search index after a delete.
func (s *Server) searchRemove(orgID int, entryType, id string) {
	s.searchIndex.Remove(orgID, entryType, id)
}

func (s *Server) handleUniversalSearch(c echo.Context) error {
	orgID := getOrgID(c)
	q := strings.TrimSpace(c.QueryParam("q"))

	results := s.searchIndex.Search(orgID, q, func() {
		s.buildSearchIndex(orgID)
	})

	return c.JSON(http.StatusOK, map[string]interface{}{"data": results})
}
