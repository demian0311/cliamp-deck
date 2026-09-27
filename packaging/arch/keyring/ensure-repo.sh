#!/bin/bash
#
# Adds or removes the [cliamp-deck] stanza in /etc/pacman.conf, idempotently.
#
#   ensure-repo.sh            add it if absent   (post_install, post_upgrade, hook)
#   ensure-repo.sh --remove   take it out again  (pre_remove)
#
# 🔴 Never rewrites the file in place with sed. A user's pacman.conf is not ours
# and a partial write to it is a machine that cannot install anything — so the
# edit is built in a temporary file next to the target and moved over it, which
# is atomic on the same filesystem.
#
# Ported from dgmo's packaging/arch/keyring/ensure-repo.sh (diagrammo ->
# cliamp-deck rename only; the awk mechanics are unchanged and still covered
# by test-ensure-repo.sh).

set -euo pipefail

CONF=${CLIAMP_DECK_PACMAN_CONF:-/etc/pacman.conf}
SERVER='https://github.com/demian0311/cliamp-deck/releases/download/arch-repo'
SECTION='[cliamp-deck]'

[[ -f $CONF ]] || exit 0

has_section() {
  grep -qE '^[[:space:]]*\[cliamp-deck\][[:space:]]*$' "$CONF"
}

case ${1:-} in
--remove)
  has_section || exit 0
  tmp=$(mktemp "${CONF}.cliamp-deck.XXXXXX")
  trap 'rm -f "$tmp"' EXIT
  # Drops the section header and every line under it until the next section or
  # end of file, plus one blank line immediately before it if there is one —
  # which is the shape post_install writes, so removing leaves the file as it
  # was found rather than accumulating blank lines over install/remove cycles.
  # 🔴 Blank lines are COUNTED, never buffered as text. An awk variable holding
  # a blank line holds the empty string, which is indistinguishable from "no
  # line held" — a buffering version silently deletes every blank line in the
  # file while passing every other test. See test-ensure-repo.sh case 3.
  awk '
    /^[[:space:]]*$/ && !dropping                          { pending++; next }
    /^[[:space:]]*\[cliamp-deck\][[:space:]]*$/            { if (pending > 0) pending--; dropping = 1; next }
    dropping && /^[[:space:]]*\[/                          { dropping = 0 }
    dropping                                                { next }
    { while (pending > 0) { print ""; pending-- } print }
    END { while (pending > 0) { print ""; pending-- } }
  ' "$CONF" >"$tmp"
  # A rewrite that lost the [options] section means the awk above went wrong;
  # leaving the original in place is always the safer failure.
  if ! grep -qE '^[[:space:]]*\[options\][[:space:]]*$' "$tmp"; then
    echo "cliamp-deck: refusing to rewrite $CONF — [options] went missing" >&2
    exit 1
  fi
  chmod --reference="$CONF" "$tmp" 2>/dev/null || chmod 644 "$tmp"
  mv -f "$tmp" "$CONF"
  trap - EXIT
  echo "cliamp-deck: removed $SECTION from $CONF"
  ;;
*)
  has_section && exit 0
  printf '\n%s\nServer = %s\n' "$SECTION" "$SERVER" >>"$CONF"
  echo "cliamp-deck: added $SECTION to $CONF — run 'pacman -Sy' to pick it up"
  ;;
esac
