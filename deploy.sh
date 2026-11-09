#!/usr/bin/env bash
# Builds the API image on this machine and ships it to the VPS over SSH.
# No registry, no CI: the server only loads images and restarts the API container.
#
#   ./deploy.sh            build the current commit, ship it, restart, check it answers
#   ./deploy.sh rollback   go back to the image that ran before the last deploy
#   ./deploy.sh status     what the server is running
#
# Run it from WSL (it needs docker, ssh and curl). Only committed code is deployed:
# the image is built from `git archive HEAD`, never from the working tree.
set -euo pipefail

HOST="${DEPLOY_HOST:-root@161.97.98.143}"
DIR="${DEPLOY_DIR:-/root/simple-to-do}"
URL="${DEPLOY_URL:-https://todo.loket.site}"
IMAGE=todoapp-api
COMPOSE=docker-compose.prod.yml

cd "$(dirname "$0")"

say() { printf '\033[1m==> %s\033[0m\n' "$*"; }
die() { printf '\033[31merror: %s\033[0m\n' "$*" >&2; exit 1; }

remote() { ssh -o BatchMode=yes "$HOST" "$@"; }

# Restarts the API on whatever todoapp-api:latest is on the server.
restart() {
  remote "cd '$DIR' && docker compose -f $COMPOSE up -d --no-build app"
}

# /api/me without a token answers 401 once the server is up (404 means old code).
healthy() {
  local code
  for _ in $(seq 1 30); do
    code=$(curl -s -o /dev/null -w '%{http_code}' "$URL/api/me" || true)
    [ "$code" = 401 ] && return 0
    sleep 1
  done
  echo "last answer from $URL/api/me: ${code:-none}" >&2
  return 1
}

status() {
  remote "cd '$DIR' && echo \"deployed: \$(cat DEPLOYED 2>/dev/null || echo unknown)\" \
    && docker ps --filter name=todoapp-api-prod --format '{{.Image}}  {{.Status}}' \
    && docker images $IMAGE --format '{{.Tag}}  {{.ID}}  {{.CreatedSince}}'"
}

rollback() {
  say "Rolling back to the previous image"
  remote "docker image inspect $IMAGE:previous >/dev/null 2>&1" || die "no previous image on the server"
  remote "docker tag $IMAGE:previous $IMAGE:latest && echo rollback > '$DIR/DEPLOYED'"
  restart
  healthy || die "the API isn't answering after the rollback; check: ssh $HOST docker logs todoapp-api-prod"
  say "Rolled back"
}

deploy() {
  git diff --quiet HEAD -- || die "uncommitted changes; commit them first (only committed code is deployed)"
  local sha; sha=$(git rev-parse --short HEAD)
  say "Building $IMAGE:$sha ($(git log -1 --format=%s))"
  git archive --format=tar HEAD | docker build -q -t "$IMAGE:$sha" - >/dev/null

  say "Shipping the image to $HOST"
  docker save "$IMAGE:$sha" | gzip -1 | remote "gunzip | docker load -q"
  git show "HEAD:$COMPOSE" | remote "cat > '$DIR/$COMPOSE'"

  say "Restarting the API"
  # Keep what's running now as :previous, so a bad deploy can go straight back.
  remote "set -e
    if id=\$(docker inspect -f '{{.Image}}' todoapp-api-prod 2>/dev/null); then docker tag \"\$id\" $IMAGE:previous; fi
    docker tag $IMAGE:$sha $IMAGE:latest
    echo $sha > '$DIR/DEPLOYED'"
  restart

  say "Checking $URL"
  if ! healthy; then
    remote "docker logs --tail 20 todoapp-api-prod" >&2 || true
    printf '\033[31mThe new version is not answering.\033[0m\n' >&2
    rollback
    exit 1
  fi
  # Old images pile up otherwise; keep latest, previous and the last few builds.
  remote "docker images $IMAGE --format '{{.Tag}}' | grep -vE '^(latest|previous)\$' | tail -n +4 | sed 's/^/$IMAGE:/' | xargs -r docker rmi -f >/dev/null" || true
  say "Deployed $sha"
}

case "${1:-deploy}" in
  deploy) deploy ;;
  rollback) rollback ;;
  status) status ;;
  *) die "usage: $0 [deploy|rollback|status]" ;;
esac
