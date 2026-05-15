#!/bin/bash
# Rebuild DB and start server for testing.
# Usage: bash contrib/test-dbrebuild.sh

set -e

cd ~/Projects/isms

just build
cp bin/isms ~/bin/

# Clean data (repos + blob storage)
rm -rf $HOME/isms/data/repos/*
rm -rf $HOME/isms/data/orgs/*

dropdb acme-isms
createdb acme-isms

~/bin/isms server migrate
~/bin/isms server serve --addr :9090
