# One enforced write path per entity (#26)

## The problem

Business rules (RBAC, guards, lifecycle timestamps, notifications) historically
lived only in the HTTP handler layer (`api_*.go`). But entities have **more than
one writer**:

- HTTP handlers (`handleUpdateX`)
- suggestion-apply (`api_suggestions.go`, runs inside `WithOrgTx`, calls the
  DB-layer `*Tx` functions directly)
- CLI / MCP (via the HTTP API)

When suggestion-apply called `db.UpdateXTx` directly it skipped every rule that
lived in the handler. Since #23 makes suggestion-apply the primary write path for
contributors and agents, the *main* input path was the *least* enforced one.
That divergence is the root of a whole cluster of integrity bugs.

## The pattern

**One enforced, transaction-aware mutation function per entity. Every writer
calls it inside a transaction.** Rules can't diverge because there's exactly one
place that enforces them.

```
enforceXWriteTx(ctx, tx, orgID, entity, prevState) error
  ├─ guards / RBAC-independent business rules   (e.g. open-CA guard)
  ├─ the row write                              (db.UpdateXTx)
  └─ derived state                              (e.g. lifecycle timestamps)
```

- Lives in the `api` package (it composes DB `*Tx` calls + business rules), named
  `enforceXWriteTx`.
- Takes a `pgx.Tx` — the caller owns the transaction (`WithOrgTx`), so the whole
  mutation is atomic.
- Returns typed errors for rule violations (e.g. `openCAsLinkedError`) so the HTTP
  handler can map them to the right status (409) while suggestion-apply surfaces
  them as the apply failure.

## Migrated entities

- **incident** — `enforceIncidentWriteTx` (open-CA guard + lifecycle timestamps).
- **corrective_action** — `enforceCorrectiveActionWriteTx` (open-task guard +
  `resolved_at`/`resolved_by_id`) and `applyCorrectiveActionDefaults` (shared
  create defaults, so apply and HTTP seed the same starting state).

Together these close the three integrity bypasses #26 names. Remaining entities
follow the same recipe, one at a time.

## Reference implementation — incident

- `enforceIncidentWriteTx` — `internal/isms/api/api_incidents_writepath.go`
- DB primitives — `internal/isms/db/suggestions_tx.go`:
  `CountOpenCAsByIncidentTx`, `SetIncidentLifecycleTx`, `UpdateIncidentTx`
- Callers:
  - `handleUpdateIncident` (`api_incidents.go`) wraps it in `WithOrgTx`, maps
    `openCAsLinkedError` → HTTP 409.
  - `applyIncidentUpdate` (`api_suggestions.go`) calls it inside the existing
    apply transaction.
- Parity test — `tests/test_incident_writepath.py`: the open-CA guard and the
  `resolved_at` timestamp now fire identically on the HTTP and apply paths.

## Adding the next entity

1. Write `enforceXWriteTx(ctx, tx, orgID, x, prev)` capturing the rules currently
   inlined in `handleUpdateX` (guards, derived timestamps, status transitions).
   Add DB `*Tx` primitives for anything that touched `d.pool`.
2. Route `handleUpdateX` through `WithOrgTx(enforceXWriteTx)`; map rule errors to
   HTTP statuses.
3. Route `applyXUpdate` (and `applyXCreate` where rules apply) through the same
   function.
4. Add a parity test asserting identical behavior (rules, timestamps, RBAC) on
   both paths.

Do **not** convert all entities at once — one at a time, each with its parity
test. This is an incremental epic (#26); incident is the first slice.
