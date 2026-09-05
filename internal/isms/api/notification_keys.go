package api

// Wire keys for stored notifications, declared once.
//
// Every keyed notification write in this package MUST use a constant from this
// file rather than a string literal. That is not style — it is what makes the
// key set enumerable, which is what NotificationKeys and its test need to
// exist at all.
//
// Why enumerable matters here (plan 82 step 7): useNotificationRender.js falls
// back to the stored English title when the catalogue has no message for a
// key. That fallback is what keeps legacy rows, agent rows and a lagging
// locale safe, and it is also what makes a MISSING translation look exactly
// like correct behaviour — no error, no warning, and a diff that reads fine to
// an English-speaking reviewer. It is wrong only for a non-English user,
// silently, in production. So the invariant "every key Go emits has a frame in
// web/src/locales/en/notifications.json" has to be checked mechanically.
//
// The alternative was grepping the Go source for `"notifications.…"` literals.
// That undercounts: the three `_with_note` variants used to be built by
// `bodyKey += "_with_note"` at four call sites, so a regex over quoted strings
// returned 22 of 25 keys and reported a clean bill of health while three body
// frames went untranslated. Declaring them removes the extraction problem
// instead of solving it, and turns "add a notification" into "add a constant"
// — which the test picks up with no further work.
//
// These strings are FROZEN. They are in `notifications.title_key` /
// `body_key` on rows already written, so a rename orphans those rows to the
// English fallback forever. Rename the catalogue nesting if you must; never a
// wire key.
//
// Note a title wire key is also the parent of its body wire keys
// (`notifications.ca_resolved` and `notifications.ca_resolved.body`), which no
// single JSON node can be. The catalogue therefore nests uniformly and the
// client maps wire key onto catalogue key; see catalogKey in
// useNotificationRender.js and its twin in notification_keys_test.go.
const (
	// Mentions — title only. The body is the comment snippet, org-authored
	// text that must never be translated, so these have no body key.
	NotifyKeyMentionComment       = "notifications.mention_comment"
	NotifyKeyMentionReviewComment = "notifications.mention_review_comment"
	NotifyKeyMentionChangeRequest = "notifications.mention_change_request"

	// Review lifecycle. Each body has a with-note variant: the "Note:" label
	// is product copy the frame owns, so an optional `note` param in one frame
	// would either dangle the label or lose its translation. `note` appears in
	// params only for the _with_note variant.
	NotifyKeyReviewForwarded             = "notifications.review_forwarded"
	NotifyKeyReviewForwardedBody         = "notifications.review_forwarded.body"
	NotifyKeyReviewForwardedBodyWithNote = "notifications.review_forwarded.body_with_note"

	NotifyKeyReviewRequested             = "notifications.review_requested"
	NotifyKeyReviewRequestedBody         = "notifications.review_requested.body"
	NotifyKeyReviewRequestedBodyWithNote = "notifications.review_requested.body_with_note"

	NotifyKeyReviewResubmitted             = "notifications.review_resubmitted"
	NotifyKeyReviewResubmittedBody         = "notifications.review_resubmitted.body"
	NotifyKeyReviewResubmittedBodyWithNote = "notifications.review_resubmitted.body_with_note"

	NotifyKeyAIReviewEscalated     = "notifications.ai_review_escalated"
	NotifyKeyAIReviewEscalatedBody = "notifications.ai_review_escalated.body"

	// Corrective actions. ca_assigned has no body key: the body is the
	// action's own description, org-authored.
	NotifyKeyCAAssigned     = "notifications.ca_assigned"
	NotifyKeyCAResolved     = "notifications.ca_resolved"
	NotifyKeyCAResolvedBody = "notifications.ca_resolved.body"

	// Suggestions. suggestion_resolved is one shared title frame with `action`
	// as a translatable param, while applied and rejected say genuinely
	// different things and so carry their own body frames — hence a title with
	// no body and two bodies with no title.
	NotifyKeySuggestionNew          = "notifications.suggestion_new"
	NotifyKeySuggestionNewBody      = "notifications.suggestion_new.body"
	NotifyKeySuggestionResolved     = "notifications.suggestion_resolved"
	NotifyKeySuggestionAppliedBody  = "notifications.suggestion_applied.body"
	NotifyKeySuggestionRejectedBody = "notifications.suggestion_rejected.body"

	// Incidents. incident_new has no body key: the body is the reporter's own
	// description.
	NotifyKeyIncidentNew        = "notifications.incident_new"
	NotifyKeyIncidentStatus     = "notifications.incident_status"
	NotifyKeyIncidentStatusBody = "notifications.incident_status.body"
)

// NotificationKeys is every wire key this build can write. Adding a constant
// above without adding it here makes TestNotificationKeysIsExhaustive fail,
// so the slice cannot silently fall behind the constants it indexes.
var NotificationKeys = []string{
	NotifyKeyMentionComment,
	NotifyKeyMentionReviewComment,
	NotifyKeyMentionChangeRequest,

	NotifyKeyReviewForwarded,
	NotifyKeyReviewForwardedBody,
	NotifyKeyReviewForwardedBodyWithNote,

	NotifyKeyReviewRequested,
	NotifyKeyReviewRequestedBody,
	NotifyKeyReviewRequestedBodyWithNote,

	NotifyKeyReviewResubmitted,
	NotifyKeyReviewResubmittedBody,
	NotifyKeyReviewResubmittedBodyWithNote,

	NotifyKeyAIReviewEscalated,
	NotifyKeyAIReviewEscalatedBody,

	NotifyKeyCAAssigned,
	NotifyKeyCAResolved,
	NotifyKeyCAResolvedBody,

	NotifyKeySuggestionNew,
	NotifyKeySuggestionNewBody,
	NotifyKeySuggestionResolved,
	NotifyKeySuggestionAppliedBody,
	NotifyKeySuggestionRejectedBody,

	NotifyKeyIncidentNew,
	NotifyKeyIncidentStatus,
	NotifyKeyIncidentStatusBody,
}

// NotificationKeyParams is the set of param names each wire key's call sites
// can send, declared per key rather than package-wide.
//
// The closed set in internal/isms/db is a vocabulary, not a contract: it says
// `reason` is a name the backend knows, not that the incident_status frame is
// ever handed one. Checking frames against the vocabulary alone lets a
// translator (or a copy edit) add {reason} to incident_status in every locale
// and pass every test, while the call site sends only status, title and id —
// so the slot renders empty and the sentence quietly loses a clause. Checking
// against this table instead asks the question that matters: is this slot
// filled *for this notification*?
//
// A title and its body share one params map at the write site, so both are
// listed from that same map. Where a key is written from several sites, the
// entry is the union of what those sites can send — the with-note bodies get
// `note` and the plain ones do not, because the call sites pick the variant by
// whether a note exists.
//
// Hand-maintained, and deliberately so: params are inline map literals built
// conditionally at each call site (`if req.Message != "" { params["note"] = … }`),
// which no static extraction can enumerate. What keeps the table honest is
// TestNotificationKeyParamsIsExhaustive — its key set is pinned to
// NotificationKeys, so a new notification cannot ship without an entry. The
// remaining risk is an entry that names more than its call site sends, which
// is the direction that fails safe: it only widens what a frame may ask for.
// The call-site half stays a runtime concern (see keyedColumns in
// internal/isms/db/notifications.go, which logs an out-of-set param).
var NotificationKeyParams = map[string][]string{
	NotifyKeyMentionComment:       {"actor"},
	NotifyKeyMentionReviewComment: {"actor"},
	NotifyKeyMentionChangeRequest: {"actor"},

	NotifyKeyReviewForwarded:             {"actor", "doc_id", "title", "version", "note"},
	NotifyKeyReviewForwardedBody:         {"actor", "doc_id", "title", "version"},
	NotifyKeyReviewForwardedBodyWithNote: {"actor", "doc_id", "title", "version", "note"},

	NotifyKeyReviewRequested:             {"actor", "title", "version", "note"},
	NotifyKeyReviewRequestedBody:         {"actor", "title", "version"},
	NotifyKeyReviewRequestedBodyWithNote: {"actor", "title", "version", "note"},

	NotifyKeyReviewResubmitted:             {"actor", "doc_id", "title", "version", "note"},
	NotifyKeyReviewResubmittedBody:         {"actor", "doc_id", "title", "version"},
	NotifyKeyReviewResubmittedBodyWithNote: {"actor", "doc_id", "title", "version", "note"},

	NotifyKeyAIReviewEscalated:     {"doc_id", "round"},
	NotifyKeyAIReviewEscalatedBody: {"doc_id", "round"},

	NotifyKeyCAAssigned:     {"title"},
	NotifyKeyCAResolved:     {"title", "id", "actor"},
	NotifyKeyCAResolvedBody: {"title", "id", "actor"},

	// suggestion_resolved is one title frame shared by the applied and
	// rejected sites, so its entry is the union of both param maps.
	NotifyKeySuggestionNew:          {"actor", "title", "suggestion_type", "entity"},
	NotifyKeySuggestionNewBody:      {"actor", "title", "suggestion_type", "entity"},
	NotifyKeySuggestionResolved:     {"action", "title", "actor", "entity", "id", "reason"},
	NotifyKeySuggestionAppliedBody:  {"action", "title", "actor", "entity", "id"},
	NotifyKeySuggestionRejectedBody: {"action", "title", "actor", "reason"},

	NotifyKeyIncidentNew:        {"severity", "title"},
	NotifyKeyIncidentStatus:     {"status", "title", "id"},
	NotifyKeyIncidentStatusBody: {"status", "title", "id"},
}
