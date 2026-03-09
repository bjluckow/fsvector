#!/usr/bin/env bash
set -euo pipefail

FSVECTOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "installing fsvector..."
go install "${FSVECTOR_DIR}/cmd/fsvector"
echo "done. run 'fsvector --help' to get started."
