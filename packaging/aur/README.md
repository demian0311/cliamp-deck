AUR packaging for `cliamp-deck` (source build). Not yet submitted to the AUR (2026-09-26).

Release bump: tag `vX.Y.Z` and push it, set `pkgver`, reset `pkgrel=1`, then run
`updpkgsums && makepkg --printsrcinfo > .SRCINFO`, and test with `makepkg -f` using Arch's Go
(`GOTOOLCHAIN=local`). Copy PKGBUILD + .SRCINFO into the AUR git repo
(`ssh://aur@aur.archlinux.org/cliamp-deck.git`) and push.

Keep go.mod's `go` directive at or below Arch's `go` package version (`pacman -Si go`), or the build fetches
a toolchain or fails.
