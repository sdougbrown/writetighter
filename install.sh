#!/usr/bin/env bash
# install.sh — bootstrapping installer for WriteTighter
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/sdougbrown/writetighter/main/install.sh | bash
#
# Downloads the latest release binary for your platform from GitHub Releases
# and installs it to ~/.local/bin (or a directory of your choice via BINDIR).

set -euo pipefail

OWNER="sdougbrown"
REPO="writetighter"
BINDIR="${BINDIR:-${HOME}/.local/bin}"

# --- helpers --------------------------------------------------------------

err()   { printf 'install: %s\n' "$*" >&2; exit 1; }
info()  { printf '  %s\n' "$*"; }

# --- detect platform ------------------------------------------------------

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Darwin) OS="darwin"  ;;
  Linux)  OS="linux"   ;;
  *) err "unsupported OS: $OS" ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) err "unsupported architecture: $ARCH" ;;
esac

info "platform: ${OS}/${ARCH}"

# --- resolve latest release tag -------------------------------------------

printf '  resolving latest release...\n'
TAG="$(curl -fsSL "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
  | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')"

if [ -z "$TAG" ]; then
  err "could not determine latest release tag"
fi

info "latest release: ${TAG}"

# --- download archive -----------------------------------------------------

# goreleaser archive naming: writetighter_Darwin_arm64.tar.gz, writetighter_Linux_x86_64.tar.gz
case "$OS" in
  darwin) ASSET_OS="Darwin"  ;;
  linux)  ASSET_OS="Linux"   ;;
esac
case "$ARCH" in
  amd64) ASSET_ARCH="x86_64" ;;
  arm64) ASSET_ARCH="arm64"  ;;
esac

ASSET="writetighter_${ASSET_OS}_${ASSET_ARCH}.tar.gz"
URL="https://github.com/${OWNER}/${REPO}/releases/download/${TAG}/${ASSET}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

printf '  downloading %s...\n' "$ASSET"
curl -fsSL -o "${TMPDIR}/${ASSET}" "$URL" \
  || err "download failed: $URL"

# --- extract & install ----------------------------------------------------

printf '  extracting...\n'
tar -xzf "${TMPDIR}/${ASSET}" -C "$TMPDIR"

mkdir -p "$BINDIR"
install -m 0755 "${TMPDIR}/writetighter" "${BINDIR}/writetighter"

info "installed writetighter ${TAG} → ${BINDIR}/writetighter"

# --- path hint ------------------------------------------------------------

case ":${PATH}:" in
  *":${BINDIR}:"*)
    info "ready: writetighter --help"
    ;;
  *)
    printf '\n'
    printf '  Add %s to your PATH to use writetighter:\n' "$BINDIR"
    printf '    export PATH="%s:$PATH"\n' "$BINDIR"
    printf '\n'
    ;;
esac