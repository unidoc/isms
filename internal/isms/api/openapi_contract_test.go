package api

// Contract test: the embedded OpenAPI spec must describe what the handlers
// really serialize. For every component listed in contractTable it builds the
// Go value the handler passes to c.JSON, fills every field by reflection,
// marshals it with encoding/json and compares the result with the schema, in
// both directions (keys) and by JSON type. A second pass marshals the zero
// value to check nullability and omitempty. See the rules below.
//
// readOnly and writeOnly follow the request DTOs listed per entry: a property
// is readOnly when no request accepts it (or the handler overwrites it), and
// writeOnly when a request accepts it but the response never emits it. A
// component that an operation takes as its request body must list its DTOs,
// and each DTO field must be able to decode the type the schema documents.
//
// Nullability rule (stated once in the comment above EpochSeconds in
// openapi.yaml; the test enforces it mechanically):
//   - a key that is omitted when empty (omitempty) is not in "required" and its
//     type does not include null;
//   - a key that marshals to null at its zero value (nil pointer, nil slice or
//     map, zero db.Epoch) has a type that includes null;
//   - every other key is never null, so its type does not include null;
//   - neverNullKeys (and neverNullByComponent) are exempt from the second
//     pass: the database always sets them (now() defaults), so their zero value
//     is an artifact of the test and the spec documents them as non-null;
//   - the exception to all of the above: a key some update request decodes
//     through Optional[T] clears the stored value when it receives null, so its
//     schema type includes null and its description says "Send null on update
//     to clear." Any other update field leaves the value alone on null, so its
//     type must not include null because of the update.
//
// Create-only keys (accepted by a create request, not by the update request)
// are silently ignored on update, so their description says so.
//
// readOnly and writeOnly are enforced only for entries that list request DTOs
// (the components that are request bodies, plus those with DTOs). A
// response-only component such as Evidence carries neither, by design.

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// contractShape is one Go value a handler passes to c.JSON for a component.
type contractShape struct {
	// typ is the Go type serialized (pointer stripped).
	typ reflect.Type
	// handler documents where the value comes from: file:line and the
	// expression passed to c.JSON. A wrong mapping would pass green against
	// the wrong shape, so keep it traced to the real handler.
	handler string
}

type contractEntry struct {
	component string
	// shapes holds one value per distinct handler shape (for example a list
	// handler that joins an extra column). Each is checked for undocumented
	// keys and wrong types; a documented key must appear in at least one shape,
	// and a key missing from some shapes must not be required.
	shapes []contractShape
	// requests are the request DTOs that accept this component's keys. A
	// component that some requestBody references MUST list them.
	requests []reflect.Type
	// serverSet lists keys the request type binds but the handler overwrites or
	// ignores (for example when the request body is the db struct itself); the
	// spec marks them readOnly.
	serverSet []string
	// fullReplace marks an update request that replaces the whole record, so
	// omitting a pointer field or sending null clears it (not just Optional
	// fields). Such fields must be nullable and say "Omit or send null to clear."
	fullReplace bool
	// requestOnly marks a component that no handler returns: it is the element
	// type of a request field. It is checked as a request shape (accepted keys
	// and types), not as a response.
	requestOnly bool
}

func shape(t reflect.Type, handler string) contractShape {
	return contractShape{typ: t, handler: handler}
}

var tReq = reflect.TypeOf

// contractTable maps each checked component to the Go values handlers
// serialize. Every handler below returns the db struct itself (never a
// wrapper, except list handlers which wrap the rows as {"data": rows, ...} in
// an inline schema outside the component), so one shape per component.
var contractTable = []contractEntry{
	{
		component: "Risk",
		// All handlers return the db.Risk itself, never a wrapper:
		//   server.go:2259 handleGetRisk:    c.JSON(200, r)     r is *db.Risk (GetRisk)
		//   server.go:2450 handleAddRisk:    c.JSON(201, out)   out is *db.Risk (GetRisk)
		//   server.go:3036 handleUpdateRisk: c.JSON(200, after) after is *db.Risk (GetRisk)
		//   server.go:2368 handleListRisks:  c.JSON(200, {"data": risks, ...}) risks is
		//     []db.Risk (PaginatedRisks); the list wrapper is an inline schema in
		//     the /risks path, not part of the Risk component.
		shapes:   []contractShape{shape(tReq(db.Risk{}), "server.go:2259 handleGetRisk: c.JSON(200, r), r is *db.Risk")},
		requests: []reflect.Type{tReq(riskCreateRequest{}), tReq(riskUpdateRequest{})},
	},
	{
		component: "ReferenceInput",
		// Not returned by any handler: element type of the "references" field
		// of the create requests (api_references.go:17).
		shapes:      []contractShape{shape(tReq(ReferenceInput{}), "api_references.go:17 ReferenceInput, element of the references field of create requests")},
		requestOnly: true,
	},
	{
		component: "Asset",
		//   server.go:2031 handleGetAsset:    c.JSON(200, a)       a is *db.Asset (GetAsset)
		//   server.go:2083 handleAddAsset:    c.JSON(201, out)     out is *db.Asset (GetAsset); 2085 falls back to a (db.Asset)
		//   server.go:2819 handleUpdateAsset: c.JSON(200, after)   after is *db.Asset (GetAsset); 2821 falls back to updated (*db.Asset)
		//   server.go:2002 handleListAssets:  {"data": assets} assets is []db.Asset (PaginatedAssets)
		shapes:   []contractShape{shape(tReq(db.Asset{}), "server.go:2031 handleGetAsset: c.JSON(200, a), a is *db.Asset")},
		requests: []reflect.Type{tReq(assetCreateRequest{}), tReq(assetUpdateRequest{})},
	},
	{
		component: "System",
		//   server.go:2171 handleGetSystem:    c.JSON(200, sys)    sys is *db.System (GetSystem)
		//   server.go:2244 handleCreateSystem: c.JSON(201, sys)    sys is db.System (re-fetched via GetSystem)
		//   server.go:3168 handleUpdateSystem: c.JSON(200, after)  after is *db.System (GetSystem); 3170 falls back to updated
		//   server.go:2142 handleListSystems:  {"data": systems} systems is []db.System (PaginatedSystems)
		shapes:   []contractShape{shape(tReq(db.System{}), "server.go:2171 handleGetSystem: c.JSON(200, sys), sys is *db.System")},
		requests: []reflect.Type{tReq(systemCreateRequest{}), tReq(systemUpdateRequest{})},
	},
	{
		component: "AccessReview",
		//   server.go:3256 handleCreateAccessReview: c.JSON(201, ar) ar is db.AccessReview (also the bound request body)
		//   server.go:3217 handleListAccessReviews:  {"data": reviews} reviews is []db.AccessReview (ListAccessReviews)
		// The request body is the db struct itself, so every key binds; the
		// handler sets system_id, CreateAccessReview sets organization_id, id
		// and created_at.
		shapes:    []contractShape{shape(tReq(db.AccessReview{}), "server.go:3256 handleCreateAccessReview: c.JSON(201, ar), ar is db.AccessReview")},
		requests:  []reflect.Type{tReq(db.AccessReview{})},
		serverSet: []string{"id", "organization_id", "system_id", "created_at"},
	},
	{
		component: "Supplier",
		//   server.go:2654 handleGetSupplier:    c.JSON(200, sup)    sup is *db.Supplier (GetSupplier)
		//   server.go:2712 handleAddSupplier:    c.JSON(201, sup)    sup is db.Supplier (re-fetched via GetSupplier)
		//   server.go:3396 handleUpdateSupplier: c.JSON(200, after)  after is *db.Supplier (GetSupplier); 3398 falls back to updated
		//   server.go:2625 handleListSuppliers:  {"data": suppliers} suppliers is []db.Supplier (PaginatedSuppliers)
		shapes:   []contractShape{shape(tReq(db.Supplier{}), "server.go:2654 handleGetSupplier: c.JSON(200, sup), sup is *db.Supplier")},
		requests: []reflect.Type{tReq(supplierCreateRequest{}), tReq(supplierUpdateRequest{})},
	},
	{
		component: "Task",
		//   api_collab.go:2599 handleGetTask:    c.JSON(200, task) task is *db.Task (GetTask)
		//   api_collab.go:2439 handleCreateTask: c.JSON(201, t)    t is db.Task
		//   api_collab.go:2720 handleUpdateTask: c.JSON(200, updated) updated is *db.Task (GetTask); 2722 falls back to t from prepareTaskUpdate
		//   api_collab.go:2581 handleUpdateTaskNotes: c.JSON(200, after) after is *db.Task
		//   api_collab.go:2356 handleListTasks:  {"data": tasks} tasks is []db.Task (PaginatedTasks); inbox_group and needs_action are filled only here
		shapes:   []contractShape{shape(tReq(db.Task{}), "api_collab.go:2599 handleGetTask: c.JSON(200, task), task is *db.Task")},
		requests: []reflect.Type{tReq(taskCreateRequest{}), tReq(taskUpdateRequest{})},
	},
	{
		component: "ChangeRequest",
		//   api_collab.go:2896 handleGetChange:    c.JSON(200, cr)      cr is *db.ChangeRequest (GetChangeRequest)
		//   api_collab.go:2968 handleCreateChange: c.JSON(201, cr)      cr is db.ChangeRequest (re-fetched)
		//   api_collab.go:3166 handleUpdateChange: c.JSON(200, updated) updated is *db.ChangeRequest
		//   api_collab.go:2867 handleListChanges:  {"data": changes} changes is []db.ChangeRequest (PaginatedChangeRequests); needs_action filled only here
		shapes:   []contractShape{shape(tReq(db.ChangeRequest{}), "api_collab.go:2896 handleGetChange: c.JSON(200, cr), cr is *db.ChangeRequest")},
		requests: []reflect.Type{tReq(changeCreateRequest{}), tReq(changeUpdateRequest{})},
	},
	{
		component: "AuditProgramme",
		//   api_audit.go:165 handleGetAuditProgramme:    c.JSON(200, p) p is *db.AuditProgramme (GetAuditProgramme)
		//   api_audit.go:152 handleCreateAuditProgramme: c.JSON(201, p) p is db.AuditProgramme
		//   api_audit.go:206 handleUpdateAuditProgramme: c.JSON(200, p) p is *db.AuditProgramme (GetAuditProgramme)
		//   api_audit.go:104 handleListAuditProgrammes:  {"data": programmes} programmes is []db.AuditProgramme (ListAuditProgrammes)
		shapes:   []contractShape{shape(tReq(db.AuditProgramme{}), "api_audit.go:165 handleGetAuditProgramme: c.JSON(200, p), p is *db.AuditProgramme")},
		requests: []reflect.Type{tReq(auditProgrammeCreateRequest{}), tReq(auditProgrammeUpdateRequest{})},
	},
	{
		component: "Audit",
		//   api_audit.go:347 handleGetAudit:    c.JSON(200, audit) audit is *db.Audit (GetAudit)
		//   api_audit.go:334 handleCreateAudit: c.JSON(201, out)   out is *db.Audit (GetAudit)
		//   api_audit.go:403 handleUpdateAudit: c.JSON(200, after) after is *db.Audit (GetAudit)
		//   api_audit.go:263 handleListAudits:  {"data": audits} audits is []db.Audit (ListAudits)
		shapes:   []contractShape{shape(tReq(db.Audit{}), "api_audit.go:347 handleGetAudit: c.JSON(200, audit), audit is *db.Audit")},
		requests: []reflect.Type{tReq(auditCreateRequest{}), tReq(auditUpdateRequest{})},
	},
	{
		component: "AuditItem",
		//   api_audit.go:559 handleCreateAuditItem: c.JSON(201, item) item is *db.AuditItem
		//   api_audit.go:607 handleUpdateAuditItem: c.JSON(200, out)  out is *db.AuditItem (GetAuditItem)
		//   api_audit.go:517 handleListAuditItems:  {"data": items} items is []db.AuditItem (ListAuditItems)
		shapes:   []contractShape{shape(tReq(db.AuditItem{}), "api_audit.go:607 handleUpdateAuditItem: c.JSON(200, out), out is *db.AuditItem")},
		requests: []reflect.Type{tReq(auditItemCreateRequest{}), tReq(auditItemUpdateRequest{})},
	},
	{
		component: "AuditFinding",
		//   api_audit.go:689 handleGetAuditFinding:    c.JSON(200, f)     f is *db.AuditFinding (GetAuditFinding)
		//   api_audit.go:769 handleAddAuditFinding:    c.JSON(201, out)   out is *db.AuditFinding (GetAuditFinding)
		//   api_audit.go:845 handleUpdateAuditFinding: c.JSON(200, after) after is *db.AuditFinding
		//   api_audit.go:627 handleListAuditFindingsForAudit: {"data": findings} findings is []db.AuditFinding (ListAuditFindings)
		//   api_audit.go:667 handlePaginatedAuditFindings: {"data": ..., ...} rows are []db.AuditFinding (PaginatedAuditFindings)
		shapes:   []contractShape{shape(tReq(db.AuditFinding{}), "api_audit.go:689 handleGetAuditFinding: c.JSON(200, f), f is *db.AuditFinding")},
		requests: []reflect.Type{tReq(auditFindingCreateRequest{}), tReq(auditFindingUpdateRequest{})},
	},
	{
		component: "LegalRequirement",
		//   api_legal.go:183 handleGetLegal:    c.JSON(200, lr)    lr is *db.LegalRequirement (GetLegalRequirement)
		//   api_legal.go:168 handleCreateLegal: c.JSON(201, lr)    lr is db.LegalRequirement
		//   api_legal.go:341 handleUpdateLegal: c.JSON(200, after) after is *db.LegalRequirement (GetLegalRequirementTx)
		//   api_legal.go:93  handleListLegal:   {"data": ...} rows are []db.LegalRequirement (PaginatedLegalRequirements)
		shapes:   []contractShape{shape(tReq(db.LegalRequirement{}), "api_legal.go:183 handleGetLegal: c.JSON(200, lr), lr is *db.LegalRequirement")},
		requests: []reflect.Type{tReq(legalCreateRequest{}), tReq(legalUpdateRequest{})},
	},
	{
		component: "Incident",
		//   api_incidents.go:222 handleGetIncident:    c.JSON(200, inc)    inc is *db.Incident (GetIncident)
		//   api_incidents.go:207 handleCreateIncident: c.JSON(201, inc)    inc is db.Incident (re-fetched)
		//   api_incidents.go:406 handleUpdateIncident: c.JSON(200, after)  after is *db.Incident (GetIncidentTx)
		//   api_incidents.go:660 handleUpdateIncidentProgress: c.JSON(200, after) after is *db.Incident
		//   api_incidents.go:99  handleListIncidents:  {"data": ...} rows are []db.Incident (PaginatedIncidents)
		shapes:   []contractShape{shape(tReq(db.Incident{}), "api_incidents.go:222 handleGetIncident: c.JSON(200, inc), inc is *db.Incident")},
		requests: []reflect.Type{tReq(incidentCreateRequest{}), tReq(incidentUpdateRequest{})},
	},
	{
		component: "CorrectiveAction",
		//   api_corrective.go:187 handleGetCorrectiveAction:    c.JSON(200, ca)    ca is *db.CorrectiveAction (GetCorrectiveAction)
		//   api_corrective.go:172 handleCreateCorrectiveAction: c.JSON(201, ca)    ca is db.CorrectiveAction (re-fetched)
		//   api_corrective.go:320 handleUpdateCorrectiveAction: c.JSON(200, after) after is *db.CorrectiveAction (GetCorrectiveActionTx)
		//   api_corrective.go:505 handleUpdateCorrectiveActionProgress: c.JSON(200, after) after is *db.CorrectiveAction
		//   api_corrective.go:75  handleListCorrectiveActions: {"data": ...} rows are []db.CorrectiveAction (PaginatedCorrectiveActions)
		shapes:   []contractShape{shape(tReq(db.CorrectiveAction{}), "api_corrective.go:187 handleGetCorrectiveAction: c.JSON(200, ca), ca is *db.CorrectiveAction")},
		requests: []reflect.Type{tReq(correctiveActionCreateRequest{}), tReq(correctiveActionUpdateRequest{})},
	},
	{
		component: "Program",
		//   api_objectives.go:197 handleGetProgram:    c.JSON(200, p)     p is *db.Program (GetProgram)
		//   api_objectives.go:169 handleCreateProgram: c.JSON(201, p)     p is db.Program
		//   api_objectives.go:256 handleUpdateProgram: c.JSON(200, after) after is *db.Program
		//   api_objectives.go:102 handleListPrograms:  {"data": programs} programs is []db.Program (ListPrograms)
		shapes:   []contractShape{shape(tReq(db.Program{}), "api_objectives.go:197 handleGetProgram: c.JSON(200, p), p is *db.Program")},
		requests: []reflect.Type{tReq(programCreateRequest{}), tReq(programUpdateRequest{})},
	},
	{
		component: "Objective",
		//   api_objectives.go:453 handleGetObjective:    c.JSON(200, o)     o is *db.Objective (GetObjective)
		//   api_objectives.go:438 handleCreateObjective: c.JSON(201, o)     o is db.Objective
		//   api_objectives.go:567 handleUpdateObjective: c.JSON(200, after) after is *db.Objective
		//   api_objectives.go:348 handleListObjectives:  {"data": ...} rows are []db.Objective (PaginatedObjectives)
		shapes:   []contractShape{shape(tReq(db.Objective{}), "api_objectives.go:453 handleGetObjective: c.JSON(200, o), o is *db.Objective")},
		requests: []reflect.Type{tReq(objectiveCreateRequest{}), tReq(objectiveUpdateRequest{})},
	},
	{
		component: "Checkin",
		//   api_objectives.go:802 handleCreateCheckin: c.JSON(201, ci)    ci is db.Checkin
		//   api_objectives.go:868 handleUpdateCheckin: c.JSON(200, after) after is *db.Checkin
		//   api_objectives.go:731 handleListCheckins:  {"data": checkins} checkins is []db.Checkin (ListCheckins)
		shapes:   []contractShape{shape(tReq(db.Checkin{}), "api_objectives.go:802 handleCreateCheckin: c.JSON(201, ci), ci is db.Checkin")},
		requests: []reflect.Type{tReq(checkinCreateRequest{}), tReq(checkinUpdateRequest{})},
	},
	{
		component: "Evidence",
		//   api_objectives.go:982 handleUploadEvidence: c.JSON(201, ev) ev is db.Evidence (multipart upload, no JSON request DTO)
		//   api_objectives.go:996 handleListEvidence:   {"data": evidence} evidence is []db.Evidence (ListEvidence)
		shapes: []contractShape{shape(tReq(db.Evidence{}), "api_objectives.go:982 handleUploadEvidence: c.JSON(201, ev), ev is db.Evidence")},
	},
	{
		component: "APIKey",
		//   api_auth.go:1101 handleListMyAPIKeys: {"data": keys} keys is []db.APIKey (ListUserAPIKeys)
		//   api_admin.go:156 handleAdminListAPIKeys: {"data": keys} keys is []db.APIKey (ListAllAPIKeysForOrg)
		// db.APIKey has no token or hash field, so neither is ever serialized by
		// a list; the token is returned only by the create response (CreatedAPIKey).
		shapes: []contractShape{shape(tReq(db.APIKey{}), "api_auth.go:1101 handleListMyAPIKeys: element of the data array, db.APIKey")},
	},
	{
		component: "CreatedAPIKey",
		// api_auth.go:1145 handleCreateMyAPIKey: c.JSON(201, map[string]interface{}{token, id, name,
		// permissions, organization_id}); mirrors a map, all keys always present.
		shapes: []contractShape{shape(tReq(createdAPIKeyMirror{}), "api_auth.go:1145 handleCreateMyAPIKey: map literal mirrored by createdAPIKeyMirror")},
	},
	{
		component: "Approval",
		//   api_collab.go:2289 handleListApprovals: {"data": approvals} approvals is []db.Approval (ApprovalsForDocument)
		shapes: []contractShape{shape(tReq(db.Approval{}), "api_collab.go:2289 handleListApprovals: element of the data array, db.Approval")},
	},
	{
		component: "ChangelogEntry",
		//   api_changelog.go:30 handleListChangelog: {"data": entries} entries is []db.ChangelogEntry (ListChangelog)
		//   api_changelog.go:59 handleEntityChangelog: {"data": entries} entries is []db.ChangelogEntry (ListEntityChangelog)
		//   api_changelog.go:~80 handleDocumentHistory (GET /changelog/document/{id}): git history and
		//     decision-log rows mapped onto db.ChangelogEntry (mergeDocumentHistory)
		shapes: []contractShape{shape(tReq(db.ChangelogEntry{}), "api_changelog.go:30 handleListChangelog: element of the data array, db.ChangelogEntry")},
	},
	{
		component: "Comment",
		//   api_collab.go:1900 handleAllOpenComments: {"data": comments} comments is []db.Comment
		//   api_collab.go:1916 handleListCommentsDB:  {"data": comments} comments is []db.Comment (CommentsForDocument)
		//   api_collab.go:2004 handleAddCommentDB:    c.JSON(201, comment) comment is db.Comment
		shapes: []contractShape{shape(tReq(db.Comment{}), "api_collab.go:2004 handleAddCommentDB: c.JSON(201, comment), comment is db.Comment")},
	},
	{
		component: "Document",
		// server.go:1262 handleGetDocument: c.JSON(200, resp) where resp is a
		// map[string]interface{} built at server.go:1232-1244, with the active_review_* keys
		// added at server.go:1248-1262 only when the document is in_review and has an open
		// review. documentMirror mirrors the map; the conditional keys are pointers with omitempty.
		shapes: []contractShape{shape(tReq(documentMirror{}), "server.go:1262 handleGetDocument: map mirrored by documentMirror")},
	},
	{
		component: "DocumentFolder",
		// server.go:1187 handleListAllDocuments: {"data": folders} where folders is []DocFolder, a
		// type declared inside the handler (server.go:1105); docFolderMirror mirrors it.
		shapes: []contractShape{shape(tReq(docFolderMirror{}), "server.go:1187 handleListAllDocuments: element of the data array, DocFolder mirrored by docFolderMirror")},
	},
	{
		component: "DocumentFile",
		// server.go:1096 DocFile, the element of DocFolder.files (declared inside handleListAllDocuments).
		shapes: []contractShape{shape(tReq(docFileMirror{}), "server.go:1158 handleListAllDocuments: DocFile element of files, mirrored by docFileMirror")},
	},
	{
		component: "DocumentNeedingReview",
		// server.go:1975 handleNeedsReview: {"data": results} where results is []NeedsReviewDoc, a type
		// declared inside the handler (server.go:1841); needsReviewDocMirror mirrors it.
		shapes: []contractShape{shape(tReq(needsReviewDocMirror{}), "server.go:1975 handleNeedsReview: element of the data array, NeedsReviewDoc mirrored by needsReviewDocMirror")},
	},
	{
		component: "ChangedDocument",
		// server.go:1313 handleChangedDocuments: {"data": changed} where changed is []store.ChangedFile.
		shapes: []contractShape{shape(tReq(store.ChangedFile{}), "server.go:1313 handleChangedDocuments: element of the data array, store.ChangedFile")},
	},
	{
		component: "DocumentVersion",
		//   api_diff.go:28 handleListVersions: {"data": versions} versions is []db.DocumentVersion (ListVersions)
		shapes: []contractShape{shape(tReq(db.DocumentVersion{}), "api_diff.go:28 handleListVersions: element of the data array, db.DocumentVersion")},
	},
	{
		component: "EntityReference",
		//   api_references.go:222 handleCreateReference: c.JSON(201, fwd) fwd is *db.EntityReference
		//   api_references.go:155 handleListReferences: {"data": result} result is []refWithTitle, a
		//     type declared inside the handler (api_references.go:127): db.EntityReference plus
		//     title (shadowing the embedded one, no omitempty) and subtype; refWithTitleMirror mirrors it.
		shapes: []contractShape{
			shape(tReq(db.EntityReference{}), "api_references.go:222 handleCreateReference: c.JSON(201, fwd), fwd is *db.EntityReference"),
			shape(tReq(refWithTitleMirror{}), "api_references.go:155 handleListReferences: element of the data array, refWithTitle mirrored by refWithTitleMirror"),
		},
		// The request body is the anonymous struct at api_references.go:166; createReferenceRequestMirror mirrors it.
		requests: []reflect.Type{tReq(createReferenceRequestMirror{})},
	},
	{
		component: "Error",
		//   errors.go:371 apiError: echo.NewHTTPError(status, errorBody{Message, Code, Params}); echo writes
		//     the errorBody as the response body (see the wire-shape note at the top of errors.go).
		//   echo.NewHTTPError(status, "text") sites: echo wraps the string as {"message": "text"}.
		shapes: []contractShape{
			shape(tReq(errorBody{}), "errors.go:371 apiError: errorBody{Message, Code, Params}"),
			shape(tReq(legacyErrorMirror{}), "echo DefaultHTTPErrorHandler: echo.NewHTTPError(status, string) is written as {\"message\": text}, mirrored by legacyErrorMirror"),
		},
	},
	{
		component: "ImplementationStatus",
		//   api_collab.go:3437 handleListImplementation: {"data": items} items is []db.ImplementationStatus
		//   api_collab.go:3506 handleUpdateImplementation: c.JSON(200, impl) impl is db.ImplementationStatus
		// PUT replaces the whole record (UpsertImplementationStatus, db/implementation.go:21-35:
		// ON CONFLICT DO UPDATE sets every column), so omitting owner, notes or target_date clears them.
		shapes:      []contractShape{shape(tReq(db.ImplementationStatus{}), "api_collab.go:3506 handleUpdateImplementation: c.JSON(200, impl), impl is db.ImplementationStatus")},
		requests:    []reflect.Type{tReq(implementationUpdateRequest{})},
		fullReplace: true,
	},
	{
		component: "Notification",
		//   api_collab.go:3785 handleListNotifications: {"data": notifications} notifications is []db.Notification
		shapes: []contractShape{shape(tReq(db.Notification{}), "api_collab.go:3785 handleListNotifications: element of the data array, db.Notification")},
	},
	{
		component: "OIDCProvider",
		//   api_admin.go:181 handleAdminListOIDC: {"data": providers} providers is []db.OIDCProvider; client_secret
		//     is replaced by "********" when set (api_admin.go:175-179).
		//   api_admin.go:215 handleAdminCreateOIDC: c.JSON(201, p) p is db.OIDCProvider with ClientSecret cleared (api_admin.go:214)
		//   api_admin.go:261 handleAdminUpdateOIDC: c.JSON(200, p) p is db.OIDCProvider with ClientSecret cleared (api_admin.go:260)
		// The request body is the db struct itself (c.Bind(&p)), so every key binds; the handler
		// overwrites id and organization_id, and the database sets created_at.
		// Update returns the freshly bound struct: created_at is zero (null) or what the client sent
		// (api_admin.go:238, db/oidc.go:209-222 has no RETURNING).
		shapes: []contractShape{
			shape(tReq(db.OIDCProvider{}), "api_admin.go:215 handleAdminCreateOIDC: c.JSON(201, p), p is db.OIDCProvider"),
			shape(tReq(db.OIDCProvider{}), "api_admin.go:261 handleAdminUpdateOIDC: c.JSON(200, p), p is db.OIDCProvider bound from the request"),
		},
		requests:  []reflect.Type{tReq(db.OIDCProvider{})},
		serverSet: []string{"id", "organization_id", "created_at"},
	},
	{
		component: "Organization",
		//   api_users.go:212 handleMyOrganizations: {"data": result} result is []orgWithRole, a type declared
		//     inside the handler (api_users.go:203): db.Organization plus role; orgWithRoleMirror mirrors it.
		//   api_users.go:318 handleCreateOrganization: c.JSON(201, map[string]interface{}{uuid, name, slug});
		//     createdOrgMirror mirrors the map.
		shapes: []contractShape{
			shape(tReq(orgWithRoleMirror{}), "api_users.go:212 handleMyOrganizations: element of the data array, orgWithRole mirrored by orgWithRoleMirror"),
			shape(tReq(createdOrgMirror{}), "api_users.go:318 handleCreateOrganization: map literal mirrored by createdOrgMirror"),
		},
	},
	{
		component: "Review",
		//   api_collab.go:353 handleListReviews: {"data": items, ...} items is []db.Review (PaginatedReviews)
		//   api_collab.go:466 handleCreateReview: c.JSON(201, review) review is *db.Review
		//   api_collab.go:498 handleGetReview: c.JSON(200, resp) resp is *reviewWithAgent, a type declared
		//     inside the handler (api_collab.go:480): *db.Review plus author_is_agent, changed_since_sent and
		//     has_reviewer_edits; reviewWithAgentMirror mirrors it (db.Review by value, same JSON).
		shapes: []contractShape{
			shape(tReq(db.Review{}), "api_collab.go:466 handleCreateReview: c.JSON(201, review), review is *db.Review"),
			shape(tReq(reviewWithAgentMirror{}), "api_collab.go:498 handleGetReview: c.JSON(200, resp), reviewWithAgent mirrored by reviewWithAgentMirror"),
		},
	},
	{
		component: "SearchResult",
		//   server.go:1290 handleSearchDocuments: {"data": results} results is []store.SearchResult
		shapes: []contractShape{shape(tReq(store.SearchResult{}), "server.go:1290 handleSearchDocuments: element of the data array, store.SearchResult")},
	},
	{
		component: "Suggestion",
		//   api_suggestions.go:277 handleListEntitySuggestions: {"data": kept} kept is []db.Suggestion (ListSuggestions)
		//   api_suggestions.go:221 handleCreateEntitySuggestion: c.JSON(201, sg) sg is db.Suggestion
		shapes: []contractShape{shape(tReq(db.Suggestion{}), "api_suggestions.go:221 handleCreateEntitySuggestion: c.JSON(201, sg), sg is db.Suggestion")},
	},
	{
		component: "UserWithRole",
		//   api_users.go:123 handleListUsers: {"data": users} users is []db.UserWithRole (ListOrgUsers)
		//   api_admin.go:47 handleAdminListMembers: {"data": users} users is []db.UserWithRole (ListOrgUsers)
		// UserWithRole is allOf User plus role; db.User is only ever serialized embedded in db.UserWithRole.
		shapes: []contractShape{shape(tReq(db.UserWithRole{}), "api_users.go:123 handleListUsers: element of the data array, db.UserWithRole")},
	},
	{
		component: "LoginResponse",
		//   api_auth.go:176 handleLogin, api_auth.go:419 handleSwitchOrg, api_passkey.go:340
		//   handlePasskeyLoginComplete, api_auth.go:1073 handleVerifyEmail: c.JSON(200, loginResponse{...});
		//   api_auth.go:92 returns loginResponse{OTPRequired: true} alone: token, email, name and role
		//   have no omitempty, so they are emitted as empty strings.
		shapes: []contractShape{shape(tReq(loginResponse{}), "api_auth.go:176 handleLogin: c.JSON(200, loginResponse{...})")},
	},
	{
		component: "CurrentUser",
		//   api_users.go:107 handleMe: c.JSON(200, resp) where resp is a map[string]interface{} built at
		//     api_users.go:79-96, plus organization_uuid/slug/name added at api_users.go:98-104 when the
		//     request has an organization; meMirror mirrors it.
		//   api_users.go:27-33 handleMe, no authenticated user: a map with email, name, role, authenticated;
		//     anonymousMeMirror mirrors it.
		shapes: []contractShape{
			shape(tReq(meMirror{}), "api_users.go:107 handleMe: c.JSON(200, resp), map mirrored by meMirror"),
			shape(tReq(anonymousMeMirror{}), "api_users.go:27 handleMe: map literal mirrored by anonymousMeMirror"),
		},
	},
}

// noGoType lists components that are not responses of any handler, with the
// reason.
var noGoType = map[string]string{
	"EpochSeconds":         "primitive shared by response schemas (db.Epoch), no handler returns it alone",
	"EpochSecondsNullable": "primitive shared by response schemas (db.Epoch), no handler returns it alone",
	"User":                 "db.User is only serialized embedded in db.UserWithRole (db/users.go:43), which is checked and merges this schema through allOf; no handler returns a bare db.User",
}

// neverNullKeys are JSON keys the database always fills; see the header.
var neverNullKeys = map[string]bool{"created_at": true, "updated_at": true}

// neverNullByComponent lists value-Epoch keys that are NOT NULL DEFAULT now()
// in the database and filled by the handler before insert, so they never
// marshal as null in practice. Each entry cites the column and the code.
var neverNullByComponent = map[string]map[string]string{
	"DocumentNeedingReview": {
		"change_summary": "both branches of handleNeedsReview set a non-empty slice: server.go:1909 (never reviewed) and server.go:1944-1950 (changeSummary falls back to a one-item slice)",
	},
	"AccessReview": {
		"reviewed_at": "access_reviews.reviewed_at is NOT NULL DEFAULT now(); handleCreateAccessReview (server.go:3244) fills a zero value before insert",
	},
	"Incident": {
		"detected_at": "incidents.detected_at is NOT NULL DEFAULT now(); applyIncidentDefaults (api_writepath_defaults.go:225) fills a zero value",
	},
	"Checkin": {
		"occurred_at": "checkins.occurred_at is NOT NULL DEFAULT now(); CreateCheckin (db/checkins.go:43) fills a zero value",
		"recorded_at": "checkins.recorded_at is NOT NULL DEFAULT now(); CreateCheckin inserts now() (db/checkins.go:49)",
	},
}

// notNeverNull lists keys of the global never-null set that are nevertheless
// null in some response of the component, with the evidence.
var notNeverNull = map[string]map[string]string{
	"OIDCProvider": {
		"created_at": "handleAdminUpdateOIDC (api_admin.go:238) binds a fresh db.OIDCProvider and UpdateOIDCProvider (db/oidc.go:209-222) has no RETURNING, so the update response carries null, or whatever the client sent",
	},
}

// neverPopulated lists struct fields a handler's query never fills, so the
// response never carries them even though the Go type has the field. The fill
// leaves them at the zero value instead of fabricating them.
var neverPopulated = map[string]map[string]string{
	"UserWithRole": {
		"pending_email": "ListOrgUsers (db/users.go:701, scan at :715) does not select pending_email",
		"locale":        "ListOrgUsers (db/users.go:701, scan at :715) does not select locale",
	},
}

func isNeverNull(path, key string) bool {
	comp := path[:strings.Index(path+".", ".")]
	if _, ok := notNeverNull[comp][key]; ok {
		return false
	}
	if neverNullKeys[key] {
		return true
	}
	_, ok := neverNullByComponent[comp][key]
	return ok
}

// zeroJSONField sets the field with the given JSON name (searching embedded
// structs) back to its zero value.
func zeroJSONField(v reflect.Value, name string) bool {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		n := strings.Split(f.Tag.Get("json"), ",")[0]
		if f.Anonymous && n == "" && f.Type.Kind() == reflect.Struct {
			if zeroJSONField(v.Field(i), name) {
				return true
			}
			continue
		}
		if n == name {
			v.Field(i).Set(reflect.Zero(f.Type))
			return true
		}
	}
	return false
}

// rawJSONKeys returns the JSON names of top-level json.RawMessage fields. The
// fill can only invent a placeholder for them ({"k":"v"}), so the JSON type the
// schema documents for them is not asserted from it: the type comes from the
// evidence cited in the schema description, not from the fill.
func rawJSONKeys(t reflect.Type, into map[string]bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		n := strings.Split(f.Tag.Get("json"), ",")[0]
		if f.Anonymous && n == "" && f.Type.Kind() == reflect.Struct {
			rawJSONKeys(f.Type, into)
			continue
		}
		if f.Type == rawJSONType && n != "" && n != "-" {
			into[n] = true
		}
	}
}

// inlineRequestChecks lists operations whose request body is an inline schema
// (not a component) that must still match the request DTO.
var inlineRequestChecks = []struct {
	op  string
	dto reflect.Type
}{
	{"PUT /audits/{id}", reflect.TypeOf(auditUpdateRequest{})},
	{"PUT /audit-findings/{id}", reflect.TypeOf(auditFindingUpdateRequest{})},
	// api_auth.go:388 handleSwitchOrg binds an anonymous struct with "slug".
	{"POST /auth/switch-org", reflect.TypeOf(switchOrgRequestMirror{})},
}

// api_auth.go:388 the anonymous request struct of handleSwitchOrg.
type switchOrgRequestMirror struct {
	Slug string `json:"slug"`
}

type specDoc struct {
	schemas specSchemas
	// requestBodies maps a component to the "METHOD /path" operations that
	// reference it directly as their request body.
	requestBodies map[string][]string
	// inlineRequests maps "METHOD /path" to an inline (non-$ref) request schema.
	inlineRequests map[string]map[string]any
	// paths is the raw paths object, for inline response schemas.
	paths map[string]map[string]any
}

type specSchemas map[string]any

func loadSpec(t *testing.T) specDoc {
	t.Helper()
	var doc struct {
		Paths      map[string]map[string]any `yaml:"paths"`
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(openAPISpec, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	if len(doc.Components.Schemas) == 0 {
		t.Fatal("openapi.yaml has no components/schemas")
	}
	out := specDoc{schemas: doc.Components.Schemas, requestBodies: map[string][]string{}, inlineRequests: map[string]map[string]any{}, paths: doc.Paths}
	const prefix = "#/components/schemas/"
	for path, item := range doc.Paths {
		for method, op := range item {
			opm, ok := op.(map[string]any)
			if !ok {
				continue
			}
			rb, _ := opm["requestBody"].(map[string]any)
			content, _ := rb["content"].(map[string]any)
			for _, mt := range content {
				mtm, _ := mt.(map[string]any)
				sch, _ := mtm["schema"].(map[string]any)
				if ref, ok := sch["$ref"].(string); ok && strings.HasPrefix(ref, prefix) {
					name := strings.TrimPrefix(ref, prefix)
					out.requestBodies[name] = append(out.requestBodies[name], strings.ToUpper(method)+" "+path)
				} else if sch != nil {
					out.inlineRequests[strings.ToUpper(method)+" "+path] = sch
				}
			}
		}
	}
	for _, ops := range out.requestBodies {
		sort.Strings(ops)
	}
	return out
}

func loadSpecSchemas(t *testing.T) specSchemas { return loadSpec(t).schemas }

// resolve follows $ref (within components/schemas) and overlays sibling
// keywords such as description and readOnly, which OpenAPI 3.1 allows.
func (s specSchemas) resolve(node any) (map[string]any, error) {
	m, ok := node.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema is %T, not a mapping", node)
	}
	if parts, ok := m["allOf"].([]any); ok {
		// allOf: merge the parts' properties and required lists.
		merged := map[string]any{}
		props := map[string]any{}
		var required []any
		for k, v := range m {
			switch k {
			case "allOf":
			case "properties": // siblings of allOf merge with the parts' properties
				for pk, pv := range v.(map[string]any) {
					props[pk] = pv
				}
			case "required":
				required = append(required, v.([]any)...)
			default:
				merged[k] = v
			}
		}
		for _, part := range parts {
			pm, err := s.resolve(part)
			if err != nil {
				return nil, err
			}
			for k, v := range pm {
				switch k {
				case "properties":
					for pk, pv := range v.(map[string]any) {
						props[pk] = pv
					}
				case "required":
					required = append(required, v.([]any)...)
				default:
					if _, ok := merged[k]; !ok {
						merged[k] = v
					}
				}
			}
		}
		merged["properties"] = props
		if required != nil {
			merged["required"] = required
		}
		return merged, nil
	}
	ref, ok := m["$ref"].(string)
	if !ok {
		return m, nil
	}
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, fmt.Errorf("unsupported $ref %q", ref)
	}
	target, ok := s[strings.TrimPrefix(ref, prefix)]
	if !ok {
		return nil, fmt.Errorf("$ref %q does not resolve", ref)
	}
	base, err := s.resolve(target)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(base)+len(m))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range m {
		if k != "$ref" {
			merged[k] = v
		}
	}
	return merged, nil
}

func schemaTypes(m map[string]any) []string {
	switch v := m["type"].(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func hasType(types []string, want string) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

func allowsNull(m map[string]any) bool { return hasType(schemaTypes(m), "null") }

func flag(m map[string]any, name string) bool {
	b, _ := m[name].(bool)
	return b
}

func jsonType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		if x == float64(int64(x)) {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

func isRequired(m map[string]any, key string) bool {
	list, _ := m["required"].([]any)
	for _, r := range list {
		if r == key {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// objSeen accumulates, across shapes, which keys the responses emitted for one
// object path.
type objSeen struct {
	path   string
	sch    map[string]any
	shapes int
	keys   map[string]int // key -> number of shapes that emitted it
}

// checkFull compares one fully populated response value with its schema:
// types, and keys the spec does not document. Keys the response emitted are
// recorded in seen; "documented but absent" is judged after all shapes ran.
func (s specSchemas) checkFull(path string, val any, node any, seen map[string]*objSeen, skip map[string]bool, errs *[]string) {
	if skip[path] {
		return
	}
	// Objects reached through the same $ref share one record, so a recursive
	// schema (a folder holding folders) is judged across all its levels.
	seenKey := path
	if nm, ok := node.(map[string]any); ok {
		if ref, ok := nm["$ref"].(string); ok {
			seenKey = "ref:" + ref
		}
	}
	sch, err := s.resolve(node)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: %v", path, err))
		return
	}
	for _, kw := range []string{"allOf", "oneOf", "anyOf"} {
		if _, ok := sch[kw]; ok {
			*errs = append(*errs, fmt.Sprintf("%s: %s is not supported by the contract test", path, kw))
			return
		}
	}
	types := schemaTypes(sch)
	if len(types) == 0 {
		*errs = append(*errs, fmt.Sprintf("%s: schema has no type", path))
		return
	}
	jt := jsonType(val)
	if !hasType(types, jt) && !(jt == "integer" && hasType(types, "number")) {
		*errs = append(*errs, fmt.Sprintf("%s: spec type %s, emitted %s", path, strings.Join(types, "|"), jt))
		return
	}
	switch v := val.(type) {
	case map[string]any:
		props, _ := sch["properties"].(map[string]any)
		if props == nil {
			if ap, ok := sch["additionalProperties"].(map[string]any); ok {
				for _, k := range sortedKeys(v) {
					s.checkFull(path+"."+k, v[k], ap, seen, skip, errs)
				}
			}
			return
		}
		o := seen[seenKey]
		if o == nil {
			o = &objSeen{path: path, sch: sch, keys: map[string]int{}}
			seen[seenKey] = o
		}
		o.shapes++
		for _, k := range sortedKeys(v) {
			o.keys[k]++
			p, ok := props[k]
			if !ok {
				*errs = append(*errs, fmt.Sprintf("%s: response has %q but the spec does not document it", path, k))
				continue
			}
			if ps, err := s.resolve(p); err == nil && flag(ps, "writeOnly") {
				*errs = append(*errs, fmt.Sprintf("%s.%s: marked writeOnly but the response emits it", path, k))
			}
			s.checkFull(path+"."+k, v[k], p, seen, skip, errs)
		}
	case []any:
		if items, ok := sch["items"]; ok {
			for i, e := range v {
				s.checkFull(fmt.Sprintf("%s[%d]", path, i), e, items, seen, skip, errs)
			}
		}
	}
}

// checkUnion reports documented keys no shape emitted (writeOnly keys are
// request-only and exempt) and required keys some shape did not emit.
func (s specSchemas) checkUnion(seen map[string]*objSeen, errs *[]string) {
	for _, sk := range sortedKeys(seen) {
		o := seen[sk]
		path := o.path
		props, _ := o.sch["properties"].(map[string]any)
		reqs, _ := o.sch["required"].([]any)
		for _, r := range reqs {
			if name, _ := r.(string); props[name] == nil {
				*errs = append(*errs, fmt.Sprintf("%s: required lists %q which is not a property", path, name))
			}
		}
		for _, k := range sortedKeys(props) {
			ps, err := s.resolve(props[k])
			if err != nil {
				*errs = append(*errs, fmt.Sprintf("%s.%s: %v", path, k, err))
				continue
			}
			if o.keys[k] == 0 && !flag(ps, "writeOnly") {
				*errs = append(*errs, fmt.Sprintf("%s: spec documents %q but the response has no such key", path, k))
			}
			if o.keys[k] > 0 && o.keys[k] < o.shapes && isRequired(o.sch, k) {
				*errs = append(*errs, fmt.Sprintf("%s: %q is emitted by only some response shapes, so it must not be in required", path, k))
			}
		}
	}
}

// zeroFact records what the zero-value marshaling did to one key, per shape.
type zeroFact struct {
	sch                 map[string]any // the object schema holding the key
	prop                map[string]any
	null, omitted, vals int
}

// collectZero compares the zero-value marshaling (full is the populated one,
// used to learn which keys exist) with the schema's nullability and required.
// Facts are judged after all shapes ran (see judgeZero).
func (s specSchemas) collectZero(path string, zero, full map[string]any, node any, facts map[string]*zeroFact) {
	sch, err := s.resolve(node)
	if err != nil {
		return // already reported by checkFull
	}
	props, _ := sch["properties"].(map[string]any)
	for _, k := range sortedKeys(full) {
		pnode, ok := props[k]
		if !ok {
			continue // reported by checkFull
		}
		ps, err := s.resolve(pnode)
		if err != nil {
			continue
		}
		p := path + "." + k
		f := facts[p]
		if f == nil {
			f = &zeroFact{sch: sch, prop: ps}
			facts[p] = f
		}
		zv, present := zero[k]
		switch {
		case !present:
			f.omitted++
		case zv == nil:
			f.null++
		default:
			f.vals++
			zm, zok := zv.(map[string]any)
			fm, fok := full[k].(map[string]any)
			if zok && fok {
				s.collectZero(p, zm, fm, ps, facts)
			}
		}
	}
}

func (s specSchemas) judgeZero(facts map[string]*zeroFact, clearable map[string]bool, errs *[]string) {
	for _, p := range sortedKeys(facts) {
		f := facts[p]
		key := p[strings.LastIndex(p, ".")+1:]
		typ := strings.Join(schemaTypes(f.prop), "|")
		switch {
		case isNeverNull(p, key):
			if allowsNull(f.prop) {
				*errs = append(*errs, fmt.Sprintf("%s: this key is never null in practice (the database or the handler always sets it), so the spec type must not include null", p))
			}
		case clearable[p]:
			// A null from the client clears the value, so the type includes
			// null whether or not the response ever emits it (checked with the
			// request fields).
		case f.null > 0:
			if !allowsNull(f.prop) {
				*errs = append(*errs, fmt.Sprintf("%s: emitted null at its zero value, spec type %s does not include null", p, typ))
			}
		default:
			if allowsNull(f.prop) {
				why := "the response never emits null for it"
				if f.omitted > 0 && f.vals == 0 {
					why = "it is omitted when empty (omitempty), so the type must not include null"
				}
				*errs = append(*errs, fmt.Sprintf("%s: spec type includes null but %s", p, why))
			}
		}
		if f.omitted > 0 && isRequired(f.sch, key) {
			*errs = append(*errs, fmt.Sprintf("%s: omitted when empty (omitempty), so it must not be in required", p))
		}
	}
}

var (
	epochType   = reflect.TypeOf(db.Epoch{})
	timeType    = reflect.TypeOf(time.Time{})
	rawJSONType = reflect.TypeOf(json.RawMessage{})
)

// fill sets every exported field: pointers non-nil, slices and maps with one
// element, Epoch and time non-zero, json.RawMessage a valid JSON object.
func fill(v reflect.Value, depth int) {
	if depth > 6 {
		// Recursive types stop growing here: an empty slice or map keeps the
		// deepest level marshaling as [] or {} rather than null, a pointer
		// stays nil, and scalars are still filled.
		switch v.Kind() {
		case reflect.Slice:
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
			return
		case reflect.Map:
			v.Set(reflect.MakeMap(v.Type()))
			return
		case reflect.Ptr:
			return
		}
	}
	switch v.Type() {
	case epochType:
		v.Set(reflect.ValueOf(db.Epoch{Time: time.Unix(1700000000, 0).UTC()}))
		return
	case timeType:
		v.Set(reflect.ValueOf(time.Unix(1700000000, 0).UTC()))
		return
	case rawJSONType:
		v.SetBytes([]byte(`{"k":"v"}`))
		return
	}
	switch v.Kind() {
	case reflect.Ptr:
		v.Set(reflect.New(v.Type().Elem()))
		fill(v.Elem(), depth+1)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				fill(v.Field(i), depth+1)
			}
		}
	case reflect.Slice:
		sl := reflect.MakeSlice(v.Type(), 1, 1)
		fill(sl.Index(0), depth+1)
		v.Set(sl)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		fill(key, depth+1)
		val := reflect.New(v.Type().Elem()).Elem()
		fill(val, depth+1)
		m.SetMapIndex(key, val)
		v.Set(m)
	case reflect.Interface:
		v.Set(reflect.ValueOf("x"))
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	}
}

func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("response is not a JSON object: %v", err)
	}
	return m
}

// jsonFields returns the JSON keys a struct accepts or emits at the top level
// with their Go types.
func jsonFields(t reflect.Type, into map[string]reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			jsonFields(f.Type, into)
			continue
		}
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		into[name] = f.Type
	}
}

// acceptedKinds returns the JSON types a request field of Go type t decodes.
// nil means "anything" (interfaces, raw JSON).
func acceptedKinds(t reflect.Type) []string {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct && strings.HasPrefix(t.Name(), "Optional[") {
		if f, ok := t.FieldByName("Value"); ok {
			return acceptedKinds(f.Type)
		}
	}
	switch t {
	case epochType:
		return []string{"integer", "string"} // Epoch.UnmarshalJSON: seconds or a date string
	case timeType:
		return []string{"string"}
	case rawJSONType:
		return nil
	}
	switch t.Kind() {
	case reflect.String:
		return []string{"string"}
	case reflect.Bool:
		return []string{"boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []string{"integer"}
	case reflect.Float32, reflect.Float64:
		return []string{"number", "integer"}
	case reflect.Slice, reflect.Array:
		return []string{"array"}
	case reflect.Map, reflect.Struct:
		return []string{"object"}
	}
	return nil
}

func isOptional(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct && strings.HasPrefix(t.Name(), "Optional[")
}

func isUpdateDTO(t reflect.Type) bool { return strings.HasSuffix(t.Name(), "UpdateRequest") }

const (
	clearPhrase      = "Send null on update to clear."
	createOnlyPhrase = "ignored on update"
)

// checkRequestFields compares the fields a request type decodes with the
// schema properties: both directions, and the type each field can decode. For
// an update request, an Optional field clears on null (so the schema type
// includes null and says so) and any other field must not advertise null.
func (s specSchemas) checkRequestFields(label string, props map[string]any, fields map[string]reflect.Type, update bool, phrase string, errs *[]string) {
	for _, k := range sortedKeys(fields) {
		if _, ok := props[k]; !ok {
			*errs = append(*errs, fmt.Sprintf("%s: the request accepts %q but the spec does not document it", label, k))
		}
	}
	for _, k := range sortedKeys(props) {
		rt, ok := fields[k]
		if !ok {
			*errs = append(*errs, fmt.Sprintf("%s.%s: documented but the request does not accept it", label, k))
			continue
		}
		ps, err := s.resolve(props[k])
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s.%s: %v", label, k, err))
			continue
		}
		if kinds := acceptedKinds(rt); kinds != nil {
			for _, st := range schemaTypes(ps) {
				if st != "null" && !hasType(kinds, st) {
					*errs = append(*errs, fmt.Sprintf("%s.%s: spec type %s but the request field (%s) only decodes %s", label, k, st, rt, strings.Join(kinds, "|")))
				}
			}
		}
		if update {
			desc, _ := ps["description"].(string)
			switch {
			case isOptional(rt) && !allowsNull(ps):
				*errs = append(*errs, fmt.Sprintf("%s.%s: the update request clears this key on null (Optional) but the spec type does not include null", label, k))
			case isOptional(rt) && !strings.Contains(desc, phrase):
				*errs = append(*errs, fmt.Sprintf("%s.%s: the update request clears this key on null, so the description must say %q", label, k, phrase))
			case !isOptional(rt) && allowsNull(ps):
				*errs = append(*errs, fmt.Sprintf("%s.%s: spec type includes null but the update field is not Optional, so null does not clear it", label, k))
			}
		}
	}
}

func TestOpenAPIContract(t *testing.T) {
	spec := loadSpec(t)
	schemas := spec.schemas
	for _, e := range contractTable {
		t.Run(e.component, func(t *testing.T) {
			node, ok := schemas[e.component]
			if !ok {
				t.Fatalf("%s: in contractTable but not in components/schemas", e.component)
			}
			if len(e.shapes) == 0 {
				t.Fatalf("%s: contractTable entry has no shapes", e.component)
			}
			var errs []string
			defer func() {
				sort.Strings(errs)
				for _, msg := range errs {
					t.Error(msg)
				}
			}()
			sch, err := schemas.resolve(node)
			if err != nil {
				t.Fatalf("%s: %v", e.component, err)
			}
			props, _ := sch["properties"].(map[string]any)

			// A request-only component is checked as a request shape.
			if e.requestOnly {
				fields := map[string]reflect.Type{}
				jsonFields(e.shapes[0].typ, fields)
				schemas.checkRequestFields(e.component, props, fields, false, "", &errs)
				return
			}

			// Request-body coverage: a component some operation takes as its
			// request body must say which DTOs it binds, or the readOnly and
			// writeOnly checks below would silently be skipped.
			if ops := spec.requestBodies[e.component]; len(ops) > 0 && len(e.requests) == 0 {
				errs = append(errs, fmt.Sprintf("component %s is a request body for %s but lists no request DTOs", e.component, ops[0]))
			}

			// Create and update requests stay separate: a key only a create
			// request accepts is ignored on update, and only Optional update
			// fields clear on null.
			create := map[string]reflect.Type{}
			update := map[string]reflect.Type{}
			for _, r := range e.requests {
				if isUpdateDTO(r) {
					jsonFields(r, update)
				} else {
					jsonFields(r, create)
				}
			}
			serverSet := map[string]bool{}
			for _, k := range e.serverSet {
				serverSet[k] = true
			}
			accepted := map[string]reflect.Type{}
			for k, rt := range create {
				if !serverSet[k] {
					accepted[k] = rt
				}
			}
			for k, rt := range update {
				accepted[k] = rt
			}
			clearable := map[string]bool{}
			for k, rt := range update {
				if isOptional(rt) || (e.fullReplace && rt.Kind() == reflect.Ptr) {
					clearable[e.component+"."+k] = true
				}
			}

			seen := map[string]*objSeen{}
			facts := map[string]*zeroFact{}
			emitted := map[string]bool{}
			for _, sh := range e.shapes {
				if sh.handler == "" {
					errs = append(errs, fmt.Sprintf("%s: a shape has no handler file:line comment", e.component))
				}
				filled := reflect.New(sh.typ)
				fill(filled.Elem(), 0)
				for _, k := range sortedKeys(neverPopulated[e.component]) {
					if !zeroJSONField(filled.Elem(), k) {
						errs = append(errs, fmt.Sprintf("%s: neverPopulated lists %q which is not a field of %s", e.component, k, sh.typ))
					}
				}
				full := marshalToMap(t, filled.Interface())
				zero := marshalToMap(t, reflect.New(sh.typ).Interface())
				for k := range full {
					emitted[k] = true
				}
				raw := map[string]bool{}
				rawJSONKeys(sh.typ, raw)
				skip := map[string]bool{}
				for k := range raw {
					skip[e.component+"."+k] = true
				}
				schemas.checkFull(e.component, full, node, seen, skip, &errs)
				schemas.collectZero(e.component, zero, full, node, facts)
			}
			schemas.checkUnion(seen, &errs)
			schemas.judgeZero(facts, clearable, &errs)

			if len(e.requests) == 0 {
				return
			}
			for _, k := range sortedKeys(accepted) {
				if _, ok := props[k]; !ok {
					errs = append(errs, fmt.Sprintf("%s: a request body accepts %q but the spec does not document it", e.component, k))
				}
			}
			for _, k := range sortedKeys(props) {
				ps, err := schemas.resolve(props[k])
				if err != nil {
					continue
				}
				desc, _ := ps["description"].(string)
				ro, wo := flag(ps, "readOnly"), flag(ps, "writeOnly")
				rt, acc := accepted[k]
				switch {
				case ro && acc:
					errs = append(errs, fmt.Sprintf("%s.%s: marked readOnly but the request body accepts it", e.component, k))
				case !ro && !acc && emitted[k]:
					errs = append(errs, fmt.Sprintf("%s.%s: not readOnly but the request body does not accept it", e.component, k))
				case wo && emitted[k]:
					// reported by checkFull
				case wo && !acc:
					errs = append(errs, fmt.Sprintf("%s.%s: marked writeOnly but no request body accepts it", e.component, k))
				case !wo && acc && !emitted[k]:
					errs = append(errs, fmt.Sprintf("%s.%s: accepted by a request body but never emitted, so it must be writeOnly", e.component, k))
				}
				if !acc || ro {
					continue
				}
				// Request field type: the schema type is what a client sends,
				// so the DTO field must decode it.
				if kinds := acceptedKinds(rt); kinds != nil {
					for _, st := range schemaTypes(ps) {
						if st != "null" && !hasType(kinds, st) {
							errs = append(errs, fmt.Sprintf("%s.%s: spec type %s but the request field (%s) only decodes %s", e.component, k, st, rt, strings.Join(kinds, "|")))
						}
					}
				}
				if len(update) == 0 {
					continue
				}
				if urt, inUpdate := update[k]; inUpdate {
					switch {
					case isOptional(urt) && !allowsNull(ps):
						errs = append(errs, fmt.Sprintf("%s.%s: the update request clears this key on null (Optional) but the spec type does not include null", e.component, k))
					case isOptional(urt) && !strings.Contains(desc, clearPhrase):
						errs = append(errs, fmt.Sprintf("%s.%s: the update request clears this key on null, so the description must say %q", e.component, k, clearPhrase))
					case e.fullReplace && urt.Kind() == reflect.Ptr && !allowsNull(ps):
						errs = append(errs, fmt.Sprintf("%s.%s: the update request replaces the record, so null clears this key, but the spec type does not include null", e.component, k))
					case e.fullReplace && urt.Kind() == reflect.Ptr && !strings.Contains(desc, "Omit or send null to clear."):
						errs = append(errs, fmt.Sprintf("%s.%s: the update request replaces the record, so the description must say %q", e.component, k, "Omit or send null to clear."))
					case e.fullReplace && urt.Kind() == reflect.Ptr:
						// documented above
					case !isOptional(urt) && allowsNull(ps) && !isNeverNull(e.component, k) && !nullFromResponse(facts, e.component, k):
						errs = append(errs, fmt.Sprintf("%s.%s: spec type includes null but the update field is not Optional, so null does not clear it", e.component, k))
					}
				} else if !strings.Contains(desc, createOnlyPhrase) {
					errs = append(errs, fmt.Sprintf("%s.%s: only a create request accepts it; the description must say it is %q", e.component, k, createOnlyPhrase))
				}
			}
		})
	}

	for _, c := range inlineRequestChecks {
		t.Run(c.op, func(t *testing.T) {
			sch, ok := spec.inlineRequests[c.op]
			if !ok {
				t.Fatalf("%s: no inline request schema in the spec", c.op)
			}
			props, _ := sch["properties"].(map[string]any)
			fields := map[string]reflect.Type{}
			jsonFields(c.dto, fields)
			var errs []string
			schemas.checkRequestFields(c.op, props, fields, isUpdateDTO(c.dto), "Send null", &errs)
			sort.Strings(errs)
			for _, msg := range errs {
				t.Error(msg)
			}
		})
	}
}

// nullFromResponse reports whether the response itself emits null at the key's
// zero value, which legitimately puts null in the schema type.
func nullFromResponse(facts map[string]*zeroFact, comp, key string) bool {
	f := facts[comp+"."+key]
	return f != nil && f.null > 0
}

// Every component must be checked, deliberately exempt, or pending.
func TestOpenAPIComponentsAccountedFor(t *testing.T) {
	schemas := loadSpecSchemas(t)
	inTable := map[string]bool{}
	for _, e := range contractTable {
		if inTable[e.component] {
			t.Errorf("component %s appears twice in contractTable", e.component)
		}
		inTable[e.component] = true
	}
	for _, name := range sortedKeys(schemas) {
		n := 0
		if inTable[name] {
			n++
		}
		if _, ok := noGoType[name]; ok {
			n++
		}
		switch n {
		case 0:
			t.Errorf("component %s is not checked: add it to contractTable (with its handler file:line) or noGoType (with the reason it has no handler)", name)
		case 1:
		default:
			t.Errorf("component %s is listed in both contractTable and noGoType", name)
		}
	}
	for name := range noGoType {
		if _, ok := schemas[name]; !ok {
			t.Errorf("noGoType lists %s which is not in components/schemas", name)
		}
	}
}

func TestOpenAPIEpochSchemas(t *testing.T) {
	schemas := loadSpecSchemas(t)
	for name, want := range map[string]string{"EpochSeconds": "integer", "EpochSecondsNullable": "integer|null"} {
		node, ok := schemas[name]
		if !ok {
			t.Errorf("%s is missing from components/schemas", name)
			continue
		}
		sch, err := schemas.resolve(node)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := strings.Join(schemaTypes(sch), "|"); got != want {
			t.Errorf("%s: spec type %s, want %s", name, got, want)
		}
		if sch["format"] != "int64" {
			t.Errorf("%s: format must be int64, got %v", name, sch["format"])
		}
	}
}

// Mirrors of response shapes that handlers build without a named exported
// type: a map[string]interface{} literal or a type declared inside the
// handler. Each mirrors the keys and value types the handler really emits;
// the comment on the contractTable entry cites the construction.

// api_auth.go:1145 handleCreateMyAPIKey (map literal).
type createdAPIKeyMirror struct {
	Token          string `json:"token"`
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Permissions    string `json:"permissions"`
	OrganizationID *int   `json:"organization_id"`
}

// server.go:1232-1262 handleGetDocument (map literal; the active_review_* keys
// are added only for a document in review, hence pointers with omitempty).
type documentMirror struct {
	DocumentID                    string  `json:"document_id"`
	Title                         string  `json:"title"`
	Type                          string  `json:"type"`
	Version                       string  `json:"version"`
	Status                        string  `json:"status"`
	Author                        string  `json:"author"`
	Owner                         string  `json:"owner"`
	ReviewCycle                   int     `json:"review_cycle"`
	NextReview                    string  `json:"next_review"`
	Content                       string  `json:"content"`
	Folder                        string  `json:"folder"`
	ActiveReviewID                *int    `json:"active_review_id,omitempty"`
	ActiveReviewStatus            *string `json:"active_review_status,omitempty"`
	ActiveReviewRound             *int    `json:"active_review_round,omitempty"`
	ActiveReviewChangedSinceSent  *bool   `json:"active_review_changed_since_sent,omitempty"`
	ActiveReviewHasReviewerEdits  *bool   `json:"active_review_has_reviewer_edits,omitempty"`
	ActiveReviewPendingSuggestion *int    `json:"active_review_pending_suggestions,omitempty"`
}

// server.go:1096 DocFile, declared inside handleListAllDocuments.
type docFileMirror struct {
	DocumentID string `json:"document_id"`
	Title      string `json:"title"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Author     string `json:"author"`
	Path       string `json:"path"`
	Folder     string `json:"folder"`
}

// server.go:1105 DocFolder, declared inside handleListAllDocuments.
type docFolderMirror struct {
	Name       string            `json:"name"`
	Title      string            `json:"title,omitempty"`
	Files      []docFileMirror   `json:"files"`
	SubFolders []docFolderMirror `json:"subfolders,omitempty"`
}

// server.go:1841 NeedsReviewDoc, declared inside handleNeedsReview.
type needsReviewDocMirror struct {
	DocumentID        string   `json:"document_id"`
	Title             string   `json:"title"`
	Folder            string   `json:"folder"`
	Path              string   `json:"path"`
	CurrentCommit     string   `json:"current_commit"`
	CurrentCommitTime string   `json:"current_commit_time"`
	ApprovedCommit    string   `json:"approved_commit"`
	ApprovedAt        string   `json:"approved_at"`
	ApprovedVersion   string   `json:"approved_version"`
	ChangeSummary     []string `json:"change_summary"`
	NeverApproved     bool     `json:"never_approved"`
}

// api_references.go:127 refWithTitle, declared inside handleListReferences. The
// outer title shadows the embedded one, as in the handler.
type refWithTitleMirror struct {
	db.EntityReference
	Title   string `json:"title"`
	Subtype string `json:"subtype,omitempty"`
}

// api_references.go:166 the anonymous request struct of handleCreateReference.
type createReferenceRequestMirror struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
}

// echo.NewHTTPError(status, "text") is written by echo as {"message": "text"}.
type legacyErrorMirror struct {
	Message string `json:"message"`
}

// api_users.go:203 orgWithRole, declared inside handleMyOrganizations.
type orgWithRoleMirror struct {
	db.Organization
	Role string `json:"role"`
}

// api_users.go:318 handleCreateOrganization (map literal).
type createdOrgMirror struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// api_collab.go:480 reviewWithAgent, declared inside handleGetReview (it embeds
// *db.Review; by value marshals the same).
type reviewWithAgentMirror struct {
	db.Review
	AuthorIsAgent    bool  `json:"author_is_agent"`
	ChangedSinceSent *bool `json:"changed_since_sent,omitempty"`
	HasReviewerEdits *bool `json:"has_reviewer_edits,omitempty"`
}

// api_users.go:79-104 handleMe (map literal; organization_* keys are added only
// when the request has an organization and it resolves).
type meMirror struct {
	ID               int     `json:"id"`
	Email            string  `json:"email"`
	Name             string  `json:"name"`
	Role             string  `json:"role"`
	IsAgent          bool    `json:"is_agent"`
	Active           bool    `json:"active"`
	Authenticated    bool    `json:"authenticated"`
	HasPassword      bool    `json:"has_password"`
	OTPEnabled       bool    `json:"otp_enabled"`
	EmailVerified    bool    `json:"email_verified"`
	PendingEmail     *string `json:"pending_email"`
	OrganizationID   int     `json:"organization_id"`
	AIEnabled        bool    `json:"ai_enabled"`
	Locale           string  `json:"locale"`
	LocalePreference *string `json:"locale_preference"`
	OrganizationUUID string  `json:"organization_uuid,omitempty"`
	OrganizationSlug string  `json:"organization_slug,omitempty"`
	OrganizationName string  `json:"organization_name,omitempty"`
}

// api_users.go:27-33 handleMe without an authenticated user (map literal).
type anonymousMeMirror struct {
	Email         string `json:"email"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	Authenticated bool   `json:"authenticated"`
}

// api_oidc.go handleOIDCLinkBegin (map literal).
type oidcLinkBeginMirror struct {
	URL string `json:"url"`
}

// api_oidc.go handleOIDCLinkComplete (map literal).
type oidcLinkCompleteMirror struct {
	Result string `json:"result"`
}

// api_oidc.go handleUnlinkIdentity (map literal).
type unlinkIdentityMirror struct {
	Status string `json:"status"`
}

// inlineResponseChecks lists operations whose response schema (or the items of
// its data array) is inline, not a component, and must match the Go value the
// handler serializes.
var inlineResponseChecks = []struct {
	op      string
	status  string
	items   bool // the schema is properties.data.items of the response
	typ     reflect.Type
	handler string
}{
	{"GET /auth/oidc/providers", "200", true, tReq(publicOIDCProvider{}), "api_oidc.go:52 handleOIDCProviders: {\"data\": out} out is []publicOIDCProvider"},
	{"GET /auth/passkeys", "200", true, tReq(passkeyListItem{}), "api_passkey.go:387 handleListPasskeys: {\"data\": items} items is []passkeyListItem"},
	{"GET /auth/identities", "200", true, tReq(identityItem{}), "api_oidc.go handleListIdentities: {\"data\": items} items is []identityItem"},
	{"POST /auth/oidc/link", "200", false, tReq(oidcLinkBeginMirror{}), "api_oidc.go handleOIDCLinkBegin: map literal mirrored by oidcLinkBeginMirror"},
	{"POST /auth/oidc/link/complete", "200", false, tReq(oidcLinkCompleteMirror{}), "api_oidc.go handleOIDCLinkComplete: map literal mirrored by oidcLinkCompleteMirror"},
	{"DELETE /auth/identities/{id}", "200", false, tReq(unlinkIdentityMirror{}), "api_oidc.go handleUnlinkIdentity: map literal mirrored by unlinkIdentityMirror"},
	{"GET /inbox", "200", true, tReq(inboxItem{}), "api_collab.go:3952 handleInbox: {\"data\": items} items is []inboxItem"},
	{"GET /activity", "200", true, tReq(db.Activity{}), "api_collab.go:3845 handleListActivity: {\"data\": activities} activities is []db.Activity (RecentActivity)"},
	{"GET /documents/{docId}/activity", "200", true, tReq(db.Activity{}), "api_collab.go:3859 handleDocumentActivity: {\"data\": activities} activities is []db.Activity (ActivityForDocument)"},
}

func TestOpenAPIInlineResponses(t *testing.T) {
	spec := loadSpec(t)
	for _, c := range inlineResponseChecks {
		t.Run(c.op, func(t *testing.T) {
			parts := strings.SplitN(c.op, " ", 2)
			op, _ := spec.paths[parts[1]][strings.ToLower(parts[0])].(map[string]any)
			resp, _ := op["responses"].(map[string]any)[c.status].(map[string]any)
			content, _ := resp["content"].(map[string]any)
			mt, _ := content["application/json"].(map[string]any)
			var node any = mt["schema"]
			if node == nil {
				t.Fatalf("%s: no inline %s response schema in the spec", c.op, c.status)
			}
			if c.items {
				sch, _ := node.(map[string]any)
				props, _ := sch["properties"].(map[string]any)
				data, _ := props["data"].(map[string]any)
				node = data["items"]
				if node == nil {
					t.Fatalf("%s: the response has no data.items schema", c.op)
				}
			}
			if _, isRef := node.(map[string]any)["$ref"]; isRef {
				t.Fatalf("%s: the schema is a $ref; list it in contractTable instead", c.op)
			}
			var errs []string
			filled := reflect.New(c.typ)
			fill(filled.Elem(), 0)
			full := marshalToMap(t, filled.Interface())
			zero := marshalToMap(t, reflect.New(c.typ).Interface())
			seen := map[string]*objSeen{}
			facts := map[string]*zeroFact{}
			spec.schemas.checkFull(c.op, full, node, seen, nil, &errs)
			spec.schemas.collectZero(c.op, zero, full, node, facts)
			spec.schemas.checkUnion(seen, &errs)
			spec.schemas.judgeZero(facts, map[string]bool{}, &errs)
			sort.Strings(errs)
			for _, msg := range errs {
				t.Error(msg)
			}
		})
	}
}

// referenceTypesFromResolver returns the reference types the API accepts: the
// string cases of the switch on entityType in refs.Resolver.Lookup
// (internal/isms/refs/refs.go), the one place that decides whether a type names
// a real entity (unknown types fall to the default case: not found). It reads
// the source because the list is a switch, not a value.
func referenceTypesFromResolver(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join("..", "refs", "refs.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse refs.go: %v", err)
	}
	var types []string
	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "Lookup" {
			return true
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			if id, ok := sw.Tag.(*ast.Ident); !ok || id.Name != "entityType" {
				return true
			}
			for _, st := range sw.Body.List {
				cc := st.(*ast.CaseClause)
				for _, e := range cc.List {
					if bl, ok := e.(*ast.BasicLit); ok && bl.Kind == token.STRING {
						v, _ := strconv.Unquote(bl.Value)
						types = append(types, v)
					}
				}
			}
			return false
		})
		return false
	})
	sort.Strings(types)
	if len(types) == 0 {
		t.Fatal("found no reference types in refs.Resolver.Lookup")
	}
	return types
}

// referenceTypesFromMigrations returns the types the latest entity_references
// CHECK constraint (column is source_type or target_type) allows.
func referenceTypesFromMigrations(t *testing.T, column string) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.sql"))
	sort.Strings(files)
	re := regexp.MustCompile(`entity_references_` + column + `_check\s+CHECK \(` + column + ` IN \(([^)]*)\)\)`)
	var last []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
			last = nil
			for _, v := range strings.Split(m[1], ",") {
				last = append(last, strings.Trim(strings.TrimSpace(v), "'"))
			}
		}
	}
	if last == nil {
		t.Fatalf("no entity_references_%s_check constraint found in migrations", column)
	}
	sort.Strings(last)
	return last
}

func enumOf(sch map[string]any) []string {
	var out []string
	list, _ := sch["enum"].([]any)
	for _, v := range list {
		out = append(out, fmt.Sprint(v))
	}
	sort.Strings(out)
	return out
}

// The reference type enums in the spec must equal the types the code accepts,
// and the code and the database CHECK must agree.
func TestOpenAPIReferenceTypes(t *testing.T) {
	spec := loadSpec(t)
	canon := referenceTypesFromResolver(t)
	want := strings.Join(canon, ",")
	for _, col := range []string{"source_type", "target_type"} {
		if got := strings.Join(referenceTypesFromMigrations(t, col), ","); got != want {
			t.Errorf("refs.Resolver.Lookup accepts [%s] but the latest entity_references %s CHECK allows [%s]", want, col, got)
		}
	}
	prop := func(comp, key string) map[string]any {
		sch, err := spec.schemas.resolve(spec.schemas[comp])
		if err != nil {
			t.Fatal(err)
		}
		ps, err := spec.schemas.resolve(sch["properties"].(map[string]any)[key])
		if err != nil {
			t.Fatal(err)
		}
		return ps
	}
	for _, c := range []struct{ comp, key string }{
		{"EntityReference", "source_type"}, {"EntityReference", "target_type"}, {"ReferenceInput", "type"},
	} {
		if got := strings.Join(enumOf(prop(c.comp, c.key)), ","); got != want {
			t.Errorf("%s.%s: spec enum [%s] differs from the reference types in code [%s]", c.comp, c.key, got, want)
		}
	}
}

// queryParamChecks lists the query parameters a handler reads (c.QueryParam),
// to compare with the operation's documented parameters.
var queryParamChecks = []struct {
	op       string
	required []string // read and rejected with 400 when empty
	handler  string
}{
	{"GET /references", []string{"id", "type"}, "api_references.go:98-101 handleListReferences reads type and id only and returns 400 when either is empty"},
}

func TestOpenAPIQueryParams(t *testing.T) {
	spec := loadSpec(t)
	for _, c := range queryParamChecks {
		t.Run(c.op, func(t *testing.T) {
			parts := strings.SplitN(c.op, " ", 2)
			op, _ := spec.paths[parts[1]][strings.ToLower(parts[0])].(map[string]any)
			params, _ := op["parameters"].([]any)
			got := map[string]bool{}
			for _, p := range params {
				pm, _ := p.(map[string]any)
				if pm["in"] != "query" {
					continue
				}
				name, _ := pm["name"].(string)
				got[name] = flag(pm, "required")
			}
			for _, k := range c.required {
				req, ok := got[k]
				switch {
				case !ok:
					t.Errorf("%s: the handler reads query parameter %q but the spec does not document it", c.op, k)
				case !req:
					t.Errorf("%s: query parameter %q is required by the handler but not marked required", c.op, k)
				}
			}
			want := map[string]bool{}
			for _, k := range c.required {
				want[k] = true
			}
			for _, k := range sortedKeys(got) {
				if !want[k] {
					t.Errorf("%s: the spec documents query parameter %q but the handler does not read it", c.op, k)
				}
			}
			// the type parameter carries the reference type enum
			for _, p := range params {
				pm, _ := p.(map[string]any)
				if pm["name"] == "type" {
					sch, _ := pm["schema"].(map[string]any)
					if got, want := strings.Join(enumOf(sch), ","), strings.Join(referenceTypesFromResolver(t), ","); got != want {
						t.Errorf("%s: parameter type enum [%s] differs from the reference types in code [%s]", c.op, got, want)
					}
				}
			}
		})
	}
}
