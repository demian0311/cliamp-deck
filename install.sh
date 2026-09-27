#!/bin/bash
# Install or update cliamp-deck from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/demian0311/cliamp-deck/master/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/demian0311/cliamp-deck/master/install.sh | bash -s -- --uninstall
#
# No sudo: the binary goes to ~/.local/bin (override with BIN_DIR) and the
# launcher entry to ~/.local/share/applications. Re-run to update.

set -euo pipefail

REPO="demian0311/cliamp-deck"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
APP_NAME="cliamp deck"
# omarchy-tui-install always writes here, whatever XDG_DATA_HOME says.
APPS_DIR="$HOME/.local/share/applications"
DESKTOP_FILE="$APPS_DIR/$APP_NAME.desktop"

say() { printf '\e[32m==>\e[0m %s\n' "$*"; }
die() { printf '\e[31merror:\e[0m %s\n' "$*" >&2; exit 1; }

if [[ ${1:-} == --uninstall ]]; then
  rm -f "$BIN_DIR/cliamp-deck" "$DESKTOP_FILE"
  say "Removed cliamp-deck. Its saved EQ and visualizer state is left in ~/.config/cliamp-deck."
  exit 0
fi

case "$(uname -m)" in
  x86_64) asset=cliamp-deck-linux-x86_64 ;;
  aarch64 | arm64) asset=cliamp-deck-linux-aarch64 ;;
  *) die "no prebuilt binary for $(uname -m); build from source: go install github.com/$REPO@latest" ;;
esac

command -v curl >/dev/null || die "curl is required"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

base="https://github.com/$REPO/releases/latest/download"
say "Downloading $asset"
curl -fsSL -o "$tmp/$asset" "$base/$asset"
curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS"
(cd "$tmp" && grep " $asset\$" SHA256SUMS | sha256sum -c --quiet -) || die "checksum mismatch for $asset"

mkdir -p "$BIN_DIR"
install -m755 "$tmp/$asset" "$BIN_DIR/cliamp-deck"
say "Installed $BIN_DIR/cliamp-deck"

# Omarchy's helper tiles the window and applies its TUI window rules; elsewhere
# a plain terminal entry does the same job.
if command -v omarchy-tui-install >/dev/null; then
  omarchy-tui-install "$APP_NAME" "$BIN_DIR/cliamp-deck" tile cliamp >/dev/null
else
  mkdir -p "$APPS_DIR"
  cat >"$DESKTOP_FILE" <<EOF
[Desktop Entry]
Name=$APP_NAME
GenericName=Music Player
Comment=A btop × Winamp terminal face for cliamp, with themed demoscene visualizers
Exec=$BIN_DIR/cliamp-deck
Icon=cliamp
Terminal=true
Type=Application
Categories=Audio;Music;Player;AudioVideo;ConsoleOnly;
StartupNotify=false
EOF
fi
say "Added \"$APP_NAME\" to the app launcher"

command -v cliamp >/dev/null ||
  printf '\e[33mnote:\e[0m cliamp is not installed; the deck needs it (https://github.com/bjarneo/cliamp)\n'

case ":$PATH:" in
  *":$BIN_DIR:"*) say "Run: cliamp-deck" ;;
  *) say "Run: $BIN_DIR/cliamp-deck  ($BIN_DIR is not on your PATH)" ;;
esac
