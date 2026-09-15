#!/usr/bin/env bash
#
# Print the store path cmd/mustur would open, without opening it.
#
# The same order as defaultDB in cmd/mustur/main.go: $MUSTUR_DB, then
# $XDG_DATA_HOME/mustur/mustur.db, then ~/.local/share/mustur/mustur.db. It
# exists so a caller can ask whether a store is there before running the binary,
# because openStore creates a missing database and an empty store answers every
# question with nothing (MUS-D-0183). The path is printed absolute; whether it
# exists is the caller's question.
#
# Usage: scripts/store-path.sh
set -euo pipefail

if [ -n "${MUSTUR_DB:-}" ]; then
  store=$MUSTUR_DB
else
  store="${XDG_DATA_HOME:-$HOME/.local/share}/mustur/mustur.db"
fi
realpath -m "$store"
