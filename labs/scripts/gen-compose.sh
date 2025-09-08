#!/usr/bin/env bash
set -euo pipefail
NODES="${1:-50}"
OUT="${2:-docker-compose.nodes.yml}"

cat > "$OUT" <<'YAML'
services:
YAML

for i in $(seq -f "%02g" 1 "$NODES"); do # Loop for creating our 50 nodes
  name="node${i}"
  cat >> "$OUT" <<YAML
  ${name}:
    image: kadlab:latest
    hostname: ${name}
    container_name: ${name}
    environment:
      - NODE_NAME=${name}
      - PORT=9999
      - AUTO_ACK=1
    expose:
      - "9999/udp"
    networks: [kadnet]
    restart: unless-stopped
YAML
done

cat >> "$OUT" <<'YAML'
networks:
  kadnet:
YAML

echo "Wrote $OUT with $NODES nodes."
