package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/client"
	"isms.sh/internal/isms/db"
)

func exportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export ISMS documents (markdown, PDF, DOCX)",
		Long:  "Export ISMS data as markdown. PDF/DOCX export requires a UniDoc license (UNIDOC_API_KEY).",
	}

	cmd.AddCommand(
		exportPolicyCmd(),
		exportDocumentsCmd(),
		exportRisksCmd(),
		exportAssetsCmd(),
		exportSuppliersCmd(),
		exportAuditPackCmd(),
		exportManualCmd(),
	)

	return cmd
}

// checkUniDocKey checks if UNIDOC_API_KEY is set for non-markdown formats.
// For markdown format, it always returns nil.
// For pdf/docx, it returns an error telling the user it's coming soon.
func checkExportFormat(format string) error {
	switch format {
	case "md", "markdown":
		return nil
	case "pdf", "docx", "xlsx":
		key := os.Getenv("UNIDOC_API_KEY")
		if key == "" {
			return fmt.Errorf("%s export requires a UniDoc license. Set UNIDOC_API_KEY in your env file.\nGet one at https://unidoc.io\n\nUse --format md for markdown export.", format)
		}
		return fmt.Errorf("PDF/DOCX export coming soon. UniDoc integration is in progress.\nUse --format md for markdown export.")
	default:
		return fmt.Errorf("unsupported format: %s (supported: md, pdf, docx)", format)
	}
}

// writeOutput writes content to a file or stdout.
func writeOutput(output, content string) error {
	if output == "" || output == "-" {
		fmt.Print(content)
		return nil
	}
	if err := os.WriteFile(output, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Exported to %s\n", output)
	return nil
}

func exportPolicyCmd() *cobra.Command {
	var format, output string

	cmd := &cobra.Command{
		Use:   "policy <doc-id>",
		Short: "Export a single policy as markdown",
		Long:  "Export a policy document. Use the document ID (e.g. POL-AC-001).",
		Example: "  isms export policy POL-AC-001\n  isms export policy POL-AC-001 --output policy.md",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkExportFormat(format); err != nil {
				return err
			}

			c := requireAPI()
			docID := args[0]
			doc, err := c.GetDocumentBody(docID)
			if err != nil {
				return fmt.Errorf("document not found: %s", docID)
			}

			var b strings.Builder
			b.WriteString(fmt.Sprintf("# %s\n\n", doc.Title))
			b.WriteString("| Field | Value |\n")
			b.WriteString("|-------|-------|\n")
			b.WriteString(fmt.Sprintf("| Document ID | %s |\n", doc.DocumentID))
			b.WriteString(fmt.Sprintf("| Version | %s |\n", doc.Version))
			b.WriteString(fmt.Sprintf("| Status | %s |\n", doc.Status))
			if doc.Author != "" {
				b.WriteString(fmt.Sprintf("| Author | %s |\n", doc.Author))
			}
			b.WriteString("\n---\n\n")
			b.WriteString(doc.Body)
			if !strings.HasSuffix(doc.Body, "\n") {
				b.WriteString("\n")
			}

			return writeOutput(output, b.String())
		},
	}

	cmd.Flags().StringVar(&format, "format", "md", "Export format: md, pdf, docx")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")
	return cmd
}

func exportDocumentsCmd() *cobra.Command {
	var format, output, folder string

	cmd := &cobra.Command{
		Use:     "documents [--folder <name>]",
		Short:   "Export all documents (or a specific folder)",
		Example: "  isms export documents\n  isms export documents --folder my-folder --output export.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkExportFormat(format); err != nil {
				return err
			}

			c := requireAPI()
			var docs []client.DocSummary
			var err error
			label := "Documents"
			if folder != "" {
				docs, err = c.ListDocsByFolder(folder)
				label = strings.Title(folder) //nolint:staticcheck
			} else {
				docs, err = c.FlattenAllDocs()
			}
			if err != nil {
				return err
			}
			if len(docs) == 0 {
				return fmt.Errorf("no documents found")
			}

			var b strings.Builder
			b.WriteString(fmt.Sprintf("# ISMS %s\n\n", label))
			b.WriteString(fmt.Sprintf("*Exported: %s*\n\n", time.Now().Format("2006-01-02")))
			b.WriteString(fmt.Sprintf("**Total: %d**\n\n", len(docs)))

			// Summary table
			b.WriteString(fmt.Sprintf("## %s Register\n\n", label))
			b.WriteString("| ID | Title | Version | Status | Author |\n")
			b.WriteString("|-----|-------|---------|--------|--------|\n")
			for _, d := range docs {
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
					d.DocumentID, d.Title, d.Version, d.Status, d.Author))
			}
			b.WriteString("\n---\n\n")

			// Full content
			for i, d := range docs {
				if i > 0 {
					b.WriteString("\n---\n\n")
				}
				b.WriteString(fmt.Sprintf("## %s: %s\n\n", d.DocumentID, d.Title))
				b.WriteString(fmt.Sprintf("*Version %s | %s | %s*\n\n", d.Version, d.Status, d.Author))
				body, err := c.GetDocumentBody(d.DocumentID)
				if err == nil {
					b.WriteString(body.Body)
					if !strings.HasSuffix(body.Body, "\n") {
						b.WriteString("\n")
					}
				}
			}

			return writeOutput(output, b.String())
		},
	}

	cmd.Flags().StringVar(&format, "format", "md", "Export format: md, pdf, docx")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().StringVar(&folder, "folder", "", "Folder name to export")
	return cmd
}

func exportRisksCmd() *cobra.Command {
	var format, output string

	cmd := &cobra.Command{
		Use:     "risks",
		Short:   "Export risk register",
		Example: "  isms export risks\n  isms export risks --output risks.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkExportFormat(format); err != nil {
				return err
			}

			c := requireAPI()
			risks, err := c.ListRisks()
			if err != nil {
				return err
			}
			if len(risks) == 0 {
				return fmt.Errorf("no risks found")
			}

			var b strings.Builder
			b.WriteString("# Risk Register\n\n")
			b.WriteString(fmt.Sprintf("*Exported: %s*\n\n", time.Now().Format("2006-01-02")))

			// Summary
			counts := map[string]int{}
			for _, r := range risks {
				counts[r.CurrentLevel]++
			}
			b.WriteString("## Summary\n\n")
			b.WriteString(fmt.Sprintf("**Total risks: %d**\n\n", len(risks)))
			for _, level := range []string{"critical", "high", "medium", "low"} {
				if counts[level] > 0 {
					b.WriteString(fmt.Sprintf("- **%s:** %d\n", level, counts[level]))
				}
			}
			b.WriteString("\n")

			// Register table
			b.WriteString("## Risk Register\n\n")
			b.WriteString("| ID | Title | L | I | Score | Level | Treatment | Status | Owner |\n")
			b.WriteString("|----|-------|---|---|-------|-------|-----------|--------|-------|\n")
			for _, r := range risks {
				b.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d | %s | %s | %s | %s |\n",
					r.Identifier, r.Title, r.CurrentLikelihood, r.CurrentImpact, r.CurrentScore, r.CurrentLevel,
					r.Treatment, r.Status, r.Owner))
			}
			b.WriteString("\n")

			// Detail per risk
			b.WriteString("## Risk Details\n\n")
			for _, r := range risks {
				b.WriteString(fmt.Sprintf("### %s: %s\n\n", r.Identifier, r.Title))
				b.WriteString(fmt.Sprintf("**Risk Level:** %s (L=%d x I=%d = %d)\n\n", r.CurrentLevel, r.CurrentLikelihood, r.CurrentImpact, r.CurrentScore))
				if r.Description != "" {
					b.WriteString(fmt.Sprintf("**Description:** %s\n\n", r.Description))
				}
				b.WriteString(fmt.Sprintf("**Treatment:** %s\n\n", r.Treatment))
				if r.TreatmentPlan != "" {
					b.WriteString(fmt.Sprintf("**Treatment Plan:** %s\n\n", r.TreatmentPlan))
				}
				b.WriteString(fmt.Sprintf("**Owner:** %s | **Status:** %s | **Next Review:** %s\n\n", r.Owner, r.Status, r.NextReview))
			}

			return writeOutput(output, b.String())
		},
	}

	cmd.Flags().StringVar(&format, "format", "md", "Export format: md, pdf, docx")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")
	return cmd
}

func exportAssetsCmd() *cobra.Command {
	var format, output string

	cmd := &cobra.Command{
		Use:     "assets",
		Short:   "Export asset register",
		Example: "  isms export assets\n  isms export assets --output assets.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkExportFormat(format); err != nil {
				return err
			}

			c := requireAPI()
			assets, err := c.ListAssets()
			if err != nil {
				return err
			}
			if len(assets) == 0 {
				return fmt.Errorf("no assets found")
			}

			var b strings.Builder
			b.WriteString("# Asset Register\n\n")
			b.WriteString(fmt.Sprintf("*Exported: %s*\n\n", time.Now().Format("2006-01-02")))
			b.WriteString(fmt.Sprintf("**Total assets: %d**\n\n", len(assets)))

			// Summary by status
			statusCount := map[string]int{}
			typeCount := map[string]int{}
			for _, a := range assets {
				statusCount[a.Status]++
				typeCount[a.AssetType]++
			}
			b.WriteString("## Summary\n\n")
			b.WriteString("**By status:**\n")
			for _, st := range db.AssetStatuses {
				if statusCount[st] > 0 {
					b.WriteString(fmt.Sprintf("- %s: %d\n", st, statusCount[st]))
				}
			}
			b.WriteString("\n**By type:**\n")
			for _, t := range db.AssetTypes {
				if typeCount[t] > 0 {
					b.WriteString(fmt.Sprintf("- %s: %d\n", t, typeCount[t]))
				}
			}
			b.WriteString("\n")

			// Asset table
			b.WriteString("## Asset Register\n\n")
			b.WriteString("| ID | Name | Type | Status | C | I | A | Owner | Location |\n")
			b.WriteString("|----|------|------|--------|---|---|---|-------|----------|\n")
			for _, a := range assets {
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %d | %d | %s | %s |\n",
					a.Identifier, a.Name, a.AssetType, a.Status, intVal(a.Confidentiality), intVal(a.Integrity), intVal(a.Availability), a.Owner, a.PrimaryLocation))
			}
			b.WriteString("\n")

			// Detail
			b.WriteString("## Asset Details\n\n")
			for _, a := range assets {
				b.WriteString(fmt.Sprintf("### %s: %s\n\n", a.Identifier, a.Name))
				b.WriteString(fmt.Sprintf("- **Type:** %s\n", a.AssetType))
				b.WriteString(fmt.Sprintf("- **Status:** %s\n", a.Status))
				b.WriteString(fmt.Sprintf("- **Confidentiality:** %d\n", intVal(a.Confidentiality)))
				b.WriteString(fmt.Sprintf("- **Integrity:** %d\n", intVal(a.Integrity)))
				b.WriteString(fmt.Sprintf("- **Availability:** %d\n", intVal(a.Availability)))
				b.WriteString(fmt.Sprintf("- **Owner:** %s\n", a.Owner))
				if a.PrimaryLocation != "" {
					b.WriteString(fmt.Sprintf("- **Location:** %s\n", a.PrimaryLocation))
				}
				if a.Description != "" {
					b.WriteString(fmt.Sprintf("- **Description:** %s\n", a.Description))
				}
				b.WriteString("\n")
			}

			return writeOutput(output, b.String())
		},
	}

	cmd.Flags().StringVar(&format, "format", "md", "Export format: md, pdf, docx")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")
	return cmd
}

func exportSuppliersCmd() *cobra.Command {
	var format, output string

	cmd := &cobra.Command{
		Use:     "suppliers",
		Short:   "Export supplier register",
		Example: "  isms export suppliers\n  isms export suppliers --output suppliers.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkExportFormat(format); err != nil {
				return err
			}

			c := requireAPI()
			suppliers, err := c.ListSuppliers()
			if err != nil {
				return err
			}
			if len(suppliers) == 0 {
				return fmt.Errorf("no suppliers found")
			}

			var b strings.Builder
			b.WriteString("# Supplier Register\n\n")
			b.WriteString(fmt.Sprintf("*Exported: %s*\n\n", time.Now().Format("2006-01-02")))
			b.WriteString(fmt.Sprintf("**Total suppliers: %d**\n\n", len(suppliers)))

			// Summary
			critCount := map[string]int{}
			for _, s := range suppliers {
				critCount[s.Criticality]++
			}
			b.WriteString("## Summary\n\n")
			b.WriteString("**By criticality:**\n")
			for _, c := range db.CriticalityLevels {
				if critCount[c] > 0 {
					b.WriteString(fmt.Sprintf("- %s: %d\n", c, critCount[c]))
				}
			}
			b.WriteString("\n")

			// Supplier table
			b.WriteString("## Supplier Register\n\n")
			b.WriteString("| ID | Name | Type | Criticality | Data Access |\n")
			b.WriteString("|----|------|------|-------------|-------------|\n")
			for _, s := range suppliers {
				dataAccess := "No"
				if s.DataAccess {
					dataAccess = "Yes"
				}
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
					s.Identifier, s.Name, s.SupplierType, s.Criticality, dataAccess))
			}
			b.WriteString("\n")

			// Detail
			b.WriteString("## Supplier Details\n\n")
			for _, s := range suppliers {
				b.WriteString(fmt.Sprintf("### %s: %s\n\n", s.Identifier, s.Name))
				b.WriteString(fmt.Sprintf("- **Type:** %s\n", s.SupplierType))
				b.WriteString(fmt.Sprintf("- **Criticality:** %s\n", s.Criticality))
				dataAccess := "No"
				if s.DataAccess {
					dataAccess = "Yes"
				}
				b.WriteString(fmt.Sprintf("- **Data Access:** %s\n", dataAccess))
				if s.Contact != "" {
					b.WriteString(fmt.Sprintf("- **Contact:** %s\n", s.Contact))
				}
				if s.ContractRef != "" {
					b.WriteString(fmt.Sprintf("- **Contract Ref:** %s\n", s.ContractRef))
				}
				if s.LastReview != nil && !s.LastReview.IsZero() {
					b.WriteString(fmt.Sprintf("- **Last Review:** %s\n", s.LastReview.Format("2006-01-02")))
				}
				if s.NextReview != nil && !s.NextReview.IsZero() {
					b.WriteString(fmt.Sprintf("- **Next Review:** %s\n", s.NextReview.Format("2006-01-02")))
				}
				if s.Notes != "" {
					b.WriteString(fmt.Sprintf("- **Notes:** %s\n", s.Notes))
				}
				b.WriteString("\n")
			}

			return writeOutput(output, b.String())
		},
	}

	cmd.Flags().StringVar(&format, "format", "md", "Export format: md, pdf, docx")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")
	return cmd
}

func exportAuditPackCmd() *cobra.Command {
	var format, output string

	cmd := &cobra.Command{
		Use:     "audit-pack <audit-id>",
		Short:   "Export audit report and findings",
		Example: "  isms export audit-pack 1\n  isms export audit-pack 1 --output audit-report.md",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkExportFormat(format); err != nil {
				return err
			}

			auditID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid audit ID: %s", args[0])
			}

			c := requireAPI()

			// Fetch audit details
			audit, err := c.GetAudit(auditID)
			if err != nil {
				return fmt.Errorf("audit not found: %d", auditID)
			}

			// Fetch items and findings
			items, err := c.ListAuditItems(auditID)
			if err != nil {
				return fmt.Errorf("fetching audit items: %w", err)
			}
			findings, err := c.ListAuditFindings(auditID)
			if err != nil {
				return fmt.Errorf("fetching audit findings: %w", err)
			}

			var b strings.Builder
			b.WriteString("# Internal Audit Report\n\n")
			b.WriteString(fmt.Sprintf("*Exported: %s*\n\n", time.Now().Format("2006-01-02")))

			// Audit metadata
			b.WriteString("## Audit Details\n\n")
			b.WriteString("| Field | Value |\n")
			b.WriteString("|-------|-------|\n")
			b.WriteString(fmt.Sprintf("| Audit ID | %d |\n", audit.ID))
			b.WriteString(fmt.Sprintf("| Title | %s |\n", audit.Title))
			b.WriteString(fmt.Sprintf("| Scope | %s |\n", audit.Scope))
			b.WriteString(fmt.Sprintf("| Auditor | %s |\n", audit.Auditor))
			b.WriteString(fmt.Sprintf("| Status | %s |\n", audit.Status))
			if audit.PlannedDate != nil {
				b.WriteString(fmt.Sprintf("| Planned Date | %s |\n", audit.PlannedDate.Format("2006-01-02")))
			}
			if audit.StartedAt != nil {
				b.WriteString(fmt.Sprintf("| Started | %s |\n", audit.StartedAt.Format("2006-01-02")))
			}
			if audit.CompletedAt != nil {
				b.WriteString(fmt.Sprintf("| Completed | %s |\n", audit.CompletedAt.Format("2006-01-02")))
			}
			b.WriteString("\n")

			if audit.Summary != "" {
				b.WriteString("## Executive Summary\n\n")
				b.WriteString(audit.Summary)
				b.WriteString("\n\n")
			}

			// Results summary
			if len(items) > 0 {
				resultCount := map[string]int{}
				for _, item := range items {
					r := item.Result
					if r == "" {
						r = "pending"
					}
					resultCount[r]++
				}

				b.WriteString("## Results Summary\n\n")
				b.WriteString(fmt.Sprintf("**Total items assessed:** %d\n\n", len(items)))
				for _, r := range []string{"conforming", "minor_nc", "major_nc", "observation", "pending"} {
					if resultCount[r] > 0 {
						b.WriteString(fmt.Sprintf("- **%s:** %d\n", r, resultCount[r]))
					}
				}
				b.WriteString("\n")

				// Items table
				b.WriteString("## Audit Items\n\n")
				b.WriteString("| Item | Type | Title | Result | Evidence |\n")
				b.WriteString("|------|------|-------|--------|----------|\n")
				for _, item := range items {
					result := item.Result
					if result == "" {
						result = "pending"
					}
					evidence := item.Evidence
					if len(evidence) > 60 {
						evidence = evidence[:57] + "..."
					}
					b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
						item.ItemID, item.ItemType, item.Title, result, evidence))
				}
				b.WriteString("\n")
			}

			// Findings
			if len(findings) > 0 {
				b.WriteString("## Findings\n\n")
				for i, f := range findings {
					b.WriteString(fmt.Sprintf("### Finding %d: %s\n\n", i+1, f.Title))
					b.WriteString(fmt.Sprintf("- **Type:** %s\n", f.FindingType))
					b.WriteString(fmt.Sprintf("- **Status:** %s\n", f.Status))
					if f.AuditItemID != nil {
						b.WriteString(fmt.Sprintf("- **Related Audit Item:** #%d\n", *f.AuditItemID))
					}
					if f.DueDate != nil {
						b.WriteString(fmt.Sprintf("- **Due Date:** %s\n", f.DueDate.Format("2006-01-02")))
					}
					// Description includes ## Corrective Action heading.
					b.WriteString(fmt.Sprintf("\n%s\n", f.Description))
					b.WriteString("\n")
				}
			} else {
				b.WriteString("## Findings\n\nNo findings recorded.\n\n")
			}

			return writeOutput(output, b.String())
		},
	}

	cmd.Flags().StringVar(&format, "format", "md", "Export format: md, pdf, docx")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")
	return cmd
}

func exportManualCmd() *cobra.Command {
	var format, output string

	cmd := &cobra.Command{
		Use:     "manual",
		Short:   "Export full ISMS manual (all document folders)",
		Example: "  isms export manual\n  isms export manual --output isms-manual.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkExportFormat(format); err != nil {
				return err
			}

			c := requireAPI()

			// Fetch all documents grouped by folder dynamically.
			allFolders, err := c.ListAllDocuments()
			if err != nil {
				return fmt.Errorf("fetching documents: %w", err)
			}

			// Flatten each folder into a list of docs.
			type folderDocs struct {
				name string
				docs []client.DocSummary
			}
			var sections []folderDocs
			for _, f := range allFolders {
				var docs []client.DocSummary
				var collect func(folder client.DocFolder)
				collect = func(folder client.DocFolder) {
					docs = append(docs, folder.Files...)
					for _, sub := range folder.SubFolders {
						collect(sub)
					}
				}
				collect(f)
				if len(docs) > 0 {
					sections = append(sections, folderDocs{name: f.Name, docs: docs})
				}
			}

			var b strings.Builder
			b.WriteString("# ISMS Manual\n\n")
			b.WriteString(fmt.Sprintf("*Exported: %s*\n\n", time.Now().Format("2006-01-02")))
			b.WriteString("## Table of Contents\n\n")

			// TOC
			for i, sec := range sections {
				b.WriteString(fmt.Sprintf("### Part %d: %s\n\n", i+1, strings.Title(sec.name))) //nolint:staticcheck
				for _, d := range sec.docs {
					b.WriteString(fmt.Sprintf("- %s: %s\n", d.DocumentID, d.Title))
				}
				b.WriteString("\n")
			}

			b.WriteString("---\n\n")

			// Full content
			for i, sec := range sections {
				if i > 0 {
					b.WriteString("---\n\n")
				}
				b.WriteString(fmt.Sprintf("# Part %d: %s\n\n", i+1, strings.Title(sec.name))) //nolint:staticcheck
				for _, d := range sec.docs {
					b.WriteString(fmt.Sprintf("## %s: %s\n\n", d.DocumentID, d.Title))
					b.WriteString(fmt.Sprintf("*Version %s | %s | %s*\n\n", d.Version, d.Status, d.Author))
					body, err := c.GetDocumentBody(d.DocumentID)
					if err == nil {
						b.WriteString(body.Body)
						if !strings.HasSuffix(body.Body, "\n") {
							b.WriteString("\n")
						}
					}
					b.WriteString("\n")
				}
			}

			return writeOutput(output, b.String())
		},
	}

	cmd.Flags().StringVar(&format, "format", "md", "Export format: md, pdf, docx")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")
	return cmd
}
