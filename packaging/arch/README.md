# packaging/arch — the `[cliamp-deck]` pacman repository

Ported 2026-09-27 from diagrammo/dgmo's `packaging/arch/` +
`.github/workflows/arch-repo.yml` (that repo packages an npm CLI; this one
repackages this repo's own prebuilt release binaries — no build step). Written
for the next agent, not a human reader.

## Layout

- `PKGBUILD` — `cliamp-deck` package. Repackages the prebuilt
  `cliamp-deck-linux-{x86_64,aarch64}` release assets (no `go build`).
  `arch=(x86_64 aarch64)`.
- `cliamp-deck.desktop` — local copy of `packaging/aur/cliamp-deck.desktop`.
  **Keep the two in sync by hand**, there is no shared source.
- LICENSE — fetched from the release tag at build time, not copied in.
- `keyring/` — `cliamp-deck-keyring`, the bootstrap package. See below.
- `../../.github/workflows/arch-repo.yml` — builds and publishes everything.

`packaging/aur/` is UNRELATED (source build via `go build`, for an eventual AUR
submission) — do not conflate the two PKGBUILDs.

## How it works

The `arch-repo` release on this repo (github.com/demian0311/cliamp-deck) IS
the pacman repository: its assets are the package(s), the repo database
(`cliamp-deck.db`/`.files` + `.tar.gz` real-file copies, since `repo-add`
leaves the bare name a symlink and a GitHub asset can't be one), and their
`.sig` files, replaced in place on every run. pacman follows GitHub's asset
redirect (this is the mechanism dgmo measured 2026-09-17 on Arch, unchanged
here) — no bucket, no custom domain, no credential beyond `GITHUB_TOKEN`.

Two triggers:
- `workflow_dispatch` with an optional `version` input (empty = re-index what
  is already published, no rebuild).
- `release: types: [published]` on a real `vX.Y.Z` release — reads the
  version from `github.event.release.tag_name`, stripped of `v`. Guarded
  against the workflow's own `arch-repo` release publish re-triggering itself
  (`tag_name != 'arch-repo'`) and against prereleases.

On a version run, it polls (up to 30 x 20s ≈ 10 min) that release for
`cliamp-deck-linux-x86_64`, `cliamp-deck-linux-aarch64` and `SHA256SUMS`, reads
the two checksums out of `SHA256SUMS`, writes them into `PKGBUILD`
(`sha256sums_x86_64`/`sha256sums_aarch64`), bumps `pkgver`/resets `pkgrel`, then
builds both arches by overriding `CARCH` in `/etc/makepkg.conf` before each
`makepkg` run — no cross toolchain needed since `package()` only `install`s an
already-built binary.

**Immutability guard**: a published package filename
(`cliamp-deck-<ver>-<rel>-<arch>.pkg.tar.zst`) is never rebuilt if the
`arch-repo` release already serves that exact name — dgmo hit real breakage
from two builds of "the same" recipe producing different bytes under one
filename (their `package()` resolves npm deps at build time; this recipe
doesn't have that problem since it just copies a pinned binary, but the guard
is kept anyway — it is also what makes a re-dispatch at an unchanged version a
no-op instead of pointless work). **A recipe change needs a `pkgrel` bump.**

## Secrets

- `ARCH_SIGNING_KEY` — **not yet created, this port did not generate one.**
  An ASCII-armored (or raw) GPG **private** key, `gpg --export-secret-keys`.
  Absent, the workflow publishes an UNSIGNED repository with a loud
  `::warning::` and `GITHUB_STEP_SUMMARY` note — it still works, but Omarchy's
  stock `SigLevel = Required DatabaseOptional` refuses an unsigned package by
  default, so the user has to write `SigLevel = Never` into their
  `pacman.conf` stanza, which is exactly the smell that makes a third-party
  repo look untrustworthy. **Set this before telling anyone to join the
  channel.**
- `ARCH_SIGNING_KEY_PASSPHRASE` — optional, only if the key above has one.

Generating the key, adding it as a secret, and any release/dispatch are all
things this port deliberately did NOT do (see the task constraints) — that is
on you.

## Landmines carried forward from dgmo (still apply here)

- **Never `sudo pacman -Syu <pkg>` on Omarchy** — its
  `00-omarchy-update-guard.hook` aborts any pacman run carrying both `-S` and
  `-u`. Use `sudo pacman -Sy cliamp-deck` there;
  `cliamp-deck-keyring.install`'s `post_install` prints the right one by
  checking for `/usr/bin/omarchy-update-pacman-guard`.
- **The `y` is not optional in either form** — adding the repository stanza
  does not fetch its database; nothing in the package can do that sync
  (scriptlets and the libalpm hook run inside a transaction already holding
  the pacman lock).
- **`omarchy-refresh-pacman` overwrites the WHOLE of `/etc/pacman.conf`** by
  plain `cp`, then runs `pacman -Syyuu`. The `[cliamp-deck]` stanza does not
  survive that on its own — `10-cliamp-deck-repo.hook`
  (`/usr/share/libalpm/hooks/`, `Target = *`, since there is no package to key
  a `cp` off) puts it back via `ensure-repo.sh` on the next transaction of any
  kind. Omarchy's own hook mechanism
  (`~/.config/omarchy/hooks/pre-refresh-pacman.d/`) is closed to a package: it
  reads only `$HOME` and runs unprivileged.
- **`pacman -U <url>` cannot work for the keyring package** — Omarchy's
  `SigLevel = Required` applies to a remote package, and this one is
  unverifiable by construction (the key that would verify it is the one it
  delivers). `LocalFileSigLevel = Optional` covers a downloaded file, hence
  `curl` first, `pacman -U ./…` second, always two commands chained with
  `&&` on one line (splitting them across terminal lines makes the second
  `sudo` prompt eat the third line as its password).
- **`pacman-key --recv-keys` will never work for this key** — the public half
  ships as a release asset (`cliamp-deck.gpg`), not to a keyserver; Arch's
  `gpg.conf` has no `keyserver` line.
- **`repo-add` leaves `cliamp-deck.db` a symlink to `.tar.gz`** — a GitHub
  release asset can't be a symlink, so both names are copied as real files
  with identical bytes, same for `.files` and each `.sig`.
- **The carry-forward (`gh release download arch-repo --pattern
  '*.pkg.tar.zst' --skip-existing`) can bring back a since-superseded
  version** — the drop loop after `repo-add -p` keeps only what
  `cliamp-deck.db.tar.gz` actually lists, or the release would grow by one
  stale package per publish forever.
- **`gh release upload --clobber` replaces an asset, never removes one** — the
  "remove stale asset" loop in the `Publish it` step is what stops a signed
  run's old `.sig`/`cliamp-deck.gpg` surviving into a later unsigned run
  (which would otherwise be checked and fail under `SigLevel = Required`).

## Where this port deliberately differs from dgmo

- **`cliamp-deck-trusted` (the fingerprint pacman-key trusts) and
  `cliamp-deck.gpg` (the public key) are BOTH generated in CI, never
  committed.** dgmo commits `diagrammo-trusted` by hand and only generates
  the `.gpg`; that split needs a person to update the fingerprint file on
  every key rotation, and dgmo's own workflow carries a checked-in-file vs
  imported-key mismatch guard for exactly that failure mode. Here, the
  `cliamp-deck-keyring.install` placeholder `@CLIAMP_DECK_KEY_FINGERPRINT@`
  and the whole of `cliamp-deck-trusted` are written from the SAME `$FPR`
  shell variable in the "Build the bootstrap package" step, so they can never
  disagree — the mismatch guard has nothing left to guard and was dropped
  rather than ported.
- **Builds both `x86_64` and `aarch64`** in one `ubuntu-latest` (x86_64)
  runner via a `CARCH` override, since dgmo only ever built `x86_64` (its
  `package()` step needs an actual node/npm toolchain per target, this one
  does not).
- **The `arch-repo` release is created with `--prerelease` on first run.**
  dgmo's equivalent release is a plain non-prerelease release that merely
  hasn't become "Latest" because newer real releases keep publishing after
  it — that is incidental, not enforced, and this repo's own `install.sh`
  resolves `releases/latest/download`, so getting this wrong would silently
  break it. Verify after the first real run:
  `gh api repos/demian0311/cliamp-deck/releases/latest --jq .tag_name` must
  NOT print `arch-repo`.
- **Fires on `release: published`, not just `workflow_dispatch`** — releases
  here are already cut by hand with both binaries and `SHA256SUMS` attached
  (there is no separate "release the CLI first, then dispatch this" step like
  dgmo's `scripts/release.sh`), so the natural trigger is the release itself.
  Guarded against retriggering on the workflow's own `arch-repo` publish and
  against prereleases.

## Cutting a release (once ARCH_SIGNING_KEY exists)

Tag and publish `vX.Y.Z` on GitHub with `cliamp-deck-linux-x86_64`,
`cliamp-deck-linux-aarch64` and `SHA256SUMS` attached (however that release is
made today) — the workflow picks it up automatically. To force a run instead
of waiting for the trigger, or to re-index without a new version:

```bash
gh workflow run arch-repo.yml -R demian0311/cliamp-deck -f version=X.Y.Z
gh workflow run arch-repo.yml -R demian0311/cliamp-deck   # re-index, no rebuild
```

## Joining the channel (once signed)

```bash
# Arch
curl -LO https://github.com/demian0311/cliamp-deck/releases/download/arch-repo/cliamp-deck-keyring.pkg.tar.zst &&
  sudo pacman -U ./cliamp-deck-keyring.pkg.tar.zst &&
  sudo pacman -Syu cliamp-deck

# Omarchy — -Syu is refused there
curl -LO https://github.com/demian0311/cliamp-deck/releases/download/arch-repo/cliamp-deck-keyring.pkg.tar.zst &&
  sudo pacman -U ./cliamp-deck-keyring.pkg.tar.zst &&
  sudo pacman -Sy cliamp-deck
```

By hand, no repository:

```bash
git clone https://github.com/demian0311/cliamp-deck.git
cd cliamp-deck/packaging/arch
makepkg -si
```

## Verifying the channel is actually serving what you think

Don't trust the workflow's green check — read the published bytes:

```bash
gh release view arch-repo --repo demian0311/cliamp-deck --json assets --jq '.assets[].name'
curl -LO https://github.com/demian0311/cliamp-deck/releases/download/arch-repo/cliamp-deck.db
bsdtar -xOf cliamp-deck.db '*/desc' | grep -A1 FILENAME
gpg --verify cliamp-deck.db.sig cliamp-deck.db   # only if signed
```
