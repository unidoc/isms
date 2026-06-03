# ISMS Dev Environment

Four containers, two stacks, one shared image (`isms-dev:latest`):

| Container | Role | Port (host) | Data |
|---|---|---|---|
| `isms-dev` | Utility/work container — Claude, shells, pytest run here. **Runs no server by itself.** | 9090 (when you `just serve`) | persistent (`isms-pgdata`, `isms-data`) |
| `isms-postgres` | Dev database | — (compose network only) | persistent volume |
| `isms-test` | Test server — auto-starts, always-on pytest target | 9091 | ephemeral |
| `isms-postgres-test` | Test database | — (compose network only) | tmpfs, wiped on reset |

Go build/module caches live on named volumes (`go-build-cache`, `go-mod-cache`)
shared by both ISMS containers — server restarts recompile incrementally (~2s),
and `just test-reset` does not throw the cache away.

## Daily workflow

```bash
just up          # start everything (idempotent)
just claude      # Claude Code in the work container
just test        # run the pytest suite against the test server (:9091)
```

**The key mental model: code changes never require touching `isms-dev`.**
The repo is bind-mounted, so every container always sees the current source.
What needs a nudge is any *running server process*, because Go servers run
compiled code from the moment they started:

| You changed… | Do this | Cost |
|---|---|---|
| Test assertions / pytest code | nothing — just `just test` | 0s |
| Go code, want tests to see it | `just test-restart` | ~2s (incremental rebuild, test DB kept) |
| Go code, dev server on :9090 | Ctrl-C the `just serve` terminal, run it again | ~2s |
| Test state is corrupted / weird failures | `just test-reset` | ~10s (DB + data wiped, server fresh) |
| Dockerfile / entrypoint.sh | `just build && just up` | minutes (image rebuild) |
| compose.yml | `just up` — and if `isms-test` was recreated, follow with `just test-reset` (see Troubleshooting: repo/DB desync) | seconds (recreates affected containers) |

## Claude and container restarts

Claude Code runs as an exec session *inside* `isms-dev`. Nothing in the
code-change workflow above touches that container, so Claude survives:
server restarts, test resets, test runs — all of it.

The only things that recreate `isms-dev` (and therefore end the session):

- `just up` after a change to `isms-dev`'s own compose config
- `just build && just up` after an image change
- `just down` / `just restart`

Recovery is one command — the session is on disk, not in the container:

```bash
just claude-continue    # resume the most recent session
just claude-resume      # pick an older session
```

Tip: when Claude edits devenv files (compose.yml, Dockerfile), expect the
next `just up`/`just build` to end the session — ask Claude to summarize
state first, or just continue and let it pick up from the transcript.

## Running tests

```bash
just test                      # whole suite
just test tests/test_risks.py # one file
just test tests/ -k review    # filter by keyword
just test tests/ -x -v        # stop at first failure, verbose
just test-e2e                  # Playwright browser + routing suites
just test-logs                 # tail the test server (boot problems live here)
```

- Tests only run when invoked — nothing runs them automatically.
- The suite is idempotent against accumulated state (fixed `test-org`),
  but state does accumulate; `just test-reset` gives a clean slate.
- Don't run two suites against the same test server at once — they share
  `test-org` and will trample each other. Sequential runs are fine.
- E2E routing tests reach the test server at `acme-logistics.localhost`
  via a compose network alias on `isms-test` (subdomain routing is resolved
  from the Host header).

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Mass failures: `opening bare repo … repository does not exist` | test DB and test data dir out of sync — `just up` recreated `isms-test` (wiping `/tmp/isms-test-data`) but not `postgres-test`, so the DB claims orgs whose git repos are gone | `just test-reset` — recreates both halves together |
| `connection refused` on :9091 right after `up`/reset | server still compiling/booting | wait ~10s; `just test-logs` |
| Server boots very slowly | cold Go cache (first boot after `nuke` or volume removal) | one-time cost; subsequent boots are fast |
| `go: command not found` in container | login-shell PATH (Debian resets it) | rebuilt image has `/etc/profile.d/golang.sh`; rebuild if missing |
| `.venv/bin/pytest: required file not found` | venv created in an older image (stale interpreter path) | `cd ~/workspace/isms && rm -rf .venv && python3 -m venv .venv && .venv/bin/pip install pytest requests playwright && .venv/bin/playwright install chromium` |
| Playwright `Executable doesn't exist` | browsers missing for the Python playwright | `.venv/bin/playwright install chromium` (image sets `PLAYWRIGHT_BROWSERS_PATH` after next rebuild) |
| Orphan-container warnings from other projects | shared compose project namespace | fixed via `name: isms-devenv`; never use `--remove-orphans` here |
