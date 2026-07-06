// Package client provides an HTTP client for the ISMS API.
// Used by the CLI when operating in remote/API mode (ISMS_API_URL set).
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"isms.sh/internal/isms/db"
)

// Client is an HTTP client for the ISMS API.
type Client struct {
	baseURL        string
	httpClient     *http.Client
	bearerToken    string // ISMS_API_TOKEN
	cfClientID     string // CF_ACCESS_CLIENT_ID
	cfClientSecret string // CF_ACCESS_CLIENT_SECRET
	organization   string // ISMS_ORGANIZATION (slug)
}

// Config holds client configuration.
type Config struct {
	BaseURL        string
	BearerToken    string
	CFClientID     string
	CFClientSecret string
	Organization   string
}

// New creates a new API client.
func New(cfg Config) *Client {
	return &Client{
		baseURL:        strings.TrimRight(cfg.BaseURL, "/"),
		bearerToken:    cfg.BearerToken,
		cfClientID:     cfg.CFClientID,
		cfClientSecret: cfg.CFClientSecret,
		organization:   cfg.Organization,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

// BaseURL returns the API base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) do(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshalling request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Auth headers
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	} else if c.cfClientID != "" {
		req.Header.Set("CF-Access-Client-Id", c.cfClientID)
		req.Header.Set("CF-Access-Client-Secret", c.cfClientSecret)
	}
	if c.organization != "" {
		req.Header.Set("X-Organization-UUID", c.organization)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) get(path string) ([]byte, error) {
	return c.do("GET", path, nil)
}

func (c *Client) post(path string, body interface{}) ([]byte, error) {
	return c.do("POST", path, body)
}

func (c *Client) put(path string, body interface{}) ([]byte, error) {
	return c.do("PUT", path, body)
}

func (c *Client) delete(path string) ([]byte, error) {
	return c.do("DELETE", path, nil)
}

// unwrapList extracts the "data" array from a wrapped response.
func unwrapList(body []byte, target interface{}) error {
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		// Fallback: try direct unmarshal
		return json.Unmarshal(body, target)
	}
	if wrapper.Data == nil {
		// No "data" key found, try direct unmarshal
		return json.Unmarshal(body, target)
	}
	return json.Unmarshal(wrapper.Data, target)
}

// --- Documents (unified) ---

// DocSummary is the API response for a document in a folder listing.
type DocSummary struct {
	DocumentID string `json:"document_id"`
	Title      string `json:"title"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Author     string `json:"author"`
	Path       string `json:"path"`
	Folder     string `json:"folder"`
}

// DocFolder is a folder of documents returned by /documents/all.
type DocFolder struct {
	Name       string       `json:"name"`
	Title      string       `json:"title,omitempty"`
	Files      []DocSummary `json:"files"`
	SubFolders []DocFolder  `json:"subfolders,omitempty"`
}

// DocBody is the API response for a single document body.
type DocBody struct {
	DocumentID string `json:"document_id"`
	Title      string `json:"title"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Author     string `json:"author"`
	Body       string `json:"body"`
	Path       string `json:"path"`
}

// ListAllDocuments returns all documents grouped by folder.
func (c *Client) ListAllDocuments() ([]DocFolder, error) {
	data, err := c.get("/v1/documents/all")
	if err != nil {
		return nil, err
	}
	var result []DocFolder
	return result, unwrapList(data, &result)
}

// GetDocumentBody returns a single document with its body content.
func (c *Client) GetDocumentBody(docID string) (*DocBody, error) {
	data, err := c.get("/v1/documents/" + docID + "/body")
	if err != nil {
		return nil, err
	}
	var result DocBody
	return &result, json.Unmarshal(data, &result)
}

// ListDocsByFolder returns a flat list of DocSummary for a given folder name.
// It searches the top-level folders and their subfolders from ListAllDocuments.
func (c *Client) ListDocsByFolder(folderName string) ([]DocSummary, error) {
	folders, err := c.ListAllDocuments()
	if err != nil {
		return nil, err
	}
	var result []DocSummary
	var collect func(folders []DocFolder)
	collect = func(folders []DocFolder) {
		for _, f := range folders {
			if f.Name == folderName {
				result = append(result, f.Files...)
				// Also collect from subfolders
				var collectAll func(subs []DocFolder)
				collectAll = func(subs []DocFolder) {
					for _, sub := range subs {
						result = append(result, sub.Files...)
						collectAll(sub.SubFolders)
					}
				}
				collectAll(f.SubFolders)
				return
			}
			collect(f.SubFolders)
		}
	}
	collect(folders)
	return result, nil
}

// FlattenAllDocs returns all documents from all folders as a flat list.
func (c *Client) FlattenAllDocs() ([]DocSummary, error) {
	folders, err := c.ListAllDocuments()
	if err != nil {
		return nil, err
	}
	var result []DocSummary
	var collect func(folders []DocFolder)
	collect = func(folders []DocFolder) {
		for _, f := range folders {
			result = append(result, f.Files...)
			collect(f.SubFolders)
		}
	}
	collect(folders)
	return result, nil
}

// DocumentDiff returns the diff output for a document.
func (c *Client) DocumentDiff(docID, since string) (string, error) {
	path := "/v1/documents/" + docID + "/diff"
	if since != "" {
		path += "?from=" + since
	}
	data, err := c.get(path)
	if err != nil {
		return "", err
	}
	// The API returns JSON with a "diff" field.
	var result struct {
		Diff string `json:"diff"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		// If not JSON, return raw text.
		return string(data), nil
	}
	return result.Diff, nil
}

// --- Assets ---

func (c *Client) ListAssets() ([]db.Asset, error) {
	data, err := c.get("/v1/assets")
	if err != nil {
		return nil, err
	}
	var result []db.Asset
	return result, unwrapList(data, &result)
}

func (c *Client) AddAsset(asset *db.Asset) (*db.Asset, error) {
	data, err := c.post("/v1/assets", asset)
	if err != nil {
		return nil, err
	}
	var result db.Asset
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateAsset(id string, asset *db.Asset) (*db.Asset, error) {
	data, err := c.put("/v1/assets/"+id, asset)
	if err != nil {
		return nil, err
	}
	var result db.Asset
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) DeleteAsset(id string) error {
	_, err := c.delete("/v1/assets/" + id)
	return err
}

// --- Risks ---

func (c *Client) ListRisks() ([]db.Risk, error) {
	data, err := c.get("/v1/risks")
	if err != nil {
		return nil, err
	}
	var result []db.Risk
	return result, unwrapList(data, &result)
}

// Reference is an entity relation to create alongside a new entity. It mirrors
// the API's "references" field ({type, id}); the create handlers turn each into
// a bidirectional entity reference.
type Reference struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func (c *Client) AddRisk(risk *db.Risk, refs []Reference) (*db.Risk, error) {
	body := struct {
		*db.Risk
		References []Reference `json:"references,omitempty"`
	}{risk, refs}
	data, err := c.post("/v1/risks", body)
	if err != nil {
		return nil, err
	}
	var result db.Risk
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateRisk(id string, risk *db.Risk) (*db.Risk, error) {
	data, err := c.put("/v1/risks/"+id, risk)
	if err != nil {
		return nil, err
	}
	var result db.Risk
	return &result, json.Unmarshal(data, &result)
}

// --- Overdue ---

func (c *Client) GetOverdueSummary() (*db.OverdueSummary, error) {
	data, err := c.get("/v1/overdue")
	if err != nil {
		return nil, err
	}
	var result db.OverdueSummary
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) CreateOverdueTasks() (*db.CreatedReviewTasks, error) {
	data, err := c.post("/v1/overdue/tasks", nil)
	if err != nil {
		return nil, err
	}
	var result db.CreatedReviewTasks
	return &result, json.Unmarshal(data, &result)
}

// --- Suppliers ---

func (c *Client) ListSuppliers() ([]db.Supplier, error) {
	data, err := c.get("/v1/suppliers?limit=200")
	if err != nil {
		return nil, err
	}
	var result []db.Supplier
	return result, unwrapList(data, &result)
}

func (c *Client) AddSupplier(supplier *db.Supplier) (*db.Supplier, error) {
	data, err := c.post("/v1/suppliers", supplier)
	if err != nil {
		return nil, err
	}
	var result db.Supplier
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateSupplier(id string, supplier *db.Supplier) (*db.Supplier, error) {
	data, err := c.put("/v1/suppliers/"+id, supplier)
	if err != nil {
		return nil, err
	}
	var result db.Supplier
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) DeleteSupplier(id string) error {
	_, err := c.delete("/v1/suppliers/" + id)
	return err
}

// --- Systems ---

func (c *Client) ListSystems() ([]db.System, error) {
	data, err := c.get("/v1/systems")
	if err != nil {
		return nil, err
	}
	var result []db.System
	return result, unwrapList(data, &result)
}

func (c *Client) CreateSystem(system *db.System) (*db.System, error) {
	data, err := c.post("/v1/systems", system)
	if err != nil {
		return nil, err
	}
	var result db.System
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateSystem(id string, system *db.System) (*db.System, error) {
	data, err := c.put("/v1/systems/"+id, system)
	if err != nil {
		return nil, err
	}
	var result db.System
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) DeleteSystem(id string) error {
	_, err := c.delete("/v1/systems/" + id)
	return err
}

// --- Access Reviews ---

func (c *Client) ListAccessReviews(systemID string) ([]db.AccessReview, error) {
	data, err := c.get("/v1/systems/" + systemID + "/access-reviews")
	if err != nil {
		return nil, err
	}
	var result []db.AccessReview
	return result, unwrapList(data, &result)
}

func (c *Client) CreateAccessReview(systemID string, ar *db.AccessReview) (*db.AccessReview, error) {
	data, err := c.post("/v1/systems/"+systemID+"/access-reviews", ar)
	if err != nil {
		return nil, err
	}
	var result db.AccessReview
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) DeleteAccessReview(id string) error {
	_, err := c.delete("/v1/access-reviews/" + id)
	return err
}

// --- Reviews ---

func (c *Client) ListReviews(status string) ([]db.Review, error) {
	path := "/v1/reviews"
	if status != "" {
		path += "?status=" + status
	}
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result []db.Review
	return result, unwrapList(data, &result)
}

func (c *Client) GetReview(id int) (*db.Review, error) {
	data, err := c.get("/v1/reviews/" + strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var result db.Review
	return &result, json.Unmarshal(data, &result)
}

type ReviewSendRequest struct {
	Reviewers []string `json:"reviewers"`
	Message   string   `json:"message,omitempty"`
}

type ReviewSendResult struct {
	ReviewID int    `json:"review_id"`
	Version  string `json:"version"`
}

func (c *Client) SendReview(docID string, req *ReviewSendRequest) (*ReviewSendResult, error) {
	data, err := c.post("/v1/reviews/"+docID+"/send", req)
	if err != nil {
		return nil, err
	}
	var result ReviewSendResult
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateReviewStatus(id int, status string) error {
	_, err := c.put("/v1/reviews/"+strconv.Itoa(id)+"/status", map[string]string{"status": status})
	return err
}

func (c *Client) ForwardReview(id int, reviewers []string, message string) error {
	_, err := c.post("/v1/reviews/"+strconv.Itoa(id)+"/forward", map[string]interface{}{
		"reviewers": reviewers,
		"message":   message,
	})
	return err
}

// --- Comments ---

func (c *Client) OpenComments() ([]db.Comment, error) {
	data, err := c.get("/v1/comments/open")
	if err != nil {
		return nil, err
	}
	var result []db.Comment
	return result, unwrapList(data, &result)
}

func (c *Client) ResolveComment(id int) error {
	_, err := c.post("/v1/comments/"+strconv.Itoa(id)+"/resolve", nil)
	return err
}

// --- Approvals ---

// ApproveReview runs a review through the dedicated approval handler
// (POST /reviews/:id/approve), which records the decision log, content hash and
// status transition atomically. decision is "approved" or "changes_requested".
func (c *Client) ApproveReview(id int, decision, comment string) error {
	_, err := c.post(fmt.Sprintf("/v1/reviews/%d/approve", id), map[string]string{
		"decision": decision,
		"comment":  comment,
	})
	return err
}

// --- Tasks ---

func (c *Client) ListTasks(assignee, status string) ([]db.Task, error) {
	params := []string{}
	if assignee != "" {
		params = append(params, "assignee="+assignee)
	}
	if status != "" {
		params = append(params, "status="+status)
	}
	path := "/v1/tasks"
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result []db.Task
	return result, unwrapList(data, &result)
}

func (c *Client) CreateTask(task *db.Task) (*db.Task, error) {
	data, err := c.post("/v1/tasks", task)
	if err != nil {
		return nil, err
	}
	var result db.Task
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) GetTask(id int) (*db.Task, error) {
	data, err := c.get("/v1/tasks/" + strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var result db.Task
	return &result, json.Unmarshal(data, &result)
}

// --- Changes ---

// ListChanges returns the change requests (up to a high limit) and the server's
// total count, so callers can report truncation. The endpoint is paginated
// (default 50) and wraps results in {data,total,...}.
func (c *Client) ListChanges(status string) ([]db.ChangeRequest, int, error) {
	path := "/v1/changes?limit=1000"
	if status != "" {
		path += "&status=" + status
	}
	data, err := c.get(path)
	if err != nil {
		return nil, 0, err
	}
	var wrapper struct {
		Data  []db.ChangeRequest `json:"data"`
		Total int                `json:"total"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, 0, err
	}
	return wrapper.Data, wrapper.Total, nil
}

func (c *Client) GetChange(id int) (*db.ChangeRequest, error) {
	data, err := c.get("/v1/changes/" + strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var result db.ChangeRequest
	return &result, json.Unmarshal(data, &result)
}

// CreateChange posts a change request. Passing *db.ChangeRequest (the file's
// convention) lets the server bind everything it accepts — incl. planned_at —
// and apply its own defaults (status defaults to "proposed"; RequestedBy is
// server-stamped from the token).
func (c *Client) CreateChange(cr *db.ChangeRequest) (*db.ChangeRequest, error) {
	data, err := c.post("/v1/changes", cr)
	if err != nil {
		return nil, err
	}
	var result db.ChangeRequest
	return &result, json.Unmarshal(data, &result)
}

// UpdateChange PUTs only the changed fields. The server's update contract is
// nil-means-leave-alone, so callers build the map from cmd.Flags().Changed to
// avoid clobbering unspecified fields with zero values.
func (c *Client) UpdateChange(id int, fields map[string]interface{}) (*db.ChangeRequest, error) {
	data, err := c.put("/v1/changes/"+strconv.Itoa(id), fields)
	if err != nil {
		return nil, err
	}
	var result db.ChangeRequest
	return &result, json.Unmarshal(data, &result)
}

// UpdateChangeStatus transitions status. The endpoint returns only {"status"},
// not a full entity, so this returns the new status string.
func (c *Client) UpdateChangeStatus(id int, status string) (string, error) {
	data, err := c.put("/v1/changes/"+strconv.Itoa(id)+"/status", map[string]string{"status": status})
	if err != nil {
		return "", err
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	return result.Status, nil
}

// --- Document Comments ---

func (c *Client) CommentsForDocument(docID string) ([]db.Comment, error) {
	data, err := c.get("/v1/documents/" + docID + "/comments")
	if err != nil {
		return nil, err
	}
	var result []db.Comment
	return result, unwrapList(data, &result)
}

// --- Review Assignments ---

func (c *Client) ListAssignmentsForReview(reviewID int) ([]db.ReviewAssignment, error) {
	data, err := c.get("/v1/reviews/" + strconv.Itoa(reviewID) + "/assignments")
	if err != nil {
		return nil, err
	}
	var result []db.ReviewAssignment
	return result, unwrapList(data, &result)
}

// --- Inbox ---

type InboxItem struct {
	Type       string `json:"type"` // review, comment, task
	ID         int    `json:"id"`
	DocumentID string `json:"document_id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	From       string `json:"from"`
	CreatedAt  string `json:"created_at"`
}

func (c *Client) InboxList() ([]InboxItem, error) {
	data, err := c.get("/v1/inbox")
	if err != nil {
		return nil, err
	}
	var result []InboxItem
	return result, unwrapList(data, &result)
}

func (c *Client) InboxDump() (json.RawMessage, error) {
	data, err := c.get("/v1/inbox/dump")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// --- Audit ---

func (c *Client) ListAuditProgrammes() ([]db.AuditProgramme, error) {
	data, err := c.get("/v1/audit/programmes")
	if err != nil {
		return nil, err
	}
	var result []db.AuditProgramme
	return result, unwrapList(data, &result)
}

func (c *Client) CreateAuditProgramme(prog *db.AuditProgramme) (*db.AuditProgramme, error) {
	data, err := c.post("/v1/audit/programmes", prog)
	if err != nil {
		return nil, err
	}
	var result db.AuditProgramme
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) ListAudits(programmeID int) ([]db.Audit, error) {
	path := "/v1/audits"
	if programmeID > 0 {
		path += "?programme_id=" + strconv.Itoa(programmeID)
	}
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result []db.Audit
	return result, unwrapList(data, &result)
}

func (c *Client) CreateAudit(audit *db.Audit) (*db.Audit, error) {
	data, err := c.post("/v1/audits", audit)
	if err != nil {
		return nil, err
	}
	var result db.Audit
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) GetAudit(id int) (*db.Audit, error) {
	data, err := c.get("/v1/audits/" + strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var result db.Audit
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateAudit(id int, fields map[string]interface{}) (*db.Audit, error) {
	data, err := c.put("/v1/audits/"+strconv.Itoa(id), fields)
	if err != nil {
		return nil, err
	}
	var result db.Audit
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateAuditStatus(id int, status string) error {
	_, err := c.put("/v1/audits/"+strconv.Itoa(id)+"/status", map[string]string{"status": status})
	return err
}

func (c *Client) ListAuditItems(auditID int) ([]db.AuditItem, error) {
	data, err := c.get("/v1/audits/" + strconv.Itoa(auditID) + "/items")
	if err != nil {
		return nil, err
	}
	var result []db.AuditItem
	return result, unwrapList(data, &result)
}

func (c *Client) AssessAuditItem(id int, result string, evidence string, notes string) error {
	_, err := c.put("/v1/audit-items/"+strconv.Itoa(id), map[string]string{
		"result":   result,
		"evidence": evidence,
		"notes":    notes,
	})
	return err
}

func (c *Client) ListAuditFindings(auditID int) ([]db.AuditFinding, error) {
	data, err := c.get("/v1/audits/" + strconv.Itoa(auditID) + "/findings")
	if err != nil {
		return nil, err
	}
	var result []db.AuditFinding
	return result, unwrapList(data, &result)
}

func (c *Client) AddAuditFinding(finding *db.AuditFinding) (*db.AuditFinding, error) {
	data, err := c.post("/v1/audit-findings", finding)
	if err != nil {
		return nil, err
	}
	var result db.AuditFinding
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) GetAuditFinding(id int) (*db.AuditFinding, error) {
	data, err := c.get("/v1/audit-findings/" + strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var result db.AuditFinding
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateAuditFinding(id int, fields map[string]interface{}) (*db.AuditFinding, error) {
	data, err := c.put("/v1/audit-findings/"+strconv.Itoa(id), fields)
	if err != nil {
		return nil, err
	}
	var result db.AuditFinding
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) DeleteAuditFinding(id int) error {
	_, err := c.delete("/v1/audit-findings/" + strconv.Itoa(id))
	return err
}

// ListAuditFindingsPaginated calls GET /v1/audit/findings with the given query
// params and returns the items slice plus the total count from the wrapped
// response ({data, total, page, page_size}).
func (c *Client) ListAuditFindingsPaginated(params map[string]string) ([]db.AuditFinding, int, error) {
	path := "/v1/audit/findings"
	if len(params) > 0 {
		parts := []string{}
		for k, v := range params {
			if v == "" {
				continue
			}
			parts = append(parts, k+"="+v)
		}
		if len(parts) > 0 {
			path += "?" + strings.Join(parts, "&")
		}
	}
	data, err := c.get(path)
	if err != nil {
		return nil, 0, err
	}
	var wrapper struct {
		Data  []db.AuditFinding `json:"data"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, 0, err
	}
	return wrapper.Data, wrapper.Total, nil
}

func (c *Client) GetAuditProgramme(id int) (*db.AuditProgramme, error) {
	data, err := c.get("/v1/audit/programmes/" + strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var result db.AuditProgramme
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateAuditProgramme(id int, fields map[string]interface{}) (*db.AuditProgramme, error) {
	data, err := c.put("/v1/audit/programmes/"+strconv.Itoa(id), fields)
	if err != nil {
		return nil, err
	}
	var result db.AuditProgramme
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) DeleteAuditProgramme(id int) error {
	_, err := c.delete("/v1/audit/programmes/" + strconv.Itoa(id))
	return err
}

// --- Legal Register ---

func (c *Client) ListLegal(status string) ([]db.LegalRequirement, error) {
	path := "/v1/legal"
	if status != "" {
		path += "?status=" + status
	}
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result []db.LegalRequirement
	return result, unwrapList(data, &result)
}

func (c *Client) CreateLegal(lr *db.LegalRequirement, refs []Reference) (*db.LegalRequirement, error) {
	body := struct {
		*db.LegalRequirement
		References []Reference `json:"references,omitempty"`
	}{lr, refs}
	data, err := c.post("/v1/legal", body)
	if err != nil {
		return nil, err
	}
	var result db.LegalRequirement
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) GetLegal(id int) (*db.LegalRequirement, error) {
	data, err := c.get("/v1/legal/" + strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var result db.LegalRequirement
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateLegal(id int, lr *db.LegalRequirement) error {
	_, err := c.put("/v1/legal/"+strconv.Itoa(id), lr)
	return err
}

func (c *Client) DeleteLegal(id int) error {
	_, err := c.delete("/v1/legal/" + strconv.Itoa(id))
	return err
}

// --- Incidents ---

func (c *Client) ListIncidents(status, severity string) ([]db.Incident, error) {
	params := []string{}
	if status != "" {
		params = append(params, "status="+status)
	}
	if severity != "" {
		params = append(params, "severity="+severity)
	}
	path := "/v1/incidents"
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result []db.Incident
	return result, unwrapList(data, &result)
}

func (c *Client) CreateIncident(inc *db.Incident, refs []Reference) (*db.Incident, error) {
	body := struct {
		*db.Incident
		References []Reference `json:"references,omitempty"`
	}{inc, refs}
	data, err := c.post("/v1/incidents", body)
	if err != nil {
		return nil, err
	}
	var result db.Incident
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) GetIncident(id int) (*db.Incident, error) {
	data, err := c.get("/v1/incidents/" + strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var result db.Incident
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateIncident(id int, inc *db.Incident) error {
	_, err := c.put("/v1/incidents/"+strconv.Itoa(id), inc)
	return err
}

func (c *Client) UpdateIncidentStatus(id int, status string) error {
	_, err := c.put("/v1/incidents/"+strconv.Itoa(id)+"/status", map[string]string{"status": status})
	return err
}

// --- Corrective Actions ---

func (c *Client) ListCorrectiveActions(status, severity, assignee string) ([]db.CorrectiveAction, error) {
	params := []string{}
	if status != "" {
		params = append(params, "status="+status)
	}
	if severity != "" {
		params = append(params, "severity="+severity)
	}
	if assignee != "" {
		params = append(params, "assignee="+assignee)
	}
	path := "/v1/corrective-actions"
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result []db.CorrectiveAction
	return result, unwrapList(data, &result)
}

func (c *Client) CreateCorrectiveAction(ca *db.CorrectiveAction) (*db.CorrectiveAction, error) {
	data, err := c.post("/v1/corrective-actions", ca)
	if err != nil {
		return nil, err
	}
	var result db.CorrectiveAction
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) GetCorrectiveAction(id int) (*db.CorrectiveAction, error) {
	data, err := c.get("/v1/corrective-actions/" + strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var result db.CorrectiveAction
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateCorrectiveAction(id int, ca *db.CorrectiveAction) error {
	_, err := c.put("/v1/corrective-actions/"+strconv.Itoa(id), ca)
	return err
}

func (c *Client) UpdateCorrectiveActionStatus(id int, status string) error {
	_, err := c.put("/v1/corrective-actions/"+strconv.Itoa(id)+"/status", map[string]string{"status": status})
	return err
}

func (c *Client) DeleteCorrectiveAction(id int) error {
	_, err := c.delete("/v1/corrective-actions/" + strconv.Itoa(id))
	return err
}

// --- Programs ---

func (c *Client) ListPrograms() ([]db.Program, error) {
	data, err := c.get("/v1/programs")
	if err != nil {
		return nil, err
	}
	var result []db.Program
	return result, unwrapList(data, &result)
}

func (c *Client) CreateProgram(p *db.Program) (*db.Program, error) {
	data, err := c.post("/v1/programs", p)
	if err != nil {
		return nil, err
	}
	var result db.Program
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) GetProgram(id int64) (*db.Program, error) {
	data, err := c.get("/v1/programs/" + strconv.FormatInt(id, 10))
	if err != nil {
		return nil, err
	}
	var result db.Program
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateProgram(id int64, p *db.Program) (*db.Program, error) {
	data, err := c.put("/v1/programs/"+strconv.FormatInt(id, 10), p)
	if err != nil {
		return nil, err
	}
	var result db.Program
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) DeleteProgram(id int64) error {
	_, err := c.delete("/v1/programs/" + strconv.FormatInt(id, 10))
	return err
}

// --- Objectives ---

func (c *Client) ListObjectives(programID int64, status string) ([]db.Objective, error) {
	params := []string{}
	if programID > 0 {
		params = append(params, "program_id="+strconv.FormatInt(programID, 10))
	}
	if status != "" {
		params = append(params, "status="+status)
	}
	path := "/v1/objectives"
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result []db.Objective
	return result, unwrapList(data, &result)
}

func (c *Client) CreateObjective(o *db.Objective) (*db.Objective, error) {
	data, err := c.post("/v1/objectives", o)
	if err != nil {
		return nil, err
	}
	var result db.Objective
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) GetObjective(id int64) (*db.Objective, error) {
	data, err := c.get("/v1/objectives/" + strconv.FormatInt(id, 10))
	if err != nil {
		return nil, err
	}
	var result db.Objective
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) UpdateObjective(id int64, o *db.Objective) (*db.Objective, error) {
	data, err := c.put("/v1/objectives/"+strconv.FormatInt(id, 10), o)
	if err != nil {
		return nil, err
	}
	var result db.Objective
	return &result, json.Unmarshal(data, &result)
}

func (c *Client) ArchiveObjective(id int64) error {
	_, err := c.post("/v1/objectives/"+strconv.FormatInt(id, 10)+"/archive", nil)
	return err
}

func (c *Client) UnarchiveObjective(id int64) error {
	_, err := c.post("/v1/objectives/"+strconv.FormatInt(id, 10)+"/unarchive", nil)
	return err
}

// --- Checkins ---

func (c *Client) ListCheckins(objectiveID int64, limit int) ([]db.Checkin, error) {
	path := "/v1/objectives/" + strconv.FormatInt(objectiveID, 10) + "/checkins"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result []db.Checkin
	return result, unwrapList(data, &result)
}

func (c *Client) CreateCheckin(objectiveID int64, ci *db.Checkin) (*db.Checkin, error) {
	data, err := c.post("/v1/objectives/"+strconv.FormatInt(objectiveID, 10)+"/checkins", ci)
	if err != nil {
		return nil, err
	}
	var result db.Checkin
	return &result, json.Unmarshal(data, &result)
}

// --- User ---

type UserInfo struct {
	Email            string `json:"email"`
	Name             string `json:"name"`
	Role             string `json:"role"`
	OrganizationUUID string `json:"organization_uuid,omitempty"`
	OrganizationSlug string `json:"organization_slug,omitempty"`
	OrganizationName string `json:"organization_name,omitempty"`
}

// BearerToken returns the configured Bearer token (for git credential helper).
func (c *Client) BearerToken() string {
	return c.bearerToken
}

func (c *Client) WhoAmI() (*UserInfo, error) {
	data, err := c.get("/v1/me")
	if err != nil {
		return nil, err
	}
	var result UserInfo
	return &result, json.Unmarshal(data, &result)
}

// OrgInfo represents an organization the user belongs to.
type OrgInfo struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Role string `json:"role"`
}

// ListMyOrgs returns all organizations the current user belongs to.
func (c *Client) ListMyOrgs() ([]OrgInfo, error) {
	data, err := c.get("/v1/me/organizations")
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Data []OrgInfo `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		// Try direct array
		var orgs []OrgInfo
		if err2 := json.Unmarshal(data, &orgs); err2 != nil {
			return nil, err
		}
		return orgs, nil
	}
	return wrapper.Data, nil
}
