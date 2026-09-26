# cliamp-deck

Alternative terminal UI for [cliamp](https://github.com/bjarneo/cliamp). Draws a
btop-style grid / Winamp-style player that re-flows across five size tiers; all
audio, providers and the spectrum come from a running cliamp over its V2 IPC socket.

Needs [cliamp](https://github.com/bjarneo/cliamp) installed (Omarchy ships it) and a truecolor terminal.

```sh
go install github.com/demian0311/cliamp-deck@latest
cliamp-deck                                   # starts `cliamp --daemon` if nothing is running
cliamp-deck --spawn=false --socket PATH       # attach to a specific instance only
```

Keys: space play/pause · n/p skip · ←/→ seek · +/- volume · j/k move · enter open/load · esc back ·
v cycle stage (spectrum, plasma, tunnel, fire, metaballs) · V fullscreen stage · q quit.

Stage effects live in `fx/` (interface: pixel frame + bass/mid/treble/beat + theme). Each cell is a `▀`
with 24-bit fg/bg, so the terminal must support truecolor. Colours come from the Omarchy theme at
`~/.local/state/omarchy/current/theme/colors.toml` (override: `--theme PATH`), re-read within 2 s of
`omarchy theme set`.

Known limits (2026-09-26):
- `spectrum.get` serves 10 bands; the deck interpolates to braille resolution.
- A cliamp *TUI* instance only refreshes its bands while its own visualizer renders, so attached to one
  the spectrum can freeze. Run cliamp as `--daemon` for a live spectrum.
- Providers only appear once configured in cliamp's config (e.g. Spotify needs a `[spotify]` section).
- Titles with wide (CJK/emoji) glyphs will misalign the grid.
