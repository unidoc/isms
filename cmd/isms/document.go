package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"isms.sh/internal/isms/client"
)

func documentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "document",
		Short:   "List, search and read documents",
		Aliases: []string{"doc", "docs"},
	}
	cmd.AddCommand(
		documentListCmd(),
		documentReadCmd(),
		documentCatCmd(),
		documentSearchCmd(),
	)
	return cmd
}

// --- list ---------------------------------------------------------------

func documentListCmd() *cobra.Command {
	var (
		folder string
		status string
		quiet  bool
	)
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List documents",
		Long:    "List all documents. With --folder, only show one section. With --status, only matching status (draft, in_review, approved, retired).",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			folders, err := c.ListAllDocuments()
			if err != nil {
				return err
			}

			matched := 0
			for _, f := range folders {
				if folder != "" && !strings.EqualFold(f.Name, folder) && !strings.EqualFold(f.Title, folder) {
					continue
				}
				docs := flattenFolder(f)
				if status != "" {
					filtered := docs[:0]
					for _, d := range docs {
						if d.Status == status {
							filtered = append(filtered, d)
						}
					}
					docs = filtered
				}
				if len(docs) == 0 {
					continue
				}
				sort.SliceStable(docs, func(i, j int) bool { return docs[i].DocumentID < docs[j].DocumentID })

				header := f.Name
				if f.Title != "" {
					header = f.Title
				}
				if !quiet {
					fmt.Printf("\n%s\n%s\n", header, strings.Repeat("─", min(60, len(header)+10)))
				}
				for _, d := range docs {
					st := d.Status
					if st == "" {
						st = "draft"
					}
					if quiet {
						fmt.Println(d.DocumentID)
					} else {
						fmt.Printf("  %-22s %-12s %s\n", d.DocumentID, st, d.Title)
					}
					matched++
				}
			}

			if matched == 0 && !quiet {
				fmt.Fprintln(os.Stderr, "No documents matched.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&folder, "folder", "", "Filter by folder name")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (draft, in_review, approved, retired)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only print document IDs, for piping")
	return cmd
}

// --- read (renders + pages) ---------------------------------------------

func documentReadCmd() *cobra.Command {
	var noPager bool
	cmd := &cobra.Command{
		Use:     "read <document-id>",
		Aliases: []string{"show"},
		Short:   "Render a document with formatted markdown and page it",
		Long:    "Renders the document with markdown styling and pages through $PAGER (or `less -R` if unset). Use --no-pager to dump to stdout.",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			id := strings.ToLower(args[0])
			doc, err := c.GetDocumentBody(id)
			if err != nil {
				return fmt.Errorf("document not found: %s", id)
			}

			width := 100
			if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
				if w, _, err := term.GetSize(fd); err == nil && w > 20 {
					width = w
					if width > 120 {
						width = 120
					}
				}
			}

			rendered, err := renderDoc(doc, width)
			if err != nil {
				return err
			}

			if noPager || !term.IsTerminal(int(os.Stdout.Fd())) {
				fmt.Print(rendered)
				return nil
			}
			return pipeToPager(rendered)
		},
	}
	cmd.Flags().BoolVar(&noPager, "no-pager", false, "Skip the pager, write to stdout")
	return cmd
}

// --- cat (raw body, for piping) -----------------------------------------

func documentCatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cat <document-id>",
		Short: "Print raw markdown (no rendering, no pager) — useful for piping",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			id := strings.ToLower(args[0])
			doc, err := c.GetDocumentBody(id)
			if err != nil {
				return fmt.Errorf("document not found: %s", id)
			}
			fmt.Print(doc.Body)
			if !strings.HasSuffix(doc.Body, "\n") {
				fmt.Println()
			}
			return nil
		},
	}
}

// --- search -------------------------------------------------------------

func documentSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search documents by ID or title",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			q := strings.ToLower(strings.Join(args, " "))
			folders, err := c.ListAllDocuments()
			if err != nil {
				return err
			}
			matches := []client.DocSummary{}
			for _, f := range folders {
				for _, d := range flattenFolder(f) {
					if strings.Contains(strings.ToLower(d.DocumentID), q) ||
						strings.Contains(strings.ToLower(d.Title), q) {
						matches = append(matches, d)
					}
				}
			}
			sort.SliceStable(matches, func(i, j int) bool { return matches[i].DocumentID < matches[j].DocumentID })
			for _, d := range matches {
				st := d.Status
				if st == "" {
					st = "draft"
				}
				fmt.Printf("  %-22s %-12s %s\n", d.DocumentID, st, d.Title)
			}
			if len(matches) == 0 {
				fmt.Fprintln(os.Stderr, "No matches.")
			}
			return nil
		},
	}
}

// --- helpers ------------------------------------------------------------

func flattenFolder(f client.DocFolder) []client.DocSummary {
	out := append([]client.DocSummary{}, f.Files...)
	for _, sub := range f.SubFolders {
		out = append(out, flattenFolder(sub)...)
	}
	return out
}

func renderDoc(doc *client.DocBody, width int) (string, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		return "", err
	}
	body := stripDocFrontmatter(doc.Body)
	rendered, err := r.Render(body)
	if err != nil {
		return "", err
	}
	header := fmt.Sprintf("# %s\n\n*%s · %s · v%s*\n\n", doc.Title, doc.DocumentID, doc.Status, doc.Version)
	headerRendered, _ := r.Render(header)
	return headerRendered + rendered, nil
}

func stripDocFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---\n") {
		return s
	}
	end := strings.Index(s[4:], "\n---\n")
	if end < 0 {
		return s
	}
	return strings.TrimLeft(s[4+end+5:], "\n")
}

func pipeToPager(content string) error {
	pagerCmd := os.Getenv("PAGER")
	if pagerCmd == "" {
		pagerCmd = "less -R"
	}
	parts := strings.Fields(pagerCmd)
	if len(parts) == 0 {
		fmt.Print(content)
		return nil
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Fallback to stdout if pager fails (e.g. less not installed)
		fmt.Print(content)
	}
	return nil
}
