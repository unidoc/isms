package api

import (
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
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

// exportFilename is "<doc-id>-v<version>.<ext>", or "<doc-id>.<ext>" without a
// version label, reduced to [a-z0-9._-].
func exportFilename(docID, version, ext string) string {
	name := docID
	if version != "" {
		name += "-v" + version
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

	version := strings.TrimSpace(c.QueryParam("version"))
	var pf *store.DocumentFile
	if version == "" || version == "current" {
		version = ""
		pf, err = st.LoadDocument(path)
		if err != nil {
			log.Printf("export %s: loading document: %v", docID, err)
			return apiError(http.StatusInternalServerError, CodeExportFailed)
		}
	} else {
		v, err := s.db.GetVersion(ctx, orgID, docID, version)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apiError(http.StatusNotFound, CodeExportVersionNotFound)
			}
			log.Printf("export %s: looking up version %s: %v", docID, version, err)
			return apiError(http.StatusInternalServerError, CodeExportFailed)
		}
		raw, err := st.ReadFileAtRef(v.CommitHash, v.FilePath)
		if err != nil {
			log.Printf("export %s: reading version %s: %v", docID, version, err)
			return apiError(http.StatusInternalServerError, CodeExportFailed)
		}
		if pf, err = store.ParseDocument(v.FilePath, raw); err != nil {
			log.Printf("export %s: parsing version %s: %v", docID, version, err)
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
	if org, err := s.db.GetOrganization(ctx, orgID); err == nil && org != nil {
		d.Org.Name = s.db.OrgBrandName(ctx, org)
		d.Org.LogoPNG = s.exportLogo(c, org.UUID)
	}
	if color, err := s.db.GetOrgSetting(ctx, orgID, "branding_color"); err == nil {
		d.Org.BrandColor = color
	}

	out, err := s.renderExport(format, &d)
	if err != nil {
		if errors.Is(err, export.ErrUnlicensed) {
			return apiError(http.StatusForbidden, CodeExportNotLicensed, Value(strings.ToUpper(format)))
		}
		log.Printf("export %s as %s failed: %v", docID, format, err)
		return apiError(http.StatusInternalServerError, CodeExportFailed)
	}

	// Fail closed: an export the activity trail cannot record is not served.
	// A plain write, not logAndNotify: an export never posts to chat.
	what := "(current)"
	if d.VersionLabel != "" {
		what = "version " + d.VersionLabel
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

	name := exportFilename(docID, d.VersionLabel, export.Ext(format))
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
