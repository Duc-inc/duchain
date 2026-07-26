#!/usr/bin/env bash
# Build the geth binary with the production RandomX cgo binding.
# Requires Go >= 1.24 and librandomx installed (see consensus/randomx/README.md).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$ROOT/build"

echo ">> Building geth (-tags randomx) ..."
CGO_ENABLED=1 go build -tags randomx -o "$ROOT/build/geth-randomx" "$ROOT/cmd/geth"
echo ">> Done: $ROOT/build/geth-randomx"
"$ROOT/build/geth-randomx" version | head -5
