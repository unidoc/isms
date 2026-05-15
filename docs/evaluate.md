# Evaluate ISMS in 10 Minutes

Get a working instance running and explore the core features.

## 1. Start the Server

```bash
# Build
go build -o isms ./cmd/isms/

# Set required env
export DATABASE_URL="postgres://user:pass@localhost/isms?sslmode=disable"
export ISMS_SECRET="your-secret-key"
export ISMS_TEMPLATE_PATH="/path/to/isms-templates"
export ISMS_USER_SIGNUP=1
export ISMS_SKIP_EMAIL_VERIFY=1

# Start server (runs migrations automatically)
./isms server serve --addr :9090
```

## 2. Create Your First Organization

```bash
# Create admin user
./isms server user create --email you@company.com --name "Your Name"
./isms server user set-password --email you@company.com --password changeme

# Create organization
./isms server org create --name "Your Company" --slug myco

# Add yourself as admin
./isms server org add-member --org myco --email you@company.com --role admin
```

## 3. Log In and Scaffold a Template

1. Open `http://localhost:9090` in your browser
2. Log in with your email and password
3. You'll see the Documents view with an empty state
4. Click any template card (e.g. ISO 27001) to scaffold it
5. The document tree populates with the template structure

## 4. Explore the Risk Register

1. Go to **Risks** in the sidebar
2. Click **Add Risk**
3. Pick a category — notice the guided helpers, suggested consequences, and impact areas
4. Set likelihood and impact — the score calculates automatically
5. Save — the risk gets a sequential identifier (RISK-1)

## 5. Send a Document for Review

1. Go to **Documents** and open any document
2. Click **Edit**, make a change, save
3. Click the send icon (paper plane) in the toolbar
4. Write what changed, pick a reviewer, send
5. Open **Reviews** — see the side-by-side comparison
6. Approve or request changes
7. Try the full round cycle: request changes → author edits → resubmit → see "Round 2" badge and "This round" / "All changes" toggle

## What to Look For

- **Review rounds**: After a resubmit, the review shows which round you're in, what changed since the last round, and per-reviewer status for the current round
- **Version history**: Click the clock icon on any document
- **Inline comments**: Click any paragraph to comment
- **Track changes**: In a review, the Changes tab shows rendered diff with "This round" vs "All changes" toggle
- **Decision log**: After merging a review, check the timeline for immutable audit records with round badges
- **Print mode**: Press Cmd+P on any document for a clean printout
- **Risk advisories**: Link a risk to an asset, then check for CIA consistency warnings
- **Multi-standard**: Add a second template (e.g. SOC 2) alongside the first

## Architecture at a Glance

```
Documents  →  Git (markdown + YAML frontmatter)
Everything else  →  PostgreSQL
Templates  →  Disk (ISMS_TEMPLATE_PATH)
```

All configuration lives in PostgreSQL. No config files to manage.

Four roles: **admin** > **manager** > **contributor** > **reader**. Any role can review documents when assigned.
