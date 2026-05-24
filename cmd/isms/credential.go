package main

import (
	"fmt"
	"os"
	"strings"

	git "github.com/go-git/go-git/v5"
)

// refreshCredentialHelperForRepo opens the repo at root, reads its origin
// remote URL, and (re)writes the credential helper with the current
// ISMS_API_TOKEN. Used by `isms git` to repair older clones whose helper
// was never written. No-op if no token / no remote.
func refreshCredentialHelperForRepo(root string) error {
	token := os.Getenv("ISMS_API_TOKEN")
	if token == "" {
		return nil
	}
	repo, err := git.PlainOpen(root)
	if err != nil {
		return err
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return err
	}
	for _, r := range remotes {
		if r.Config().Name != gitRemoteName {
			continue
		}
		urls := r.Config().URLs
		if len(urls) == 0 {
			continue
		}
		return ensureCredentialHelper(repo, urls[0], token)
	}
	return nil
}

// ensureCredentialHelper writes the git credential helper to .git/config so
// that `git` CLI invocations (via `isms git ...`, `isms sync` rebase, etc.)
// can authenticate to the ISMS server without the user supplying a token.
//
// The helper is scoped to the HOST (not the full URL) so prefix matching
// catches all paths under the host. Idempotent — safe to call on every
// invocation; existing helper config is overwritten with the current token
// so token rotation just works.
func ensureCredentialHelper(repo *git.Repository, remoteURL, token string) error {
	hostScope := credentialScopeFromURL(remoteURL)
	if hostScope == "" {
		return fmt.Errorf("credential helper: could not derive host from %q", remoteURL)
	}
	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("reading repo config: %w", err)
	}
	helper := fmt.Sprintf("!f() { echo username=x-token-auth; echo password=%s; }; f", token)
	cfg.Raw.Section("credential").Subsection(hostScope).SetOption("helper", helper)
	// Don't prompt for terminal input on auth failure — fail fast instead.
	cfg.Raw.Section("credential").SetOption("prompt", "false")
	if err := repo.SetConfig(cfg); err != nil {
		return fmt.Errorf("writing repo config: %w", err)
	}
	return nil
}

// credentialScopeFromURL strips path and query from a URL, returning the
// scheme + host (with optional port). That's the scope at which git's
// credential matching applies — anything under that prefix uses the helper.
//
//	https://sts.commandvector.net/git/<uuid>  → https://sts.commandvector.net
//	https://isms.sh:9443/git/foo              → https://isms.sh:9443
func credentialScopeFromURL(u string) string {
	// Trim scheme
	scheme := ""
	rest := u
	if i := strings.Index(u, "://"); i > 0 {
		scheme = u[:i]
		rest = u[i+3:]
	}
	// Take up to first '/' '?' or '#'
	end := len(rest)
	for i, r := range rest {
		if r == '/' || r == '?' || r == '#' {
			end = i
			break
		}
	}
	host := rest[:end]
	if host == "" {
		return ""
	}
	if scheme != "" {
		return scheme + "://" + host
	}
	return host
}
