package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"isms.sh/internal/isms/db"
)

// Trust center certifications (#521): the manager endpoints, the upload path
// and the one download helper every file route shares.

// bodyLimitSkips lists the routes that carry a file and therefore set their own
// 50M BodyLimit instead of the global 10M one. They are matched by method and
// by the route pattern echo resolved (c.Path()), never by the raw URL: a prefix
// match on the URL could be spoofed into lifting the cap on any route.
var bodyLimitSkips = map[string]bool{
	"POST /api/v1/checkins/:id/evidence":                true,
	"POST /api/v1/trust-center/certifications":          true,
	"POST /api/v1/trust-center/certifications/:id/file": true,
}

// bodyLimitSkipper reports whether the global body cap steps aside for the
// matched route.
func bodyLimitSkipper(c echo.Context) bool {
	return bodyLimitSkips[c.Request().Method+" "+c.Path()]
}

// globalBodyLimit is the request body cap for every route that is not in
// bodyLimitSkips.
func globalBodyLimit() echo.MiddlewareFunc {
	return middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{Limit: "10M", Skipper: bodyLimitSkipper})
}

// uploadBodyLimit is the per-route cap of the routes in bodyLimitSkips.
func uploadBodyLimit() echo.MiddlewareFunc { return middleware.BodyLimit("50M") }

// trustCertMaxJSON caps a JSON create or update body.
const trustCertMaxJSON = 64 << 10

// trustCertFileTypes maps a sniffed content type to the extension stored and
// served with it. It is the only place a stored type or extension comes from.
var trustCertFileTypes = map[string]string{
	"application/pdf": ".pdf",
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
}

// ---- JSON shapes -----------------------------------------------------------

type trustCertItemJSON struct {
	ID          string `json:"id"` // the item's public id, as in GET /trust-center/items
	AccessLevel string `json:"access_level"`
	SortOrder   int    `json:"sort_order"`
}

// trustCertificationJSON is a certification as the manager endpoints return it.
// Dates are YYYY-MM-DD or null; the blob key never appears.
type trustCertificationJSON struct {
	ID              int64              `json:"id"`
	Label           string             `json:"label"`
	LinkType        string             `json:"link_type"`
	Issuer          string             `json:"issuer"`
	ValidFrom       *string            `json:"valid_from"`
	ValidUntil      *string            `json:"valid_until"`
	State           string             `json:"state"`
	Source          string             `json:"source"`
	URL             string             `json:"url"`
	FileName        string             `json:"file_name"`
	FileContentType string             `json:"file_content_type"`
	FileSize        int64              `json:"file_size"`
	FileSHA256      string             `json:"file_sha256"`
	VerifiedAt      *string            `json:"verified_at"`
	VerifiedBy      string             `json:"verified_by"`
	CreatedBy       string             `json:"created_by"`
	CreatedAt       int64              `json:"created_at"`
	UpdatedAt       int64              `json:"updated_at"`
	Item            *trustCertItemJSON `json:"item"`
}

// trustManagerCertification is the summary carried by a GET /trust-center/items
// row of kind certification.
type trustManagerCertification struct {
	ID         int64   `json:"id"`
	Label      string  `json:"label"`
	LinkType   string  `json:"link_type"`
	Issuer     string  `json:"issuer"`
	ValidFrom  *string `json:"valid_from"`
	ValidUntil *string `json:"valid_until"`
	State      string  `json:"state"`
	Source     string  `json:"source"`
}

func trustCertSource(c *db.TrustCertification) string {
	if c.FileKey != "" {
		return "file"
	}
	return "link"
}

func trustCertificationResponse(c *db.TrustCertification) trustCertificationJSON {
	out := trustCertificationJSON{
		ID: c.ID, Label: c.Label, LinkType: c.LinkType, Issuer: c.Issuer,
		ValidFrom: c.ValidFrom, ValidUntil: c.ValidUntil, State: c.State, Source: trustCertSource(c),
		URL: c.URL, FileName: c.FileName, FileContentType: c.FileContentType, FileSize: c.FileSize,
		FileSHA256: c.FileSHA256, VerifiedAt: c.VerifiedAt, VerifiedBy: c.VerifiedBy,
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt.Unix(), UpdatedAt: c.UpdatedAt.Unix(),
	}
	if c.Item != nil {
		out.Item = &trustCertItemJSON{ID: c.Item.PublicID, AccessLevel: c.Item.AccessLevel, SortOrder: c.Item.SortOrder}
	}
	return out
}

func trustManagerCertificationFor(c *db.TrustCertification) *trustManagerCertification {
	return &trustManagerCertification{
		ID: c.ID, Label: c.Label, LinkType: c.LinkType, Issuer: c.Issuer,
		ValidFrom: c.ValidFrom, ValidUntil: c.ValidUntil, State: c.State, Source: trustCertSource(c),
	}
}

// ---- request parsing -------------------------------------------------------

// trustDate is a JSON date field: absent leaves Set false, null leaves Value
// nil, and a value is kept raw so that anything but a YYYY-MM-DD string (a
// number, an object) is answered with trust_date_invalid naming the field
// instead of failing the whole body as a malformed request.
type trustDate = Optional[json.RawMessage]

// resolveTrustDate returns the date of a field that is present: nil for null,
// else the strictly parsed day. An empty string is not null.
func resolveTrustDate(d trustDate, field string) (*string, error) {
	if d.Value == nil {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(*d.Value, &s); err != nil {
		return nil, apiError(http.StatusBadRequest, CodeTrustDateInvalid, Field(field))
	}
	v, err := db.ParseTrustDate(s)
	if err != nil {
		return nil, apiError(http.StatusBadRequest, CodeTrustDateInvalid, Field(field))
	}
	return &v, nil
}

// formDate reads a multipart date field. Browsers send "" for an empty date
// input, so an empty field means no date; anything else must be YYYY-MM-DD.
func formDate(c echo.Context, field string) (*string, error) {
	raw := c.FormValue(field)
	if raw == "" {
		return nil, nil
	}
	v, err := db.ParseTrustDate(raw)
	if err != nil {
		return nil, apiError(http.StatusBadRequest, CodeTrustDateInvalid, Field(field))
	}
	return &v, nil
}

type trustCertificationCreateRequest struct {
	Label       string    `json:"label"`
	LinkType    string    `json:"link_type"`
	Issuer      string    `json:"issuer"`
	URL         string    `json:"url"`
	ValidFrom   trustDate `json:"valid_from"`
	ValidUntil  trustDate `json:"valid_until"`
	VerifiedAt  trustDate `json:"verified_at"`
	Publish     bool      `json:"publish"`
	AccessLevel string    `json:"access_level"`
}

type trustCertificationUpdateRequest struct {
	Label      *string   `json:"label"`
	LinkType   *string   `json:"link_type"`
	Issuer     *string   `json:"issuer"`
	URL        *string   `json:"url"`
	ValidFrom  trustDate `json:"valid_from"`
	ValidUntil trustDate `json:"valid_until"`
	VerifiedAt trustDate `json:"verified_at"`
}

type trustCertificationPublishRequest struct {
	AccessLevel string `json:"access_level"`
}

// bindTrustJSON reads a small JSON body.
func bindTrustJSON(c echo.Context, v any) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, trustCertMaxJSON)
	// An empty body is an empty request; the handler then reports what is missing.
	if err := json.NewDecoder(c.Request().Body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	return nil
}

func isMultipart(c echo.Context) bool {
	return strings.HasPrefix(strings.ToLower(c.Request().Header.Get(echo.HeaderContentType)), "multipart/form-data")
}

// trustCertError maps a db error to the API's answer.
func trustCertError(c echo.Context, what string, err error) error {
	switch {
	case errors.Is(err, db.ErrTrustCertificationNotFound):
		return errNotFound("certification")
	case errors.Is(err, db.ErrTrustItemNotFound):
		return errNotFound("item")
	case errors.Is(err, db.ErrTrustItemAlreadyPublished):
		return apiError(http.StatusConflict, CodeTrustCertificationAlreadyPublished)
	case errors.Is(err, db.ErrTrustCertificationSource):
		return apiError(http.StatusBadRequest, CodeTrustCertificationSource)
	case errors.Is(err, db.ErrTrustCenterInvalid):
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	return trustInternalError(c, what, err)
}

func trustCertID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errInvalidEntityID("certification")
	}
	return id, nil
}

// trustCertURL validates the url of a link certification (http or https only).
func trustCertURL(raw string) (string, error) {
	u, ok := validateExternalLinkURL(raw)
	if !ok {
		return "", apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	return u, nil
}

// ---- upload ----------------------------------------------------------------

// sanitizeUploadName keeps the display name of an upload: the last path
// element, no control characters, at most 255 characters.
func sanitizeUploadName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > 255 {
		name = string(r[:255])
	}
	if name == "" {
		name = "certificate"
	}
	return name
}

// storeTrustCertFile checks an uploaded file by its bytes and writes it to the
// blob store. The first 512 bytes are sniffed: only PDF, PNG and JPEG pass,
// whatever the client's Content-Type or file name say. The stored content type,
// extension and key come from the sniffed type alone. The file is read as a
// stream (hash, rewind, copy), never into memory. A rejected file writes
// nothing.
func (s *Server) storeTrustCertFile(ctx context.Context, org *db.Organization, fh *multipart.FileHeader) (*db.TrustCertificationFile, error) {
	src, err := fh.Open()
	if err != nil {
		return nil, apiError(http.StatusBadRequest, CodeTrustFileRequired)
	}
	defer src.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, trustUploadReadError(err)
	}
	ctype := http.DetectContentType(head[:n])
	ext, ok := trustCertFileTypes[ctype]
	if n == 0 || !ok {
		return nil, apiError(http.StatusBadRequest, CodeTrustFileType)
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewinding upload: %w", err)
	}
	h := sha256.New()
	size, err := io.Copy(h, src)
	if err != nil {
		return nil, trustUploadReadError(err)
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewinding upload: %w", err)
	}
	key := "trust/certifications/" + uuid.NewString() + ext
	if err := s.blobs.PutStream(ctx, org.UUID, key, ctype, src); err != nil {
		// A write that failed part-way may have left bytes under the key.
		s.deleteTrustBlob(ctx, org, key)
		return nil, fmt.Errorf("storing certification file: %w", err)
	}
	return &db.TrustCertificationFile{
		Key: key, Name: sanitizeUploadName(fh.Filename), ContentType: ctype,
		Size: size, SHA256: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

// trustUploadReadError passes a "request body too large" from the body limit
// through as the 413 it is.
func trustUploadReadError(err error) error {
	var he *echo.HTTPError
	if errors.As(err, &he) && he.Code == http.StatusRequestEntityTooLarge {
		return he
	}
	return fmt.Errorf("reading upload: %w", err)
}

// isBodyTooLarge reports whether err is the body limit cutting a read short.
func isBodyTooLarge(err error) bool {
	var he *echo.HTTPError
	if errors.As(err, &he) && he.Code == http.StatusRequestEntityTooLarge {
		return true
	}
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// parseUploadForm parses the multipart body with a small in-memory budget, so
// file parts spill to temporary files instead of living in memory (Go's default
// is 32 MB, which FormValue or FormFile would use). It must run before any
// FormValue / FormFile. Only the body limit being hit is an error here (the 413,
// also for a chunked body with no Content-Length); a body that is simply not
// multipart is left for the caller's own "no file" answer.
func parseUploadForm(c echo.Context) error {
	if err := c.Request().ParseMultipartForm(1 << 20); err != nil && isBodyTooLarge(err) {
		return echo.ErrStatusRequestEntityTooLarge
	}
	return nil
}

// uploadedFile returns the multipart "file" field. A body the limit cut short
// is the 413, not a missing file.
func uploadedFile(c echo.Context) (*multipart.FileHeader, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		if isBodyTooLarge(err) {
			return nil, echo.ErrStatusRequestEntityTooLarge
		}
		return nil, apiError(http.StatusBadRequest, CodeTrustFileRequired)
	}
	return fh, nil
}

// optionalUploadedFile is uploadedFile for a request where the file part may be
// absent (a link certification): absent is nil, not an error.
func optionalUploadedFile(c echo.Context) (*multipart.FileHeader, error) {
	if _, err := c.FormFile("file"); errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	return uploadedFile(c)
}

// deleteTrustBlob removes a blob after the database no longer points at it. A
// failure is logged: the certification is already gone and the caller has
// nothing to retry.
//
// It runs on a context that is not cancelled with the request: a client that
// disconnects must not leave the blob behind.
func (s *Server) deleteTrustBlob(ctx context.Context, org *db.Organization, key string) {
	if key == "" {
		return
	}
	if err := s.blobs.Delete(context.WithoutCancel(ctx), org.UUID, key); err != nil {
		log.Printf("trust center: org %d: deleting blob %s: %v", org.ID, key, err)
	}
}

// releaseTrustBlob deletes a blob stored for a write that returned err, unless
// the database now references it. An error from a commit can mean the write
// happened, so the row is read back (on a context that is not cancelled) and the
// blob is kept when it is in use; if that cannot be read either, the blob is
// kept and logged rather than risk deleting a file a row points at.
func (s *Server) releaseTrustBlob(ctx context.Context, org *db.Organization, key string) {
	bg := context.WithoutCancel(ctx)
	used, err := s.db.TrustCertificationFileReferenced(bg, org.ID, key)
	if err != nil {
		log.Printf("trust center: org %d: keeping blob %s, cannot tell whether it is referenced: %v", org.ID, key, err)
		return
	}
	if !used {
		s.deleteTrustBlob(bg, org, key)
	}
}

// createTrustCertification stores the file (when there is one) and then the
// row. A failed insert deletes the blob it just wrote, so nothing is orphaned.
func (s *Server) createTrustCertification(ctx context.Context, org *db.Organization, in db.TrustCertificationInput, fh *multipart.FileHeader, actor string) (*db.TrustCertification, error) {
	if fh != nil {
		f, err := s.storeTrustCertFile(ctx, org, fh)
		if err != nil {
			return nil, err
		}
		in.File = f
	}
	cert, err := s.db.CreateTrustCertification(ctx, org.ID, in, actor)
	if err != nil {
		if in.File != nil {
			s.releaseTrustBlob(ctx, org, in.File.Key)
		}
		return nil, err
	}
	return cert, nil
}

// ---- download --------------------------------------------------------------

// errTrustFileGone: the certification row says there is a file but the blob
// store has none.
var errTrustFileGone = errors.New("trust center certification file is missing from the blob store")

// trustDownloadName is the name a file is offered under: the stored display
// name with its extension forced to match the stored type, so a name cannot
// claim a type the bytes are not. Control characters are removed.
func trustDownloadName(name, contentType string) string {
	name = sanitizeUploadName(name)
	want := trustCertFileTypes[contentType]
	lower := strings.ToLower(name)
	ok := strings.HasSuffix(lower, want)
	if contentType == "image/jpeg" && strings.HasSuffix(lower, ".jpeg") {
		ok = true
	}
	if !ok && want != "" {
		name += want
	}
	return name
}

// serveTrustCertFile streams a certification's file through the application on
// every backend, never as a redirect or a presigned URL, so the caller's access
// decision is the only way to the bytes. The response is an attachment of the
// stored (sniffed) type, uncacheable, and not sniffable.
func (s *Server) serveTrustCertFile(c echo.Context, org *db.Organization, cert *db.TrustCertification) error {
	ctx := c.Request().Context()
	rc, size, err := s.blobs.Open(ctx, org.UUID, cert.FileKey)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errTrustFileGone
		}
		return fmt.Errorf("opening certification file: %w", err)
	}
	defer rc.Close()
	h := c.Response().Header()
	disp := mime.FormatMediaType("attachment", map[string]string{"filename": trustDownloadName(cert.FileName, cert.FileContentType)})
	if disp == "" {
		disp = "attachment"
	}
	h.Set("Content-Disposition", disp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	if size > 0 {
		h.Set("Content-Length", strconv.FormatInt(size, 10))
	}
	return c.Stream(http.StatusOK, cert.FileContentType, rc)
}

// trustPublicCertificationFile answers a visitor's file request, and the
// manager preview's: the same checks for both. The certification must be
// published, not hidden, not past valid_until, have a file, and be at access
// level public; every other case is the same 404. Gated and restricted files
// are readable by managers only, through the manager route, until visitor
// sign-in exists.
func (s *Server) trustPublicCertificationFile(c echo.Context, org *db.Organization, publicID string) error {
	cert, err := s.db.GetServableTrustCertification(c.Request().Context(), org.ID, strings.ToLower(publicID))
	if errors.Is(err, db.ErrTrustCertificationNotFound) {
		return trustPublicNotFound(c)
	}
	if err != nil {
		return trustPublicError(c, err)
	}
	if cert.Item == nil || cert.Item.AccessLevel != db.TrustAccessPublic || cert.FileKey == "" {
		return trustPublicNotFound(c)
	}
	if err := s.serveTrustCertFile(c, org, cert); err != nil {
		if errors.Is(err, errTrustFileGone) {
			log.Printf("trust center: org %d certification %d: %v", org.ID, cert.ID, err)
			return trustPublicNotFound(c)
		}
		return trustPublicError(c, err)
	}
	return nil
}

// ---- manager endpoints -----------------------------------------------------

// GET /api/v1/trust-center/certifications
func (s *Server) handleListTrustCertifications(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	certs, err := s.db.ListTrustCertifications(c.Request().Context(), getOrgID(c))
	if err != nil {
		return trustInternalError(c, "listing certifications", err)
	}
	out := make([]trustCertificationJSON, 0, len(certs))
	for i := range certs {
		out = append(out, trustCertificationResponse(&certs[i]))
	}
	return c.JSON(http.StatusOK, out)
}

// POST /api/v1/trust-center/certifications takes multipart (fields and a
// "file") or JSON (fields and a "url"). With publish true it also publishes the
// new certification; if that step fails the certification stays, unpublished.
func (s *Server) handleCreateTrustCertification(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	var in db.TrustCertificationInput
	var fh *multipart.FileHeader
	var publish bool
	var level string
	if isMultipart(c) {
		if err := parseUploadForm(c); err != nil {
			return err
		}
		var err error
		if in.ValidFrom, err = formDate(c, "valid_from"); err != nil {
			return err
		}
		if in.ValidUntil, err = formDate(c, "valid_until"); err != nil {
			return err
		}
		if in.VerifiedAt, err = formDate(c, "verified_at"); err != nil {
			return err
		}
		in.Label, in.LinkType, in.Issuer = c.FormValue("label"), c.FormValue("link_type"), c.FormValue("issuer")
		in.URL = strings.TrimSpace(c.FormValue("url"))
		level = c.FormValue("access_level")
		switch c.FormValue("publish") {
		case "true":
			publish = true
		case "", "false":
		default:
			return apiError(http.StatusBadRequest, CodeInvalidRequest)
		}
		if fh, err = optionalUploadedFile(c); err != nil {
			return err
		}
	} else {
		var req trustCertificationCreateRequest
		if err := bindTrustJSON(c, &req); err != nil {
			return err
		}
		var err error
		if in.ValidFrom, err = resolveTrustDate(req.ValidFrom, "valid_from"); err != nil {
			return err
		}
		if in.ValidUntil, err = resolveTrustDate(req.ValidUntil, "valid_until"); err != nil {
			return err
		}
		if in.VerifiedAt, err = resolveTrustDate(req.VerifiedAt, "verified_at"); err != nil {
			return err
		}
		in.Label, in.LinkType, in.Issuer, in.URL = req.Label, req.LinkType, req.Issuer, strings.TrimSpace(req.URL)
		publish, level = req.Publish, req.AccessLevel
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" {
		return errRequired("label")
	}
	if (in.URL != "") == (fh != nil) {
		return apiError(http.StatusBadRequest, CodeTrustCertificationSource)
	}
	if in.URL != "" {
		u, err := trustCertURL(in.URL)
		if err != nil {
			return err
		}
		in.URL = u
	}
	if err := db.ValidateTrustCertificationInput(in); err != nil {
		return trustCertError(c, "validating certification", err)
	}
	// The level is checked even when nothing is published, so a bad value is
	// not silently ignored.
	if publish {
		var err error
		if level, err = s.trustCertLevel(c, level); err != nil {
			return err
		}
	} else if level != "" {
		if _, err := s.trustCertLevel(c, level); err != nil {
			return err
		}
	}

	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return trustInternalError(c, "reading organization", err)
	}
	cert, err := s.createTrustCertification(ctx, org, in, fh, actor)
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		return trustCertError(c, "creating certification", err)
	}
	if publish {
		if _, err := s.db.PublishTrustCertification(ctx, orgID, cert.ID, level, actor); err != nil {
			return trustCertError(c, "publishing certification", err)
		}
		if cert, err = s.db.GetTrustCertification(ctx, orgID, cert.ID); err != nil {
			return trustCertError(c, "reading certification", err)
		}
	}
	return c.JSON(http.StatusCreated, trustCertificationResponse(cert))
}

// trustCertLevel resolves the access level of a publish request: the one given,
// else the organization's default.
func (s *Server) trustCertLevel(c echo.Context, level string) (string, error) {
	if level == "" {
		settings, err := s.db.GetTrustCenterSettings(c.Request().Context(), getOrgID(c))
		if err != nil {
			return "", trustInternalError(c, "reading settings", err)
		}
		return settings.DefaultAccessLevel, nil
	}
	switch level {
	case db.TrustAccessPublic, db.TrustAccessGated, db.TrustAccessRestricted:
		return level, nil
	}
	return "", apiError(http.StatusBadRequest, CodeInvalidRequest)
}

// PUT /api/v1/trust-center/certifications/:id is a partial update: an absent
// key leaves the field alone, null clears a date.
func (s *Server) handleUpdateTrustCertification(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := trustCertID(c)
	if err != nil {
		return err
	}
	var req trustCertificationUpdateRequest
	if err := bindTrustJSON(c, &req); err != nil {
		return err
	}
	// The request is turned into a patch and merged into the row by the database
	// under its lock, so a concurrent update of other fields is never undone.
	patch := db.UpdateTrustCertificationPatch{LinkType: req.LinkType, Issuer: req.Issuer}
	if req.Label != nil {
		label := strings.TrimSpace(*req.Label)
		if label == "" {
			return errRequired("label")
		}
		patch.Label = &label
	}
	for _, f := range []struct {
		name string
		src  trustDate
		dst  *db.OptionalTrustDate
	}{
		{"valid_from", req.ValidFrom, &patch.ValidFrom},
		{"valid_until", req.ValidUntil, &patch.ValidUntil},
		{"verified_at", req.VerifiedAt, &patch.VerifiedAt},
	} {
		if !f.src.Set {
			continue
		}
		v, err := resolveTrustDate(f.src, f.name)
		if err != nil {
			return err
		}
		*f.dst = db.OptionalTrustDate{Set: true, Value: v}
	}
	if req.URL != nil {
		u, err := trustCertURL(strings.TrimSpace(*req.URL))
		if err != nil {
			return err
		}
		patch.URL = &u
	}
	cert, err := s.db.PatchTrustCertification(ctx, orgID, id, patch, getUserEmail(c))
	if err != nil {
		return trustCertError(c, "updating certification", err)
	}
	return c.JSON(http.StatusOK, trustCertificationResponse(cert))
}

// POST /api/v1/trust-center/certifications/:id/file replaces the file of a file
// certification. The old blob is deleted after the new one is recorded.
func (s *Server) handleReplaceTrustCertificationFile(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := trustCertID(c)
	if err != nil {
		return err
	}
	cur, err := s.db.GetTrustCertification(ctx, orgID, id)
	if err != nil {
		return trustCertError(c, "reading certification", err)
	}
	if cur.FileKey == "" {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	if err := parseUploadForm(c); err != nil {
		return err
	}
	fh, err := uploadedFile(c)
	if err != nil {
		return err
	}
	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return trustInternalError(c, "reading organization", err)
	}
	f, err := s.storeTrustCertFile(ctx, org, fh)
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		return trustInternalError(c, "storing certification file", err)
	}
	cert, oldKey, err := s.db.ReplaceTrustCertificationFile(ctx, orgID, id, *f, getUserEmail(c))
	if err != nil {
		s.releaseTrustBlob(ctx, org, f.Key)
		return trustCertError(c, "replacing certification file", err)
	}
	s.deleteTrustBlob(ctx, org, oldKey)
	return c.JSON(http.StatusOK, trustCertificationResponse(cert))
}

// POST /api/v1/trust-center/certifications/:id/verify
func (s *Server) handleVerifyTrustCertification(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	id, err := trustCertID(c)
	if err != nil {
		return err
	}
	cert, err := s.db.VerifyTrustCertification(c.Request().Context(), getOrgID(c), id, getUserEmail(c))
	if err != nil {
		return trustCertError(c, "verifying certification", err)
	}
	return c.JSON(http.StatusOK, trustCertificationResponse(cert))
}

// DELETE /api/v1/trust-center/certifications/:id removes the certification, its
// item and, after the commit, its file.
func (s *Server) handleDeleteTrustCertification(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := trustCertID(c)
	if err != nil {
		return err
	}
	// Read before the delete, so the blob can always be cleaned after the commit.
	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return trustInternalError(c, "reading organization", err)
	}
	key, err := s.db.DeleteTrustCertification(ctx, orgID, id, getUserEmail(c))
	if err != nil {
		return trustCertError(c, "deleting certification", err)
	}
	s.deleteTrustBlob(ctx, org, key)
	return c.NoContent(http.StatusNoContent)
}

// POST /api/v1/trust-center/certifications/:id/publish; access_level is
// optional and defaults to the organization's default.
func (s *Server) handlePublishTrustCertification(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := trustCertID(c)
	if err != nil {
		return err
	}
	var req trustCertificationPublishRequest
	if err := bindTrustJSON(c, &req); err != nil {
		return err
	}
	level, err := s.trustCertLevel(c, req.AccessLevel)
	if err != nil {
		return err
	}
	if _, err := s.db.PublishTrustCertification(ctx, orgID, id, level, getUserEmail(c)); err != nil {
		return trustCertError(c, "publishing certification", err)
	}
	cert, err := s.db.GetTrustCertification(ctx, orgID, id)
	if err != nil {
		return trustCertError(c, "reading certification", err)
	}
	return c.JSON(http.StatusCreated, trustCertificationResponse(cert))
}

// DELETE /api/v1/trust-center/certifications/:id/publish unpublishes and keeps
// the certification.
func (s *Server) handleUnpublishTrustCertification(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	id, err := trustCertID(c)
	if err != nil {
		return err
	}
	if err := s.db.UnpublishTrustCertification(c.Request().Context(), getOrgID(c), id, getUserEmail(c)); err != nil {
		return trustCertError(c, "unpublishing certification", err)
	}
	return c.NoContent(http.StatusNoContent)
}

// GET /api/v1/trust-center/certifications/:id/file downloads a certification's
// file at any access level and any validity, for managers.
func (s *Server) handleDownloadTrustCertificationFile(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := trustCertID(c)
	if err != nil {
		return err
	}
	cert, err := s.db.GetTrustCertification(ctx, orgID, id)
	if err != nil {
		return trustCertError(c, "reading certification", err)
	}
	if cert.FileKey == "" {
		return errNotFound("certification")
	}
	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return trustInternalError(c, "reading organization", err)
	}
	if err := s.serveTrustCertFile(c, org, cert); err != nil {
		if errors.Is(err, errTrustFileGone) {
			return errNotFound("certification")
		}
		return trustInternalError(c, "streaming certification file", err)
	}
	return nil
}

// GET /api/v1/trust-center/preview/certifications/:id/file answers exactly what
// the public file route would for the session's organization, ignoring whether
// the trust center is switched on.
func (s *Server) handleTrustPreviewCertificationFile(c echo.Context) error {
	org, _, err := s.trustPreviewOrg(c)
	if err != nil {
		return err
	}
	return s.trustPublicCertificationFile(c, org, c.Param("id"))
}
