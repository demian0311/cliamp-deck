AUR packaging for `cliamp-deck` (source build). Not yet submitted to the AUR (checked 2026-09-27: RPC returns 0 results; this machine has no AUR SSH key). Ships a `.desktop` entry as a local source, so the launcher needs no `omarchy-tui-install`.

Release bump: tag `vX.Y.Z` and push it, set `pkgver`, reset `pkgrel=1`, then run
`updpkgsums && makepkg --printsrcinfo > .SRCINFO`, and test with `makepkg -f` using Arch's Go
(`GOTOOLCHAIN=local`). Copy PKGBUILD + .SRCINFO into the AUR git repo
(`ssh://aur@aur.archlinux.org/cliamp-deck.git`) and push.

Keep go.mod's `go` directive at or below Arch's `go` package version (`pacman -Si go`), or the build fetches
a toolchain or fails.
