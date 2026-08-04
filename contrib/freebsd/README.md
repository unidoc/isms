# Running ISMS on FreeBSD (jails)

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

install -m 755 isms /usr/local/bin/isms
install -m 755 rc.d/isms /usr/local/etc/rc.d/isms

# Service account (no login, no home needed — git storage lives under a data dir
# you choose via config):
pw useradd isms -d /nonexistent -s /usr/sbin/nologin -c "ISMS service"

# Configuration:
install -m 600 -o isms isms.env.sample /usr/local/etc/isms.env
$EDITOR /usr/local/etc/isms.env        # set DATABASE_URL, ISMS_TEMPLATE_PATH, ISMS_BASE_URL
```

## Database + templates

- Create the PostgreSQL role/database and point `DATABASE_URL` at it (note:
  `DATABASE_URL`, not `ISMS_DATABASE_URL`).
- Apply the schema — migrations are embedded:

  ```sh
  env $(grep -v '^#' /usr/local/etc/isms.env | xargs) isms migrate
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
sysrc isms_args="server --addr 127.0.0.1:8080"
service isms restart
```

## rc.conf variables

| variable        | default                      | meaning                          |
| --------------- | ---------------------------- | -------------------------------- |
| `isms_enable`   | `NO`                         | enable the service               |
| `isms_user`     | `isms`                       | user to run as                   |
| `isms_env_file` | `/usr/local/etc/isms.env`    | file with `ISMS_*` / `DATABASE_URL` config |
| `isms_args`     | `server`                     | `isms` subcommand + flags        |

Logs go to syslog via `daemon(8)`; the pidfile is `/var/run/isms.pid`.

## `pkg install` (later)

This tarball + rc script is all you need to run ISMS in a jail. A native
`pkg install isms` can come later, either from a self-hosted pkg repo or an
official FreeBSD port — tracked separately.
