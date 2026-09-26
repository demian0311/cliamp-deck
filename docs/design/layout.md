# Layout spec (settled 2026-09-26, not yet built)

Decided in a round-by-round interview against the browser mockup (private artifact "Deck Layout Lab",
https://claude.ai/artifact/XaZGcb9htX6kWANjRVM4TT). Replaces the current tiered grid in `view.go`.
Size references are this laptop (1440×900, foot size 8): fullscreen ≈200×58, half tile ≈100×58,
quarter ≈100×28, small split 60×24; LG ultrawide ≈280×58.

## Shape
- Winamp stack, full width: player (top) → [EQ, when open] → visualizer → sources (bottom) → status line.
- Side-by-side when `cols >= 4.5 * rows` (ultrawide 280×58 → side; 200×58, 100×28 → stacked): left column
  (min(100, 40% of cols)) = player, EQ, sources; visualizer takes the right, full height.
- Below ~60×24 (current XS): title line, time line, visualizer. Sources appear only when focused, full screen.

## Heights (stacked)
- Player 6 rows (5 when rows < 30). EQ panel 10 rows (7 when rows < 30), only while open; it takes rows from
  the visualizer.
- Remaining rows split 60/40 visualizer/sources. While sources has focus, visualizer drops to 25% (min 3 rows)
  and reverts when focus leaves. Sources minimum 8 rows (5 when rows < 30).

## Visualizer
- One cycle: spectrum → plasma → tunnel → fire → metaballs (`fx.Stock()` after spectrum).
- Cycle: `v` anywhere; `←/→` when the visualizer has focus or has taken over; clickable `‹ n/N ›` on its frame.
- Takeover: click the visualizer, Enter when it has focus, or `V` anywhere. It fills the terminal; click / Enter / Esc / `V`
  returns. A one-line bar (state, track, station, `‹ name ›`) shows for 3 s after any key or mouse move, then fades.

## Focus, keys, mouse
- Tab / Shift-Tab cycles focus: player → visualizer → sources. The focused frame is highlighted.
- Sources: tabs `sources | queue | history` (`[` `]` or click a tab). `↑↓`/wheel move; Enter plays now (replaces
  queue; `track.play`); `a` append to the end of the live playlist (`queue`, by path); `A` play next
  (`track.queue` for a supplied track, `queue.enqueue` for one already in the live playlist). Checked against
  `cliamp remote capabilities` on v2.0.1, 2026-09-26.
- `/` searches the current source; at the top level it searches all configured sources, results grouped by source.
- Mouse now: visualizer (takeover, `‹ ›`), source rows (click select, double-click play, wheel scroll), tabs.
  Later: transport buttons and a clickable seek bar. Bubble Tea v2: set `View.MouseMode` (cell motion).
- `e` opens the EQ under the player: `←→` band, `↑↓` gain ±1 dB (±12), `p` next preset (cliamp's 16 built-ins),
  `e`/Esc close. Applied live through the `eq` operation (name | band + value).

## Player
- Rows: title (marquee) · source/station · time line (track: thin bar; stream: `● LIVE on air mm:ss`) ·
  transport + volume.
- One braille level meter (mono), colour ramp green → amber → red, top right. cliamp exposes no stereo levels
  over IPC; L/R needs an upstream PR first.

## Sources you haven't set up
- Dimmed group at the bottom of the source list: "Spotify · s to connect", etc.
- `s` warns that cliamp's player will restart (playback stops), suspends the deck, runs `cliamp setup` in the
  same terminal, resumes, restarts the cliamp daemon, and reloads `provider.list`.
- Needed: the list of providers cliamp *can* configure. `provider.list` only returns configured ones; source it
  from `cliamp setup`'s list (Navidrome, Plex, Jellyfin, Spotify, Qobuz, Tidal, NetEase, Audiobookshelf,
  YouTube Music as of v2.0.1) and re-verify on bump.

## Persistence (deck-owned; cliamp's daemon does not save EQ)
- `~/.config/cliamp-deck/state.toml`: EQ (preset name + 10 bands) and last visualizer. On attach, reapply the EQ.
- Not remembered: source tab or selection, takeover, EQ panel open.

## Deferred / upstream candidates
- cliamp daemon saving EQ like its TUI does (then drop deck-side EQ persistence).
- `spectrum.get` unsmoothed and/or more bands; stereo levels.
- Clickable transport and seek.
