// Package mcp implements a Model Context Protocol server for the ISMS platform.
// It exposes ISMS entities, suggestions, and documents as MCP tools over stdio.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════
// JSON-RPC / MCP PROTOCOL
// ═══════════════════════════════════════════════════════════════════════

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ═══════════════════════════════════════════════════════════════════════
// API CLIENT
// ═══════════════════════════════════════════════════════════════════════

type apiClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func (c *apiClient) get(path string) (json.RawMessage, error) {
	u := c.baseURL + "/api/v1" + path
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(body))
	}
	return json.RawMessage(body), nil
}

func (c *apiClient) post(path string, payload interface{}) (json.RawMessage, error) {
	return c.doJSON("POST", path, payload)
}

func (c *apiClient) doJSON(method, path string, payload interface{}) (json.RawMessage, error) {
	u := c.baseURL + "/api/v1" + path
	var bodyReader io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, u, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(body))
	}
	return json.RawMessage(body), nil
}

// ═══════════════════════════════════════════════════════════════════════
// TOOL DEFINITIONS
// ═══════════════════════════════════════════════════════════════════════

func toolDefinitions() []mcpTool {
	tools := []mcpTool{
		{
			Name:        "list_entities",
			Description: "List operational entities (risks, incidents, suppliers, legal_requirements, changes, corrective_actions, systems, assets, objectives, tasks). Returns array of entities.",
			InputSchema: jsonSchema(map[string]prop{
				"entity_type": {Type: "string", Desc: "Entity type to list (risk, incident, supplier, legal_requirement, change, corrective_action, system, asset, objective, task)"},
				"status":      {Type: "string", Desc: "Optional status filter"},
			}, []string{"entity_type"}),
		},
		{
			Name:        "get_entity",
			Description: "Get a single entity by type and ID. Returns full entity details.",
			InputSchema: jsonSchema(map[string]prop{
				"entity_type": {Type: "string", Desc: "Entity type"},
				"entity_id":   {Type: "string", Desc: "Entity identifier (e.g. RISK-1, SUPPLIER-3, or numeric ID)"},
			}, []string{"entity_type", "entity_id"}),
		},
		{
			Name:        "get_entity_history",
			Description: "Get the change history (audit trail) for an entity. Returns changelog entries showing what changed, when, and by whom.",
			InputSchema: jsonSchema(map[string]prop{
				"entity_type": {Type: "string", Desc: "Entity type"},
				"entity_id":   {Type: "string", Desc: "Entity identifier or numeric ID"},
			}, []string{"entity_type", "entity_id"}),
		},
		{
			Name:        "get_entity_links",
			Description: "Get cross-references for an entity. Shows what other entities are linked to it.",
			InputSchema: jsonSchema(map[string]prop{
				"entity_type": {Type: "string", Desc: "Entity type"},
				"entity_id":   {Type: "string", Desc: "Entity identifier"},
			}, []string{"entity_type", "entity_id"}),
		},
		{
			Name:        "list_documents",
			Description: "List all documents in the ISMS with their metadata (title, status, author, document_id, version).",
			InputSchema: jsonSchema(map[string]prop{}, nil),
		},
		{
			Name:        "get_document",
			Description: "Get a document by its document_id. Returns the full markdown body and frontmatter metadata.",
			InputSchema: jsonSchema(map[string]prop{
				"document_id": {Type: "string", Desc: "Document identifier (e.g. iso27001-4-1, risk-management-policy)"},
			}, []string{"document_id"}),
		},
		{
			Name:        "list_suggestions",
			Description: "List entity suggestions. Can filter by status, entity_type, or entity_id.",
			InputSchema: jsonSchema(map[string]prop{
				"status":      {Type: "string", Desc: "Filter by status: open, in_review, applied, rejected"},
				"entity_type": {Type: "string", Desc: "Filter by entity type"},
				"entity_id":   {Type: "string", Desc: "Filter by entity ID"},
			}, nil),
		},
		{
			Name:        "create_suggestion",
			Description: "Create a suggestion to propose an operational change. Suggestions are reviewed and applied by managers. Use this to propose new entities, updates, reassessments, links, or document reviews.",
			InputSchema: jsonSchema(map[string]prop{
				"entity_type":     {Type: "string", Desc: "Target entity type (risk, incident, supplier, legal_requirement, change_request, corrective_action, objective, task, document)"},
				"entity_id":       {Type: "string", Desc: "Target entity ID (omit for 'create' suggestions)"},
				"suggestion_type": {Type: "string", Desc: "Type: create, update, reassess, link, review"},
				"title":           {Type: "string", Desc: "Short descriptive title for the suggestion"},
				"payload":         {Type: "object", Desc: "Module-specific payload (fields to create/update)"},
				"rationale":       {Type: "string", Desc: "Why this change is being suggested"},
				"source_refs":     {Type: "array", Desc: "Evidence: array of {type, id} objects linking to supporting entities"},
			}, []string{"entity_type", "suggestion_type", "title", "payload"}),
		},
		{
			Name:        "apply_suggestion",
			Description: "Apply a suggestion — atomically creates or updates the target entity. Only managers/admins can apply. Use force=true to override stale warnings.",
			InputSchema: jsonSchema(map[string]prop{
				"suggestion_id": {Type: "string", Desc: "Suggestion ID to apply"},
				"force":         {Type: "boolean", Desc: "Force apply even if entity changed since suggestion was created"},
			}, []string{"suggestion_id"}),
		},
		{
			Name:        "reject_suggestion",
			Description: "Reject a suggestion with a reason. The original suggester will be notified.",
			InputSchema: jsonSchema(map[string]prop{
				"suggestion_id": {Type: "string", Desc: "Suggestion ID to reject"},
				"reason":        {Type: "string", Desc: "Why the suggestion is being rejected"},
			}, []string{"suggestion_id", "reason"}),
		},
		// ── Overview + operational awareness ──
		{
			Name:        "get_isms_overview",
			Description: "Get a complete overview of the ISMS: counts of all entity types, overdue items, open reviews, open suggestions, recent activity. Use this first to understand the current state of the management system.",
			InputSchema: jsonSchema(map[string]prop{}, nil),
		},
		{
			Name:        "get_overdue_items",
			Description: "Get all overdue items across the ISMS: risks, suppliers, systems, legal requirements, tasks, documents past their review cycle, and objectives with overdue check-ins.",
			InputSchema: jsonSchema(map[string]prop{}, nil),
		},
		// ── Document review participation ──
		{
			Name:        "list_reviews",
			Description: "List document reviews. Can filter by status (pending, changes_requested, approved, merged, closed).",
			InputSchema: jsonSchema(map[string]prop{
				"status": {Type: "string", Desc: "Filter by status"},
			}, nil),
		},
		{
			Name:        "get_review",
			Description: "Get a document review with full details including assignments, round number, and status.",
			InputSchema: jsonSchema(map[string]prop{
				"review_id": {Type: "string", Desc: "Review ID"},
			}, []string{"review_id"}),
		},
		{
			Name:        "get_review_diff",
			Description: "Get the diff (changes) for a document review. Shows what changed in the document.",
			InputSchema: jsonSchema(map[string]prop{
				"review_id": {Type: "string", Desc: "Review ID"},
			}, []string{"review_id"}),
		},
		{
			Name:        "comment_on_review",
			Description: "Add a comment to a document review. Can target a specific paragraph with paragraph_index and quote. Can include a suggestion_body to propose replacement text.",
			InputSchema: jsonSchema(map[string]prop{
				"review_id":       {Type: "string", Desc: "Review ID"},
				"body":            {Type: "string", Desc: "Comment text"},
				"paragraph_index": {Type: "number", Desc: "Target paragraph index (0-based) for inline comments"},
				"quote":           {Type: "string", Desc: "Quoted text from the paragraph being commented on"},
				"suggestion_body": {Type: "string", Desc: "Proposed replacement text for the paragraph (creates an inline suggestion)"},
			}, []string{"review_id", "body"}),
		},
		{
			Name:        "approve_review",
			Description: "Approve or request changes on a document review. Decision can be: approved, changes_requested, or proposed_revision.",
			InputSchema: jsonSchema(map[string]prop{
				"review_id": {Type: "string", Desc: "Review ID"},
				"decision":  {Type: "string", Desc: "Decision: approved, changes_requested, or proposed_revision"},
				"comment":   {Type: "string", Desc: "Review note explaining the decision"},
			}, []string{"review_id", "decision"}),
		},
		{
			Name:        "get_review_content",
			Description: "Get the current document content on the review branch. Use this to read the document being reviewed.",
			InputSchema: jsonSchema(map[string]prop{
				"review_id": {Type: "string", Desc: "Review ID"},
			}, []string{"review_id"}),
		},
		{
			Name:        "merge_review",
			Description: "Merge an approved review — publishes the document version. This is the final step that makes the reviewed document official. Only works on fully approved reviews.",
			InputSchema: jsonSchema(map[string]prop{
				"review_id": {Type: "string", Desc: "Review ID to merge"},
			}, []string{"review_id"}),
		},
		{
			Name:        "edit_review_content",
			Description: "Edit document content on the review branch. Use this to address reviewer comments by modifying the document. The content should be the full document body (markdown).",
			InputSchema: jsonSchema(map[string]prop{
				"review_id": {Type: "string", Desc: "Review ID"},
				"content":   {Type: "string", Desc: "Full document body (markdown) to write to the review branch"},
				"message":   {Type: "string", Desc: "Commit message describing the changes"},
			}, []string{"review_id", "content"}),
		},
		{
			Name:        "confirm_document_review",
			Description: "Confirm that a document has been reviewed and is still valid — used for periodic/annual reviews where no changes are needed. Creates audit evidence (approval record, decision log, version snapshot) without requiring a full review cycle. The document owner should confirm the AI review summary before this is final.",
			InputSchema: jsonSchema(map[string]prop{
				"document_id": {Type: "string", Desc: "Document ID to confirm review for"},
				"comment":     {Type: "string", Desc: "Review summary explaining why the document is still valid, what was checked, and any minor observations"},
			}, []string{"document_id", "comment"}),
		},
	}

	// ── Agent workflow ──
	tools = append(tools,
		mcpTool{
			Name:        "get_pending_actions",
			Description: "Get actions this agent should take next. Returns reviews awaiting resubmit (you are author, status is changes_requested), reviews awaiting your re-review (you are reviewer, assignment is pending), and agent-actionable notifications. Actor-scoped to the authenticated API token user.",
			InputSchema: jsonSchema(map[string]prop{}, nil),
		},
	)

	return tools
}

// ═══════════════════════════════════════════════════════════════════════
// TOOL DISPATCH + IMPLEMENTATIONS
// ═══════════════════════════════════════════════════════════════════════

func (c *apiClient) callTool(name string, args map[string]interface{}) (*mcpToolResult, error) {
	switch name {
	case "list_entities":
		return c.toolListEntities(args)
	case "get_entity":
		return c.toolGetEntity(args)
	case "get_entity_history":
		return c.toolGetEntityHistory(args)
	case "get_entity_links":
		return c.toolGetEntityLinks(args)
	case "list_documents":
		return c.toolListDocuments()
	case "get_document":
		return c.toolGetDocument(args)
	case "list_suggestions":
		return c.toolListSuggestions(args)
	case "create_suggestion":
		return c.toolCreateSuggestion(args)
	case "apply_suggestion":
		return c.toolApplySuggestion(args)
	case "reject_suggestion":
		return c.toolRejectSuggestion(args)
	case "get_isms_overview":
		return c.toolGetISMSOverview()
	case "get_overdue_items":
		return c.toolGetOverdueItems()
	case "list_reviews":
		return c.toolListReviews(args)
	case "get_review":
		return c.toolGetReview(args)
	case "get_review_diff":
		return c.toolGetReviewDiff(args)
	case "comment_on_review":
		return c.toolCommentOnReview(args)
	case "approve_review":
		return c.toolApproveReview(args)
	case "get_review_content":
		return c.toolGetReviewContent(args)
	case "merge_review":
		return c.toolMergeReview(args)
	case "edit_review_content":
		return c.toolEditReviewContent(args)
	case "confirm_document_review":
		return c.toolConfirmDocumentReview(args)
	case "get_pending_actions":
		return c.toolGetPendingActions()
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (c *apiClient) toolListEntities(args map[string]interface{}) (*mcpToolResult, error) {
	entityType := str(args, "entity_type")
	path := entityTypeToAPIPath(entityType)
	if path == "" {
		return errResult("unknown entity_type: " + entityType), nil
	}
	if status := str(args, "status"); status != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	data, err := c.get(path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolGetEntity(args map[string]interface{}) (*mcpToolResult, error) {
	entityType := str(args, "entity_type")
	entityID := str(args, "entity_id")
	path := entityTypeToAPIPath(entityType)
	if path == "" {
		return errResult("unknown entity_type: " + entityType), nil
	}
	numID := stripPrefix(entityID)
	data, err := c.get(path + "/" + url.PathEscape(numID))
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolGetEntityHistory(args map[string]interface{}) (*mcpToolResult, error) {
	entityType := str(args, "entity_type")
	entityID := str(args, "entity_id")
	numID := stripPrefix(entityID)
	data, err := c.get("/changelog/" + url.PathEscape(entityType) + "/" + url.PathEscape(numID))
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolGetEntityLinks(args map[string]interface{}) (*mcpToolResult, error) {
	entityType := str(args, "entity_type")
	entityID := str(args, "entity_id")
	q := url.Values{}
	q.Set("type", entityType)
	q.Set("id", entityID)
	data, err := c.get("/references?" + q.Encode())
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolListDocuments() (*mcpToolResult, error) {
	data, err := c.get("/documents/all")
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolGetDocument(args map[string]interface{}) (*mcpToolResult, error) {
	docID := str(args, "document_id")
	data, err := c.get("/documents/" + url.PathEscape(docID) + "/body")
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolListSuggestions(args map[string]interface{}) (*mcpToolResult, error) {
	q := url.Values{}
	if v := str(args, "status"); v != "" {
		q.Set("status", v)
	}
	if v := str(args, "entity_type"); v != "" {
		q.Set("entity_type", v)
	}
	if v := str(args, "entity_id"); v != "" {
		q.Set("entity_id", v)
	}
	path := "/suggestions"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	data, err := c.get(path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolCreateSuggestion(args map[string]interface{}) (*mcpToolResult, error) {
	body := map[string]interface{}{
		"entity_type":       str(args, "entity_type"),
		"suggestion_type":   str(args, "suggestion_type"),
		"title":             str(args, "title"),
		"payload":           args["payload"],
		"suggested_by_type": "agent",
	}
	if v := str(args, "entity_id"); v != "" {
		body["entity_id"] = v
	}
	if v := str(args, "rationale"); v != "" {
		body["rationale"] = v
	}
	if v, ok := args["source_refs"]; ok {
		body["source_refs"] = v
	}
	data, err := c.post("/suggestions", body)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolApplySuggestion(args map[string]interface{}) (*mcpToolResult, error) {
	id := str(args, "suggestion_id")
	body := map[string]interface{}{}
	if v, ok := args["force"]; ok {
		body["force"] = v
	}
	data, err := c.post("/suggestions/"+url.PathEscape(id)+"/apply", body)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolRejectSuggestion(args map[string]interface{}) (*mcpToolResult, error) {
	id := str(args, "suggestion_id")
	reason := str(args, "reason")
	data, err := c.post("/suggestions/"+url.PathEscape(id)+"/reject", map[string]string{"reason": reason})
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

// ═══════════════════════════════════════════════════════════════════════
// OVERVIEW + OPERATIONAL AWARENESS
// ═══════════════════════════════════════════════════════════════════════

func (c *apiClient) toolGetISMSOverview() (*mcpToolResult, error) {
	type result struct {
		key  string
		data json.RawMessage
		err  error
	}

	endpoints := map[string]string{
		"risks":                 "/risks",
		"incidents":             "/incidents",
		"suppliers":             "/suppliers",
		"legal":                 "/legal",
		"changes":               "/changes",
		"corrective_actions":    "/corrective-actions",
		"tasks":                 "/tasks",
		"reviews":               "/reviews",
		"open_suggestions":      "/suggestions?status=open",
		"reviewing_suggestions": "/suggestions?status=in_review",
		"overdue":               "/overdue",
		"documents":             "/documents/all",
	}

	// Fetch all endpoints in parallel
	ch := make(chan result, len(endpoints))
	for key, path := range endpoints {
		go func(k, p string) {
			data, err := c.get(p)
			ch <- result{key: k, data: data, err: err}
		}(key, path)
	}

	overview := map[string]interface{}{}
	for range endpoints {
		r := <-ch
		if r.err != nil {
			overview[r.key] = map[string]string{"error": r.err.Error()}
			continue
		}
		var wrapped struct {
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(r.data, &wrapped) == nil && wrapped.Data != nil {
			var arr []json.RawMessage
			if json.Unmarshal(wrapped.Data, &arr) == nil {
				overview[r.key+"_count"] = len(arr)
			}
			overview[r.key] = wrapped.Data
		} else {
			overview[r.key] = r.data
		}
	}

	b, _ := json.MarshalIndent(overview, "", "  ")
	return textResult(string(b)), nil
}

func (c *apiClient) toolGetOverdueItems() (*mcpToolResult, error) {
	data, err := c.get("/overdue")
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

// ═══════════════════════════════════════════════════════════════════════
// DOCUMENT REVIEW PARTICIPATION
// ═══════════════════════════════════════════════════════════════════════

func (c *apiClient) toolListReviews(args map[string]interface{}) (*mcpToolResult, error) {
	path := "/reviews"
	if status := str(args, "status"); status != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	data, err := c.get(path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolGetReview(args map[string]interface{}) (*mcpToolResult, error) {
	id := str(args, "review_id")
	data, err := c.get("/reviews/" + url.PathEscape(id))
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolGetReviewDiff(args map[string]interface{}) (*mcpToolResult, error) {
	id := str(args, "review_id")
	data, err := c.get("/reviews/" + url.PathEscape(id) + "/diff")
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolCommentOnReview(args map[string]interface{}) (*mcpToolResult, error) {
	id := str(args, "review_id")
	body := map[string]interface{}{
		"body": str(args, "body"),
	}
	if v := str(args, "quote"); v != "" {
		body["quote"] = v
	}
	if v := str(args, "suggestion_body"); v != "" {
		body["suggestion_body"] = v
	}
	if v, ok := args["paragraph_index"]; ok {
		body["paragraph_index"] = v
	}
	data, err := c.post("/reviews/"+url.PathEscape(id)+"/comment", body)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolApproveReview(args map[string]interface{}) (*mcpToolResult, error) {
	id := str(args, "review_id")
	body := map[string]interface{}{
		"decision": str(args, "decision"),
	}
	if v := str(args, "comment"); v != "" {
		body["comment"] = v
	}
	data, err := c.post("/reviews/"+url.PathEscape(id)+"/approve", body)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolGetReviewContent(args map[string]interface{}) (*mcpToolResult, error) {
	id := str(args, "review_id")
	data, err := c.get("/reviews/" + url.PathEscape(id) + "/content")
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolGetPendingActions() (*mcpToolResult, error) {
	data, err := c.get("/agent/pending-actions")
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolEditReviewContent(args map[string]interface{}) (*mcpToolResult, error) {
	id := str(args, "review_id")
	body := map[string]interface{}{
		"content": str(args, "content"),
	}
	if v := str(args, "message"); v != "" {
		body["message"] = v
	}
	data, err := c.doJSON("PUT", "/reviews/"+url.PathEscape(id)+"/content", body)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolConfirmDocumentReview(args map[string]interface{}) (*mcpToolResult, error) {
	docID := str(args, "document_id")
	comment := str(args, "comment")
	data, err := c.post("/documents/"+url.PathEscape(docID)+"/confirm-review", map[string]string{"comment": comment})
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

func (c *apiClient) toolMergeReview(args map[string]interface{}) (*mcpToolResult, error) {
	id := str(args, "review_id")
	data, err := c.post("/reviews/"+url.PathEscape(id)+"/merge", nil)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return textResult(string(data)), nil
}

// ═══════════════════════════════════════════════════════════════════════
// HELPERS
// ═══════════════════════════════════════════════════════════════════════

func entityTypeToAPIPath(t string) string {
	switch t {
	case "risk":
		return "/risks"
	case "incident":
		return "/incidents"
	case "supplier":
		return "/suppliers"
	case "legal_requirement":
		return "/legal"
	case "change", "change_request":
		return "/changes"
	case "corrective_action":
		return "/corrective-actions"
	case "system":
		return "/systems"
	case "asset":
		return "/assets"
	case "objective":
		return "/objectives"
	case "task":
		return "/tasks"
	default:
		return ""
	}
}

func stripPrefix(id string) string {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) == 2 {
		allLetters := true
		for _, c := range parts[0] {
			if c < 'A' || c > 'Z' {
				allLetters = false
				break
			}
		}
		if allLetters {
			return parts[1]
		}
	}
	return id
}

func str(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		if f, ok := v.(float64); ok {
			return fmt.Sprintf("%.0f", f)
		}
	}
	return ""
}

func textResult(text string) *mcpToolResult {
	return &mcpToolResult{Content: []mcpContent{{Type: "text", Text: text}}}
}

func errResult(msg string) *mcpToolResult {
	return &mcpToolResult{Content: []mcpContent{{Type: "text", Text: "Error: " + msg}}, IsError: true}
}

type prop struct {
	Type string
	Desc string
}

func jsonSchema(props map[string]prop, required []string) map[string]interface{} {
	properties := map[string]interface{}{}
	for k, v := range props {
		properties[k] = map[string]string{"type": v.Type, "description": v.Desc}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// ═══════════════════════════════════════════════════════════════════════
// SERVE: STDIO MCP SERVER LOOP
// ═══════════════════════════════════════════════════════════════════════

// Serve runs the MCP server on stdin/stdout. Blocks until stdin is closed.
func Serve(apiURL, apiToken string) error {
	client := &apiClient{
		baseURL: strings.TrimRight(apiURL, "/"),
		token:   apiToken,
		http:    &http.Client{Timeout: 30 * time.Second},
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(jsonRPCResponse{
				JSONRPC: "2.0",
				Error:   &jsonRPCError{Code: -32700, Message: "parse error"},
			})
			continue
		}

		resp := handleRequest(client, &req)
		if resp != nil {
			writeResponse(*resp)
		}
	}
	return scanner.Err()
}

func handleRequest(client *apiClient, req *jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]interface{}{"name": "isms-mcp", "version": "1.0.0"},
			},
		}
	case "notifications/initialized":
		return nil
	case "tools/list":
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"tools": toolDefinitions()},
		}
	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &jsonRPCError{Code: -32602, Message: "invalid params"}}
		}
		result, err := client.callTool(params.Name, params.Arguments)
		if err != nil {
			return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: errResult(err.Error())}
		}
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
	case "ping":
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{}}
	default:
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &jsonRPCError{Code: -32601, Message: "method not found: " + req.Method}}
	}
}

func writeResponse(resp jsonRPCResponse) {
	b, _ := json.Marshal(resp)
	os.Stdout.Write(b)
	os.Stdout.Write([]byte("\n"))
}
