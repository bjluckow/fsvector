#!/usr/bin/env bash
set -euo pipefail

FSVECTOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

docker compose -f "${FSVECTOR_DIR}/docker-compose.yml" up -d
