#!/usr/bin/env sh
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

# Map container user to host UID/GID so bind-mounted files keep their owner.
sed -i "s/^isms\:x\:1000\:1000/isms\:x\:$PUID\:$PGID/" /etc/passwd
sed -i "s/^isms\:x\:1000/isms\:x\:$PGID/" /etc/group

# Named volumes (Go caches) are created root-owned on first use — hand the
# mountpoints to isms so `go run` can write to them. Not -R: contents written
# by isms already have the right owner, and -R on a warm cache is slow.
for d in /home/isms/.cache/go-build /home/isms/.gopath/pkg/mod; do
    [ -d "$d" ] && chown isms:isms "$d"
done

exec gosu isms "$@"
