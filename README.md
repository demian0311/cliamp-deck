# cliamp-deck

Alternative terminal UI for [cliamp](https://github.com/bjarneo/cliamp). Draws a
btop-style grid / Winamp-style player that re-flows across five size tiers; all
audio, providers and the spectrum come from a running cliamp over its V2 IPC socket.

```sh
go build -o cliamp-deck . && ./cliamp-deck          # starts `cliamp --daemon` if nothing is running
./cliamp-deck --spawn=false --socket PATH           # attach to a specific instance only
```

Keys: space play/pause · n/p skip · ←/→ seek · +/- volume · j/k move · enter open/load · esc back · q quit.

Known limits (2026-09-26):
- `spectrum.get` serves 10 bands; the deck interpolates to braille resolution.
- A cliamp *TUI* instance only refreshes its bands while its own visualizer renders, so attached to one
  the spectrum can freeze. Run cliamp as `--daemon` for a live spectrum.
- Providers only appear once configured in cliamp's config (e.g. Spotify needs a `[spotify]` section).
- Titles with wide (CJK/emoji) glyphs will misalign the grid.
