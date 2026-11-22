#!/usr/bin/env bash
# Build the pocketkv image and (re)start it as a background container.
# Data lives in the Docker volume "pocketkv-data", so it survives redeploys.
set -euo pipefail
cd "$(dirname "$0")"

NAME=pocketkv
VOLUME=pocketkv-data

if [[ ! -f .env ]]; then
  echo "Missing .env: run 'cp .env.example .env' and set DASHBOARD_USER / DASHBOARD_PASSWORD." >&2
  exit 1
fi
command -v docker >/dev/null || { echo "Docker is not installed: https://docs.docker.com/engine/install/" >&2; exit 1; }

# Host port (or ip:port) to publish on; read from .env, default 8080.
PORT=$(sed -n 's/^PORT=//p' .env | tail -n 1)
PORT=${PORT:-8080}

docker build -t "$NAME" .
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d \
  --name "$NAME" \
  --restart unless-stopped \
  --env-file .env \
  -e ADDR=:8080 \
  -e DATA_FILE=/data/pocketkv.json \
  -p "$PORT:8080" \
  -v "$VOLUME:/data" \
  "$NAME" >/dev/null

echo "pocketkv is running on port $PORT (logs: docker logs -f $NAME)"
