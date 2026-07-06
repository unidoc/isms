package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
	"isms.sh/internal/isms/client"
	"isms.sh/internal/isms/db"
)

var (
	ismsRoot string
	envFile  string
)

func main() {
	root := &cobra.Command{
		Use:   "isms",
		Short: "isms.sh — The Intelligent Management System",
		Long:  "Manage your ISMS with AI. Git-native, API-first, multi-tenant.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// --env flag overrides everything
			ef := envFile
			if ef == "" {
				// Check if we're inside the "server" subcommand tree
				if isServerCommand(cmd) {
					ef = os.Getenv("ISMS_SERVER_ENV")
				} else {
					ef = os.Getenv("ISMS_ENV")
				}
			}
			if ef != "" {
				loadEnvFile(ef)
			}
		},
	}

	root.PersistentFlags().StringVar(&ismsRoot, "root", "", "ISMS repository root directory (default: git repo root)")
	root.PersistentFlags().StringVar(&envFile, "env", "", "Load environment from file (overrides auto-detection)")

	// Client commands (default namespace) — uses ISMS_ENV
	root.AddCommand(
		cloneCmd(),
		gitCmd(),
		documentCmd(),
		assetCmd(),
		riskCmd(),
		supplierCmd(),
		systemCmd(),
		exportCmd(),
		reviewCmd(),
		inboxCmd(),
		diffCmd(),
		statusCmd(),
		auditCmd(),
		incidentCmd(),
		correctiveCmd(),
		changeCmd(),
		legalCmd(),
		programCmd(),
		objectiveCmd(),
		checkinCmd(),
		syncCmd(),
		overdueCmd(),
		tuiCmd(),
		whoamiCmd(),
		versionCmd(),
	)

	// Server commands — uses ISMS_SERVER_ENV
	root.AddCommand(serverCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// serverCmd groups all server/admin commands under "isms server".
func serverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Server administration (requires DATABASE_URL)",
		Long:  "Server-side commands for managing the ISMS platform. These require direct database access and run on the server machine.",
	}

	cmd.AddCommand(
		serveCmd(),
		migrateCmd(),
		managerCmd(),
		mcpCmd(),
		orgCmd(),
		userCmd(),
		apiKeyCmd(),
		testEmailCmd(),
	)

	return cmd
}

// isServerCommand checks if the command is inside the "server" subcommand tree.
func isServerCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "server" {
			return true
		}
	}
	return false
}

// configuredRoot returns the explicitly configured repo root — the --root flag,
// else ISMS_ROOT — or "" if neither is set. The single place that encodes that
// precedence, shared by resolveRepoRoot and clone so they can't drift.
func configuredRoot() string {
	if ismsRoot != "" {
		return ismsRoot
	}
	return os.Getenv("ISMS_ROOT")
}

// resolveRepoRoot locates the local ISMS clone for repo commands (sync, git):
// the configured root (--root / ISMS_ROOT), then walking up from cwd to a .git.
// Errors when none is found so commands fail clearly instead of silently
// operating on the current directory.
func resolveRepoRoot() (string, error) {
	if r := configuredRoot(); r != "" {
		return r, nil
	}
	return gitRoot()
}

func gitRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		repo, err := git.PlainOpen(dir)
		if err == nil {
			wt, err := repo.Worktree()
			if err == nil {
				return wt.Filesystem.Root(), nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("not a git repository")
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load env file %s: %v\n", path, err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		os.Setenv(key, val)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func apiClient() *client.Client {
	apiURL := os.Getenv("ISMS_API_URL")
	if apiURL == "" {
		baseURL := os.Getenv("ISMS_BASE_URL")
		if baseURL == "" {
			return nil
		}
		apiURL = strings.TrimRight(baseURL, "/") + "/api"
	}
	apiKey := os.Getenv("ISMS_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ISMS_API_TOKEN")
	}
	return client.New(client.Config{
		BaseURL:        apiURL,
		BearerToken:    apiKey,
		CFClientID:     os.Getenv("CF_ACCESS_CLIENT_ID"),
		CFClientSecret: os.Getenv("CF_ACCESS_CLIENT_SECRET"),
		Organization:   os.Getenv("ISMS_ORGANIZATION"),
	})
}

func requireAPI() *client.Client {
	c := apiClient()
	if c == nil {
		fmt.Fprintln(os.Stderr, "error: ISMS_API_URL (or ISMS_BASE_URL) not set. Configure it in your env file.")
		os.Exit(1)
	}
	return c
}

// refSpec pairs a reference type with the target IDs from a CLI relation flag
// (e.g. --risks RISK-001,RISK-002 → {typ: "risk", ids: [...]}).
type refSpec struct {
	typ string
	ids []string
}

// buildRefs flattens relation-flag values into client references, skipping
// blanks. Used by `add` commands so their declared relation flags are sent
// rather than silently dropped (#52).
func buildRefs(specs ...refSpec) []client.Reference {
	var refs []client.Reference
	for _, s := range specs {
		for _, id := range s.ids {
			if id = strings.TrimSpace(id); id != "" {
				refs = append(refs, client.Reference{Type: s.typ, ID: id})
			}
		}
	}
	return refs
}

// intVal safely dereferences an *int, returning 0 for nil.
func intVal(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// parseEpochPtr parses a date string (YYYY-MM-DD) into an *Epoch. Returns nil for empty strings.
func parseEpochPtr(s string) (*db.Epoch, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
	}
	e := db.NewEpoch(t)
	return &e, nil
}

// epochPtrStr formats an *Epoch as YYYY-MM-DD or returns empty string for nil/zero.
func epochPtrStr(e *db.Epoch) string {
	if e == nil || e.IsZero() {
		return ""
	}
	return e.Format("2006-01-02")
}

// createReviewTask creates a review task via the API. Sets created_by from ISMS_USER env.
func createReviewTask(c *client.Client, task *db.Task) error {
	if task.CreatedBy == "" {
		task.CreatedBy = os.Getenv("ISMS_USER")
	}
	if task.Assignee == "" {
		task.Assignee = task.CreatedBy
	}
	if task.Status == "" {
		task.Status = "open"
	}
	if task.Priority == "" {
		task.Priority = "medium"
	}
	_, err := c.CreateTask(task)
	return err
}
