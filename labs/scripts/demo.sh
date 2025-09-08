#!/usr/bin/env bash
set -euo pipefail
FILE="${1:-docker-compose.nodes.yml}"

send_and_show() {
  local src="$1" dst="$2" msg="$3"
  echo "$src -> $dst"
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  docker compose -f "$FILE" exec -T "$src" sh -lc "/app/node send $dst \"$msg\"" # Send the msg from src to dest
  sleep 1
  docker compose -f "$FILE" logs --since "$ts" "$src" "$dst"
  echo
}

send_and_show node01 node42 "MSG1"
send_and_show node07 node13 "MSG2"
send_and_show node13 node07 "MSG3"
