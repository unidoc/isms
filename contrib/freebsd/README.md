# Running ISMS on FreeBSD

Works on any FreeBSD system — a bare-metal/VM host or a jail; there is nothing
jail-specific here (a jail is just a FreeBSD userland with the same rc(8) system).

ISMS ships as a single self-contained binary — the web UI and DB migrations are
embedded, so there is nothing else to deploy for the app itself. You still need a
PostgreSQL database (commonly its own jail/host) and the templates repo on disk.

The release tarball `isms_<version>_freebsd_amd64.tar.gz` (also `arm64`) contains:

```
isms              # the binary
rc.d/isms         # the service script (this dir)
isms.env.sample   # configuration sample
FREEBSD.md        # this file
```

## Install

```sh
# In the jail, as root:
fetch https://github.com/unidoc/isms/releases/download/vX.Y.Z/isms_X.Y.Z_freebsd_amd64.tar.gz
tar xzf isms_X.Y.Z_freebsd_amd64.tar.gz

install -d /opt/isms/bin /opt/isms/etc
install -m 755 isms /opt/isms/bin/isms
install -m 755 rc.d/isms /usr/local/etc/rc.d/isms

# Service account (no login, no home — it only runs the daemon):
pw useradd isms -d /nonexistent -s /usr/sbin/nologin -c "ISMS service"

# Data directory: per-org git repos ($ISMS_DATA_DIR/repos/{slug}.git) and, with
# the file storage backend, branding + evidence blobs. Create it and hand it to
# the service user (matches ISMS_DATA_DIR in the env file):
mkdir -p /var/db/isms && chown isms:isms /var/db/isms

# Configuration. Install as root:wheel — NOT owned by the isms user: the rc.d
# script sources this file as root (before dropping privileges), so an env file
# writable by isms would be a local root-escalation path.
install -m 600 -o root -g wheel isms.env.sample /opt/isms/etc/server.env
$EDITOR /opt/isms/etc/server.env        # DATABASE_URL, ISMS_DATA_DIR, ISMS_STORAGE_BACKEND, ISMS_TEMPLATE_PATH
```

## Database + templates

- Create the PostgreSQL role/database and point `DATABASE_URL` at it (note:
  `DATABASE_URL`, not `ISMS_DATABASE_URL`).
- Apply the schema — migrations are embedded:

  ```sh
  env $(grep -v '^#' /opt/isms/etc/server.env | xargs) isms server migrate
  ```

- Put the standard templates repo at `ISMS_TEMPLATE_PATH`.

## Enable + run

```sh
sysrc isms_enable=YES
service isms start
service isms status
```

By default it listens on `:8080` and runs as the `isms` user. To change the
listen address (e.g. bind to loopback behind a reverse proxy):

```sh
sysrc isms_args="server serve --addr 127.0.0.1:8080"
service isms restart
```

## rc.conf variables

| variable        | default                       | meaning                          |
| --------------- | ------------------------------ | -------------------------------- |
| `isms_enable`   | `NO`                           | enable the service               |
| `isms_user`     | `isms`                         | user to run as                   |
| `isms_env_file` | `/opt/isms/etc/server.env`     | file with `DATABASE_URL` / `ISMS_*` config |
| `isms_logfile`  | `/var/log/isms.log`            | where `daemon(8) -o` sends stdout/stderr |
| `isms_args`     | `server serve`                 | `isms` subcommand + flags        |

Logs go to `isms_logfile` via `daemon(8) -o` (not syslog); the pidfile is
`/var/run/isms.pid`. `daemon -r` restarts the process if it exits.

## `pkg install` (later)

This tarball + rc script is all you need to run ISMS in a jail. A native
`pkg install isms` can come later, either from a self-hosted pkg repo or an
official FreeBSD port — tracked separately.
