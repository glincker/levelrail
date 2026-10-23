#!/usr/bin/env bash
# Local SSO lab: Keycloak and SimpleSAMLphp in Docker, lab credentials only.
# Usage: scripts/sso-lab.sh up|down|status|env
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
lab="$root/test/sso-lab"
compose=(docker compose -f "$lab/docker-compose.yml")
kc="${LEVELRAIL_LIVE_KEYCLOAK_URL:-https://localhost:18443}"
ssp="${LEVELRAIL_LIVE_SSP_URL:-https://localhost:18182}"

certs() {
  [ -f "$lab/certs/tls.crt" ] && return 0
  mkdir -p "$lab/certs"
  openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
    -keyout "$lab/certs/tls.key" -out "$lab/certs/tls.crt" >/dev/null 2>&1
  chmod 644 "$lab/certs/tls.key"
}

print_env() {
  cat <<EOT
Keycloak 26.0.8   admin console   $kc  (admin / admin)
  OIDC issuer      http://localhost:18080/realms/lab
  OIDC client      lab-oidc / lab-secret  (code + PKCE S256)
  SAML metadata    $kc/realms/lab/protocol/saml/descriptor
  SAML client      lab-saml-sp (wildcard redirect, assertion signed)
  Users (realm lab, email domain lab.example)
    ada     ada-pass     ada@lab.example         group Engineering
    grace   grace-pass   grace@lab.example       group Admins
    mallory mallory-pass mallory@other.example   group Engineering (outside the lab domain)
SimpleSAMLphp 1.15  metadata  $ssp/simplesaml/saml2/idp/metadata.php
    user1 / user1pass  (group1), user2 / user2pass  (group2)

Run the live tests:
  export LEVELRAIL_LIVE_IDP=1
  CGO_ENABLED=0 go test ./internal/api -run 'TestLive' -count=1 -v
EOT
}

wait_ready() {
  local url="$1" name="$2" i
  for i in $(seq 1 90); do
    if curl -ksf -o /dev/null "$url"; then echo "$name ready"; return 0; fi
    sleep 2
  done
  echo "$name did not become ready in 180s" >&2
  return 1
}

case "${1:-}" in
  up)
    certs
    "${compose[@]}" up -d
    wait_ready "$kc/realms/lab/.well-known/openid-configuration" keycloak
    wait_ready "$ssp/simplesaml/saml2/idp/metadata.php" simplesamlphp
    print_env
    ;;
  down)
    "${compose[@]}" down -v --remove-orphans
    ;;
  status)
    "${compose[@]}" ps
    curl -ksf -o /dev/null "$kc/realms/lab/.well-known/openid-configuration" && echo "keycloak: healthy" || echo "keycloak: down"
    curl -ksf -o /dev/null "$ssp/simplesaml/saml2/idp/metadata.php" && echo "simplesamlphp: healthy" || echo "simplesamlphp: down"
    ;;
  env)
    print_env
    ;;
  *)
    echo "usage: $0 up|down|status|env" >&2
    exit 2
    ;;
esac
