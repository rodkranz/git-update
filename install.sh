#!/usr/bin/env bash
set -euo pipefail

DEST="${1:-$HOME/bin}"
mkdir -p "$DEST"
go build -trimpath -o "$DEST/git-update" .
echo "Installed: $DEST/git-update"
echo "Run: git update /path/to/projects"
