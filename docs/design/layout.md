# Layout spec (settled and built 2026-09-26)

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
- Layout changes (focus, EQ, takeover) ease over `layoutDuration` (ease-out) instead of snapping: panel edges
  interpolate from what was on screen; a panel appearing grows from its new top. Shape changes and resizes snap.
- Tab / Shift-Tab cycles focus: player → visualizer → sources. The focused frame is highlighted and its title drawn
  negative; the sources panel's active tab is negative only while it has focus. The open EQ belongs to player focus:
  it takes the keys (and the highlight) while the player has focus, and tab leaves it open.
- Player: `↑↓` volume ±2 dB, `←→` seek ±5 s (also `,` `.` from anywhere).
- Sources: tabs `sources | queue | history` (`[` `]` or click a tab). `↑↓`/wheel move (skipping headers); `→`/`l` opens a source or country, `←`/`h` goes back a level (esc too) — Radio lists pinned entries then countries (most stations first, ties alphabetical), a country lists its stations with bitrate in the dim right column; Enter plays now (replaces
  queue; `track.play`); `a` append to the end of the live playlist (`queue`, by path); `A` play next
  (`track.queue` for a supplied track, `queue.enqueue` for one already in the live playlist). Checked against
  `cliamp remote capabilities` on v2.0.1, 2026-09-26.
- Enter on a row spins a braille throbber in its marker column (`pendingRow`) until it lands: a source's list
  arrives, or cliamp reports the playlist/track playing with position > 0. A failed op or `pendingTimeout` stops it.
- `/` searches the current source; at the top level it searches all configured sources, results grouped by source.
- Mouse now: visualizer (takeover, `‹ ›`), source rows (click select, double-click play, wheel scroll), tabs.
  Later: transport buttons and a clickable seek bar. Bubble Tea v2: set `View.MouseMode` (cell motion).
- `e` toggles the EQ under the player (and focuses the player) — the only way in or out: `←→` band (wraps),
  `↑↓` gain ±1 dB (±12), `p` next preset (cliamp's 16 built-ins), `0` Flat (EQ off), `e`/Esc close. Applied live through the `eq` operation (name | band + value).

## Player
- Rows: title (marquee) · source/station · time line (track: thin bar; stream: `● LIVE on air mm:ss`) ·
  transport + volume. A stream drops `◄◄ ►►` (nothing to skip through). Volume is a half-row solid bar (-30..+6 dB, no
  number) directly under the meter, same x and width. Meter, volume and EQ bands colour along `rampStops` (theme
  green → cyan → blue → magenta → red, 24-bit via `cRGB` cells). Playing marker is `»` (list rows and title).
- Meter (title and source rows, right): two text rows of solid ▀ half-block bars, one per half row — highs on top,
  lows at the bottom — from cliamp's bands split into `meterRanges` ranges, each in dB below its own decaying peak
  over `meterRangeDB`, each range its own colour along `rampStops` with a faint track when unlit. A bright peak
  cell holds `peakHold` frames then slides back `peakFall` per frame. (Braille was dropped: its dots leave gaps.)
- Stream time line: the `●` before LIVE blinks while playing.
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
- `~/.config/cliamp-deck/state.toml`: EQ (preset name + 10 bands), last visualizer, and where the sources list was
  left (open provider, radio country, selected row). On attach, reapply the EQ and reopen that provider/country.
  Saved on opening a provider or country, going back, and quitting; not overwritten before the restore lands.
- Not remembered: queue/history tab, takeover, EQ panel open.

## Deferred / upstream candidates
- cliamp daemon saving EQ like its TUI does (then drop deck-side EQ persistence).
- `spectrum.get` unsmoothed and/or more bands; stereo levels.
- Clickable transport and seek.
