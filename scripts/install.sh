#!/usr/bin/env bash
set -euo pipefail

FSVECTOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${HOME}/bin"
WRAPPER="${BIN_DIR}/fsvector"

echo "fsvector installer"
echo "  repo dir : $FSVECTOR_DIR"
echo "  bin dir  : $BIN_DIR"

# create ~/bin if it doesn't exist
mkdir -p "$BIN_DIR"

# write the shell wrapper
cat > "$WRAPPER" <<EOF
#!/usr/bin/env bash
set -euo pipefail

FSVECTOR_DIR="${FSVECTOR_DIR}"

# load .env if present
if [ -f "\${FSVECTOR_DIR}/.env" ]; then
  set -a
  source "\${FSVECTOR_DIR}/.env"
  set +a
fi

CMD="\${1:-}"

case "\$CMD" in
  daemon)
    SUBCMD="\${2:-}"
    case "\$SUBCMD" in
      start)
        echo "starting fsvectord stack..."
        docker compose -f "\${FSVECTOR_DIR}/docker-compose.yml" up -d postgres embedsvc convertsvc
        echo "waiting for services to be healthy..."
        docker compose -f "\${FSVECTOR_DIR}/docker-compose.yml" wait postgres embedsvc convertsvc 2>/dev/null || true
        echo "starting fsvectord..."
        docker compose -f "\${FSVECTOR_DIR}/docker-compose.yml" up -d fsvectord
        ;;
      stop)
        docker compose -f "\${FSVECTOR_DIR}/docker-compose.yml" down
        ;;
      status)
        docker compose -f "\${FSVECTOR_DIR}/docker-compose.yml" ps
        ;;
      logs)
        docker compose -f "\${FSVECTOR_DIR}/docker-compose.yml" logs -f fsvectord
        ;;
      *)
        echo "usage: fsvector daemon <start|stop|status|logs>"
        exit 1
        ;;
    esac
    ;;
  *)
    # pass all args through to the fsvector query binary
    docker compose -f "\${FSVECTOR_DIR}/docker-compose.yml" run --rm fsvector "\$@"
    ;;
esac
EOF

chmod +x "$WRAPPER"

echo ""
echo "installed to $WRAPPER"

# check if ~/bin is on PATH
if [[ ":$PATH:" != *":${BIN_DIR}:"* ]]; then
  echo ""
  echo "  WARNING: ${BIN_DIR} is not on your PATH."
  echo "  Add this to your ~/.zshrc or ~/.bashrc:"
  echo ""
  echo "    export PATH=\"\$PATH:${HOME}/bin\""
  echo ""
fi

echo "done. run 'fsvector --help' to get started."
