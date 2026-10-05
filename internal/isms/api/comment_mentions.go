package api

import (
	"context"
	"regexp"

	"isms.sh/internal/isms/db"
)

// entityMentionPattern matches a #IDENT reference in a comment body. It is the
// pattern web/src/composables/entityReferenceLinks.js renders as a link, and
// the two must stay in step: a mention is saved as a link (#194) only when the
// page also shows it as one.
var entityMentionPattern = regexp.MustCompile(`#([A-Z][A-Z0-9]*)(?:-(\d+))?\b`)

// mentionPrefixTypes mirrors ENTITY_ROUTES in entityReferenceLinks.js: the
// fixed identifier prefixes and the reference type each one names.
var mentionPrefixTypes = map[string]string{
	"RISK": "risk", "INC": "incident", "TASK": "task", "CA": "corrective_action",
	"SUPPLIER": "supplier", "SYSTEM": "system", "LEGAL": "legal_requirement",
	"CR": "change_request", "ASSET": "asset", "AST": "asset",
}

// entityMention is a #IDENT as typed, with the reference type it would name.
type entityMention struct {
	Type string
	ID   string
}

// parseEntityMentions returns the distinct #IDENT mentions in body, in the
// order they first appear. Like the page, #PREFIX-N with a fixed prefix names
// that register; any other #KEY-N names an objective and a bare #KEY a
// program, which only resolve if that program key exists.
func parseEntityMentions(body string) []entityMention {
	var out []entityMention
	seen := map[entityMention]bool{}
	for _, m := range entityMentionPattern.FindAllStringSubmatch(body, -1) {
		prefix, num := m[1], m[2]
		var em entityMention
		switch {
		case num == "":
			em = entityMention{Type: "program", ID: prefix}
		case mentionPrefixTypes[prefix] != "":
			em = entityMention{Type: mentionPrefixTypes[prefix], ID: prefix + "-" + num}
		default:
			em = entityMention{Type: "objective", ID: prefix + "-" + num}
		}
		if !seen[em] {
			seen[em] = true
			out = append(out, em)
		}
	}
	return out
}

// resolveCommentMentions turns the #IDENT mentions in a comment on
// (subjectType, subjectID) into the links to save for it. Both ends are
// canonicalised the way POST /references stores them. A mention that names
// nothing (a typo, an unknown key) is skipped and stays plain text, as is a
// mention of the subject itself; a subject that cannot carry references
// (e.g. a check-in) yields none.
func (s *Server) resolveCommentMentions(ctx context.Context, orgID int, viewer db.TaskViewer, subjectType, subjectID, body string) []db.CommentReference {
	mentions := parseEntityMentions(body)
	if len(mentions) == 0 {
		return nil
	}
	sourceID, found := s.canonicalReferenceID(ctx, orgID, viewer, subjectType, subjectID)
	if !found {
		return nil
	}
	var refs []db.CommentReference
	seen := map[entityMention]bool{}
	for _, m := range mentions {
		targetID, found := s.canonicalReferenceID(ctx, orgID, viewer, m.Type, m.ID)
		if !found {
			continue
		}
		key := entityMention{Type: m.Type, ID: targetID}
		if seen[key] || (m.Type == subjectType && targetID == sourceID) {
			continue
		}
		seen[key] = true
		refs = append(refs, db.CommentReference{SourceType: subjectType, SourceID: sourceID, TargetType: m.Type, TargetID: targetID})
	}
	return refs
}
