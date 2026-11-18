package api

import (
	"errors"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/export"
	"isms.sh/internal/isms/license"
	"isms.sh/internal/isms/store"
)

// maxExportLogo caps the organisation logo read for an export; a larger logo
// is skipped rather than printed.
const maxExportLogo = 2 << 20

func (s *Server) exportFormats() []string {
	if s.exportCaps != nil {
		return s.exportCaps().Formats()
	}
	return license.Current().Formats()
}

func (s *Server) renderExport(format string, d *export.ExportDoc) ([]byte, error) {
	if s.exportRender != nil {
		return s.exportRender(format, d)
	}
	return export.Render(format, d)
}

var exportNameUnsafe = regexp.MustCompile(`[^a-z0-9._-]`)

// exportFilename is "<doc-id>-v<version>.<ext>", "<doc-id>-<date>.<ext>" for a
// version without a number, or "<doc-id>.<ext>", reduced to [a-z0-9._-].
func exportFilename(docID, version, date, ext string) string {
	name := docID
	switch {
	case version != "":
		name += "-v" + version
	case date != "":
		name += "-" + date
	}
	name = exportNameUnsafe.ReplaceAllString(strings.ToLower(name), "-")
	return name + "." + ext
}

// GET /api/v1/documents/:docId/export?format=pdf|docx[&version=<v>|current]
//
// Anyone who can read the document can export it: the same access as
// /documents/:docId/body, and no stricter. The format must be licensed on this
// server. The default version is the working copy (HEAD); an explicit version
// is read from the commit recorded for it, so the draft stamp follows that
// version and not HEAD.
func (s *Server) handleExportDocument(c echo.Context) error {
	ctx := c.Request().Context()
	orgID := getOrgID(c)
	docID := c.Param("docId")

	format := c.QueryParam("format")
	if format != "pdf" && format != "docx" {
		return apiError(http.StatusBadRequest, CodeExportFormatInvalid)
	}
	licensed := false
	for _, f := range s.exportFormats() {
		if f == format {
			licensed = true
		}
	}
	if !licensed {
		return apiError(http.StatusForbidden, CodeExportNotLicensed, Value(strings.ToUpper(format)))
	}

	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	path := st.FindDocumentByID(docID)
	if path == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document not found: "+docID)
	}

	// A version is addressed by its label (`version`) or its row id (`version_id`).
	// An empty label, which every template document's first approval has, can only
	// be addressed by id: "" means the current working copy.
	version := strings.TrimSpace(c.QueryParam("version"))
	idParam := strings.TrimSpace(c.QueryParam("version_id"))
	if idParam != "" && version != "" {
		return apiError(http.StatusBadRequest, CodeExportVersionAmbiguous)
	}
	versionID := 0
	if idParam != "" {
		n, err := strconv.Atoi(idParam)
		if err != nil || n <= 0 {
			return apiError(http.StatusBadRequest, CodeExportVersionInvalid)
		}
		versionID = n
	}
	var pf *store.DocumentFile
	var row *db.DocumentVersion // the recorded version being exported, if any
	if versionID == 0 && (version == "" || version == "current") {
		version = ""
		pf, err = st.LoadDocument(path)
		if err != nil {
			log.Printf("export %s: loading document: %v", docID, err)
			return apiError(http.StatusInternalServerError, CodeExportFailed)
		}
	} else {
		var v *db.DocumentVersion
		var err error
		if versionID != 0 {
			v, err = s.db.GetVersionByID(ctx, orgID, docID, versionID)
		} else {
			v, err = s.db.GetVersion(ctx, orgID, docID, version)
		}
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apiError(http.StatusNotFound, CodeExportVersionNotFound)
			}
			log.Printf("export %s: looking up version %s%d: %v", docID, version, versionID, err)
			return apiError(http.StatusInternalServerError, CodeExportFailed)
		}
		row = v
		raw, err := st.ReadFileAtRef(v.CommitHash, v.FilePath)
		if err != nil {
			log.Printf("export %s: reading version %d: %v", docID, v.ID, err)
			return apiError(http.StatusInternalServerError, CodeExportFailed)
		}
		if pf, err = store.ParseDocument(v.FilePath, raw); err != nil {
			log.Printf("export %s: parsing version %d: %v", docID, v.ID, err)
			return apiError(http.StatusInternalServerError, CodeExportFailed)
		}
	}

	blocks, err := export.Parse([]byte(pf.Body))
	if err != nil {
		log.Printf("export %s: parsing body: %v", docID, err)
		return apiError(http.StatusInternalServerError, CodeExportFailed)
	}
	d := export.ExportDoc{
		Meta:         pf.Frontmatter,
		VersionLabel: pf.Frontmatter.Version,
		GeneratedAt:  time.Now().UTC(),
		Blocks:       blocks,
	}
	if d.VersionLabel == "" {
		d.VersionLabel = version
	}
	// A recorded version whose document carries no version number is named by the
	// date it was recorded (UTC), in the file name and the activity trail; its
	// cover shows "-" for the number.
	date := ""
	if row != nil {
		if d.VersionLabel == "" {
			d.VersionLabel = row.Version
		}
		if d.VersionLabel == "" {
			date = row.CreatedAt.Time.UTC().Format("2006-01-02")
		}
	}
	if org, err := s.db.GetOrganization(ctx, orgID); err == nil && org != nil {
		d.Org.Name = s.db.OrgBrandName(ctx, org)
		d.Org.LogoPNG = s.exportLogo(c, org.UUID)
	}
	if color, err := s.db.GetOrgSetting(ctx, orgID, "branding_color"); err == nil {
		d.Org.BrandColor = color
	}

	// Every render spends a metered credit: take one from the caller's and the
	// organization's allowance only now, after every check that can refuse.
	release, wait, ok := s.exportLimit.take(exportLimitUser(c), orgID)
	if !ok {
		mins := int(math.Ceil(wait.Minutes()))
		if mins < 1 {
			mins = 1
		}
		h := c.Response().Header()
		h.Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		h.Set("Cache-Control", "no-store")
		return apiError(http.StatusTooManyRequests, CodeExportRateLimited, Count(mins))
	}

	out, err := s.renderExport(format, &d)
	if err != nil {
		release()
		if errors.Is(err, export.ErrUnlicensed) {
			return apiError(http.StatusForbidden, CodeExportNotLicensed, Value(strings.ToUpper(format)))
		}
		log.Printf("export %s as %s failed: %v", docID, format, err)
		return apiError(http.StatusInternalServerError, CodeExportFailed)
	}

	// Fail closed: an export the activity trail cannot record is not served.
	// A plain write, not logAndNotify: an export never posts to chat.
	what := "(current)"
	switch {
	case d.VersionLabel != "":
		what = "version " + d.VersionLabel
	case date != "":
		what = "version of " + date
	}
	if err := s.db.LogActivity(ctx, orgID, &db.Activity{
		DocumentID: docID,
		Actor:      getUserEmail(c),
		Action:     "document_exported",
		Detail:     "Exported " + docID + " " + what + " as " + strings.ToUpper(format),
	}); err != nil {
		log.Printf("export %s: recording activity: %v", docID, err)
		return apiError(http.StatusInternalServerError, CodeExportFailed)
	}

	name := exportFilename(docID, d.VersionLabel, date, export.Ext(format))
	disp := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if disp == "" {
		disp = "attachment"
	}
	h := c.Response().Header()
	h.Set("Content-Disposition", disp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	return c.Blob(http.StatusOK, export.MIMEType(format), out)
}

// exportLimitUser is who an export is counted against: the member's email,
// lower-cased (an API key carries its owner's), or the API key itself when it
// has no email, so no request goes uncounted.
func exportLimitUser(c echo.Context) string {
	if email := strings.ToLower(strings.TrimSpace(getUserEmail(c))); email != "" {
		return email
	}
	if id, ok := c.Get("api_key_id").(int); ok {
		return "api-key:" + strconv.Itoa(id)
	}
	return ""
}

// exportLogo returns the organisation's PNG logo, or nil when there is none,
// it is larger than maxExportLogo, or it cannot be read. The SVG logo is never
// read: only a raster logo is printed.
func (s *Server) exportLogo(c echo.Context, orgUUID string) []byte {
	if s.blobs == nil || orgUUID == "" {
		return nil
	}
	ctx := c.Request().Context()
	if ok, _ := s.blobs.Exists(ctx, orgUUID, "branding/logo.png"); !ok {
		return nil
	}
	rc, size, err := s.blobs.Open(ctx, orgUUID, "branding/logo.png")
	if err != nil {
		return nil
	}
	defer rc.Close()
	if size > maxExportLogo {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(rc, maxExportLogo+1))
	if err != nil || len(b) > maxExportLogo {
		return nil
	}
	return b
}
