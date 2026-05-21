#!/bin/sh
# flotilla installer — full VPS bootstrap.
#
#   curl -fsSL https://raw.githubusercontent.com/DmitriyKurilenko/flotilla/main/install.sh | sh
#
# Interactive by design. No flags, no non-interactive mode: it prompts
# once for the Let's Encrypt email and proceeds. See
# docs/ARCHITECTURE.md §4.1 for the contract this script implements.
#
# Idempotent: re-running upgrades the binary and re-verifies the
# ingress; it never destroys project state.

set -eu

REPO_OWNER="DmitriyKurilenko"
REPO_NAME="flotilla"
RAW_BASE="https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/main"
API_BASE="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}"
TRAEFIK_DIR="/opt/traefik"
BIN_DEST="/usr/local/bin/flotilla"

# ─── output helpers ──────────────────────────────────────────────────
if [ -t 1 ]; then
  C_OK='\033[0;32m'; C_ERR='\033[0;31m'; C_INFO='\033[1;33m'; C_OFF='\033[0m'
else
  C_OK=''; C_ERR=''; C_INFO=''; C_OFF=''
fi
ok()   { printf "${C_OK}✓ %s${C_OFF}\n" "$1"; }
info() { printf "${C_INFO}→ %s${C_OFF}\n" "$1"; }
die()  { printf "${C_ERR}✗ %s${C_OFF}\n" "$1" >&2; exit 1; }

need_root() {
  if [ "$(id -u)" -ne 0 ]; then
    die "install.sh must run as root (it installs Docker, writes /opt and /usr/local/bin). Re-run with sudo."
  fi
}

# ─── step 1: detect OS/arch ──────────────────────────────────────────
OS=""
ARCH=""
OS_ID=""
detect_platform() {
  uname_s="$(uname -s)"
  case "$uname_s" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *) die "unsupported OS: $uname_s (flotilla targets Linux servers; macOS is dev-only)" ;;
  esac

  uname_m="$(uname -m)"
  case "$uname_m" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) die "unsupported architecture: $uname_m" ;;
  esac

  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    OS_ID="$(. /etc/os-release; echo "${ID:-}")"
  fi
  ok "platform: ${OS}/${ARCH}${OS_ID:+ (${OS_ID})}"
}

# ─── step 2: Docker ──────────────────────────────────────────────────
ensure_docker() {
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    # Verify Docker is new enough for the API version Traefik expects.
    ver_major="$(docker version --format '{{.Server.Version}}' | cut -d. -f1)"
    ver_minor="$(docker version --format '{{.Server.Version}}' | cut -d. -f2)"
    if [ "$ver_major" -lt 25 ] || { [ "$ver_major" -eq 25 ] && [ "$ver_minor" -lt 0 ]; }; then
      die "Docker ${ver_major}.${ver_minor} is too old. Minimum required is 25.0. Upgrade Docker and re-run install.sh."
    fi
    ok "Docker present (${ver_major}.${ver_minor})"
    return
  fi
  case "$OS_ID" in
    debian|ubuntu)
      info "Docker not found — installing via https://get.docker.com"
      curl -fsSL https://get.docker.com | sh
      docker info >/dev/null 2>&1 || die "Docker installed but the daemon is not responding"
      ok "Docker installed"
      ;;
    *)
      die "Docker is required and not installed. Install Docker for your platform, then re-run this installer."
      ;;
  esac
}

# ─── step 3: Compose plugin ──────────────────────────────────────────
ensure_compose() {
  if docker compose version >/dev/null 2>&1; then
    ok "Docker Compose plugin present"
    return
  fi
  die "The Docker Compose plugin is missing. Install it (e.g. 'apt-get install docker-compose-plugin') and re-run."
}

# ─── step 4: ACME email (interactive, tty required) ──────────────────
ACME_EMAIL=""
prompt_email() {
  if [ ! -e /dev/tty ]; then
    die "install.sh needs a terminal to ask for the Let's Encrypt email. Run it on a tty (no piped/non-interactive mode)."
  fi
  while [ -z "$ACME_EMAIL" ]; do
    printf "Email for Let's Encrypt expiry notifications: " > /dev/tty
    IFS= read -r ACME_EMAIL < /dev/tty || die "no input received (a terminal is required)"
    ACME_EMAIL="$(printf '%s' "$ACME_EMAIL" | tr -d '[:space:]')"
  done
  ok "ACME email: ${ACME_EMAIL}"
}

# ─── step 5: proxy network ───────────────────────────────────────────
ensure_proxy_network() {
  if docker network inspect proxy >/dev/null 2>&1; then
    ok "Docker network 'proxy' exists"
  else
    docker network create proxy >/dev/null
    ok "created Docker network 'proxy'"
  fi
}

# ─── step 6: ingress detection ───────────────────────────────────────
# Sets INGRESS_STATE to one of: reuse | deploy | conflict
INGRESS_STATE="deploy"
detect_ingress() {
  existing="$(docker ps --filter 'label=flotilla.role=ingress' --format '{{.ID}}' 2>/dev/null || true)"
  if [ -z "$existing" ]; then
    existing="$(docker ps --filter 'ancestor=traefik' --format '{{.ID}}' 2>/dev/null | head -n1 || true)"
  fi
  if [ -n "$existing" ]; then
    # Verify the existing Traefik has Docker provider enabled; without
    # it flotilla projects' labels are invisible to the ingress.
    has_docker_provider="$(docker inspect --format='{{range .Config.Cmd}}{{.}} {{end}}' "$existing" 2>/dev/null | grep -o 'providers.docker=true' || true)"
    if [ -z "$has_docker_provider" ]; then
      existing_name="$(docker inspect --format='{{.Name}}' "$existing" 2>/dev/null | sed 's|^/||')"
      [ -z "$existing_name" ] && existing_name="$existing"
      info "removing existing Traefik (container ${existing_name}) — it does not have Docker provider and will not work with flotilla"
      docker stop "$existing_name" >/dev/null 2>&1 || true
      docker rm   "$existing_name" >/dev/null 2>&1 || true
      ok "removed ${existing_name}"
      INGRESS_STATE="deploy"
      return
    fi
    INGRESS_STATE="reuse"
    ok "existing Traefik detected (container ${existing}); reusing it"
    return
  fi

  # Any container bound to host port 80, 443, or 8080?
  # 8080 is Traefik dashboard/API (127.0.0.1:8080); if occupied,
  # `docker compose up` for Traefik will fail with a cryptic error.
  busy="$(docker ps --format '{{.Names}} {{.Ports}}' 2>/dev/null | grep -E ':80->|:443->|:8080->' || true)"
  if [ -n "$busy" ]; then
    # Extract every conflicting Docker container name and remove them.
    offender_names="$(printf '%s' "$busy" | awk '{print $1}')"
    info "removing containers that block ports 80/443/8080"
    printf '%s\n' "$offender_names" | while IFS= read -r name; do
      [ -z "$name" ] && continue
      docker stop "$name" >/dev/null 2>&1 || true
      docker rm   "$name" >/dev/null 2>&1 || true
      ok "removed ${name}"
    done
    INGRESS_STATE="deploy"
    return
  fi
  INGRESS_STATE="deploy"
}

# ─── step 7: deploy Traefik ──────────────────────────────────────────
deploy_traefik() {
  if [ "$INGRESS_STATE" = "reuse" ]; then
    info "skipping Traefik deploy (reusing existing ingress)"
    return
  fi

  mkdir -p "${TRAEFIK_DIR}/letsencrypt" "${TRAEFIK_DIR}/logs"

  info "fetching Traefik bundle"
  if curl -fsSL "${RAW_BASE}/embed/traefik/compose.yml" -o "${TRAEFIK_DIR}/compose.yml" 2>/dev/null; then
    : # downloaded from GitHub
  elif [ -f "./embed/traefik/compose.yml" ]; then
    info "GitHub raw not available (project not published yet); copying local embed/traefik/compose.yml"
    cp "./embed/traefik/compose.yml" "${TRAEFIK_DIR}/compose.yml"
  else
    die "could not download embed/traefik/compose.yml"
  fi

  # .env carries ACME_EMAIL. Rewrite each run so the email stays current.
  printf 'ACME_EMAIL=%s\n' "$ACME_EMAIL" > "${TRAEFIK_DIR}/.env"
  chmod 600 "${TRAEFIK_DIR}/.env"

  info "starting Traefik"
  ( cd "$TRAEFIK_DIR" && docker compose --env-file .env -f compose.yml up -d ) \
    || die "docker compose up failed for Traefik"

  info "waiting for Traefik API (max 60s)"
  i=0
  while [ "$i" -lt 30 ]; do
    if curl -fsS -o /dev/null "http://127.0.0.1:8080/api/version" 2>/dev/null; then
      ok "Traefik is up"
      return
    fi
    i=$((i + 1))
    sleep 2
  done
  die "Traefik did not become reachable on 127.0.0.1:8080 within 60s — check 'docker logs flotilla-traefik'"
}

# ─── step 8: install flotilla binary ─────────────────────────────────
install_binary() {
  info "resolving latest flotilla release"
  tag="$(curl -fsSL "${API_BASE}/releases/latest" 2>/dev/null \
        | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/' || true)"
  if [ -z "$tag" ]; then
    die "could not determine the latest release tag (no published release yet?). Build from source with 'make build' until v0.1 is tagged."
  fi
  ver="${tag#v}"

  asset="${REPO_NAME}_${ver}_${OS}_${ARCH}.tar.gz"
  base="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${tag}"

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  info "downloading ${asset}"
  curl -fsSL "${base}/${asset}" -o "${tmp}/${asset}" || die "download failed: ${base}/${asset}"
  curl -fsSL "${base}/checksums.txt" -o "${tmp}/checksums.txt" || die "could not fetch checksums.txt"

  info "verifying SHA-256"
  expected="$(grep " ${asset}\$" "${tmp}/checksums.txt" | awk '{print $1}')"
  [ -n "$expected" ] || die "no checksum entry for ${asset}"
  actual="$(sha256sum "${tmp}/${asset}" | awk '{print $1}')"
  [ "$expected" = "$actual" ] || die "checksum mismatch for ${asset} (expected ${expected}, got ${actual})"

  tar -xzf "${tmp}/${asset}" -C "$tmp"
  install -m 0755 "${tmp}/flotilla" "$BIN_DEST" || die "could not install to ${BIN_DEST}"
  ok "flotilla installed: $("$BIN_DEST" --version)"
}

# ─── main ────────────────────────────────────────────────────────────
main() {
  need_root
  detect_platform
  ensure_docker
  ensure_compose
  prompt_email
  ensure_proxy_network
  detect_ingress
  deploy_traefik
  install_binary

  printf "\n"
  ok "flotilla is ready."
  printf "Next: %s\n" "flotilla deploy /opt/<your-project>"
}

main "$@"
