# Releasing ISMS

How we ship. The goal is discipline and an honest trail: small changes,
reviewed, shipped on a steady cadence — not speed without rigor.

## Principles

- **The version number tells the truth.** It is derived from what shipped,
  never chosen for effect.
- **Discipline over churn.** A little every week, reviewed and green, beats
  big irregular drops.
- **Everything is a ticket on a milestone.** The roadmap rolls up from issues;
  anyone can pick up a well-specified ticket and ship it into the next release.

## Cadence — the weekly train

We ship on a weekly cadence (a release train): whatever is merged, reviewed,
and green goes out that week.

- The **train is time-based**; the **version is content-based**.
- A week with only bug fixes ships a **patch**. A week that lands a feature
  ships a **minor**.
- Do not force "a minor every week" — that makes the number lie. Ship weekly;
  let the content name the release.
- **The cadence is a rhythm, not a quota.** Aim for roughly weekly while
  actively working; skip quiet weeks and vacations without guilt. Forcing out
  an empty release to hit a date is the real failure — it ships churn and makes
  the version lie. The quality bar never flexes; the calendar does.

## Versioning (semver, honestly applied)

| Bump | When | Examples |
|------|------|----------|
| **patch** `0.6.1` | bug fixes only | print fix, table render, owner-change |
| **minor** `0.7.0` | new features | per-event review diff, mobile polish |
| **major** `1.0.0` | cornerstone capability / architectural milestone | Trust center; 1.0 = core feature-complete and stable |

Rule of thumb: **fix → patch, feature → minor.** You never have to ask
"patch or minor" — the work decides. `1.0` is reached on substance (the core
is feature-complete and stable), not as a marketing number.

**Schema changes ship in feature releases.** Anything that needs a database
migration goes in a **minor**, never a patch. Patches stay migration-free —
trivially safe to ship and to roll back.

The one exception: a **serious security issue** (or active data loss) whose fix
requires a migration may ship in a patch — security comes first. Any such
exception must be called out explicitly in that release.

## Release focus (themes)

Each minor has a single headline focus, decided ahead of time. The focus and
the tickets behind it live in the **GitHub milestone description** — that is the
single source of truth, not this document. Patches don't need a theme; they are
whatever fixes are ready that week.

## Scope per release

Keep each release small, coherent, and shippable.

- Solo / small-team guardrail: **~2 tickets per patch.** This is a guardrail,
  not a law — the real test is "small, coherent, shippable."
- With more contributors, throughput rises and the **train schedule** — not a
  ticket count — becomes the structure; releases naturally carry more.

## Milestones and backlog

A milestone is a **commitment for the next 1–2 releases**, not a wish list.

- Assign only the next patch/minor's tickets to a milestone.
- Everything else stays in the **backlog** (no milestone) and is pulled in
  two-at-a-time as a release is started.
- bug → patch milestone · feature → minor milestone · cornerstone → major.

## Mechanics

A two-step, PR-based flow (recipes in the root `Justfile`):

```
just release-pr X.Y.Z   # version-bump PR (reviewed, CI runs)
# … merge the PR …
just release X.Y.Z      # verifies master carries the version, signs the tag,
                        # pushes — CI (goreleaser) publishes the GitHub Release
```

- `just snapshot` builds the release artifacts locally (dry run, nothing
  published) — use it to test the pipeline before tagging.
- Binaries: static `linux/amd64`, `linux/arm64`, macOS universal,
  `openbsd/amd64`, `openbsd/arm64`, with `checksums.txt` and a changelog.
- **Tag from a human, build from GitHub.** Releases are built and published in
  CI, never from a laptop — reproducible, traceable, no local secrets.

## Gates (non-negotiable)

- **`master`**: PR + 1 approval + green CI (`go-unit`, `integration`) + signed
  commits. Stale approvals are dismissed on new pushes, and the most recent
  push must be approved by someone other than its author. No force-push, no
  deletion.
- **`v*` tags**: creation / update / deletion restricted to admins (ruleset),
  and signed.

## Branching — master is sacred

`master` is always releasable. Every merge keeps it stable and shippable —
that is exactly what the gates protect. A release tag is simply a snapshot of
`master` at release time; `master` is the always-ready next release, never a
place where work-in-progress lands.

- Never push anything to `master` you wouldn't ship.
- All work happens on branches and lands via reviewed, green PRs.
- The weekly patch is cut from `master` with `just release`.

When we later need to patch an older line while a newer one moves on (security
errata on 0.6.x while 0.7 develops), we will cut a `X.Y-stable` branch. Until
then it is trunk-based: one sacred `master`.

## House style

- **Secure and correct by default.** Safety is the default state, not a flag.
- **A small core, and we remove code.** Deleting a path — a dead vendor
  integration, an unused field — is a win, not a loss.
- **A gentle rhythm, not a forced march.** Ship roughly weekly when active;
  skip quiet weeks without guilt. The quality bar never relaxes; the calendar
  does.
- **Reliable errata.** Security and reliability fixes land as clean patch
  releases, promptly.
- **Rigor lives in the process, warmth lives in the people.** CI, rulesets and
  signed gates are the uncompromising gatekeeper — so reviewers can be exacting
  about the code and generous with the contributor. New contributors are
  welcome; every release is worth celebrating.

## Why this way

Every change is a reviewed ticket on a milestone; the roadmap is just the issue
state rolled up; the build is reproducible in CI. That transparency is
deliberate — a steady, honest trail anyone can read and contribute to, rather
than undisciplined shipping. Steadiness compounds.
