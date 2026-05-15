#!/usr/bin/env sh
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

# Map container user to host UID/GID
sed -i "s/^tower\:x\:1000\:1000/tower\:x\:$PUID\:$PGID/" /etc/passwd
sed -i "s/^tower\:x\:1000/tower\:x\:$PGID/" /etc/group

exec gosu tower "$@"
