#!/usr/bin/env bash
set -euo pipefail
FILE="${1:-docker-compose.nodes.yml}"

echo "node01 -> node42"
docker compose -f "$FILE" exec -T node01 sh -lc '/app/node send node42 "hello via UDP"'

echo "node07 -> node13"
docker compose -f "$FILE" exec -T node07 sh -lc '/app/node send node13 "ping over UDP"'

echo "node13 -> node07"
docker compose -f "$FILE" exec -T node13 sh -lc '/app/node send node07 "pong over UDP"'

echo
docker compose -f "$FILE" logs --tail=20 node42 node07 node13
