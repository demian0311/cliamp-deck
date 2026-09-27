# Layout spec (settled and built 2026-09-26)

Decided in a round-by-round interview against the browser mockup (private artifact "Deck Layout Lab",
https://claude.ai/artifact/XaZGcb9htX6kWANjRVM4TT). Replaces the current tiered grid in `view.go`.
Size references are this laptop (1440×900, foot size 8): fullscreen ≈200×58, half tile ≈100×58,
quarter ≈100×28, small split 60×24; LG ultrawide ≈280×58.

## Shape
- Winamp stack, full width: player (top) → [EQ, when open] → visualizer → sources (bottom) → status line.
- Side-by-side when `cols >= 160` and `cols >= 2.5 * rows` (ultrawide 280×58, laptop fullscreen 200×58 → side;
  100×58, 100×28 → stacked; was `cols >= 4.5 * rows` until 2026-09-26, which kept 200×58 stacked): left column
  (min(100, 40% of cols)) = player, EQ, sources; visualizer takes the right, full height.
- Below ~60×24 (current XS): title line, time line, visualizer. Sources appear only when focused, full screen.

## Heights (stacked)
- Player 6 rows (5 when rows < 30). EQ panel 10 rows (7 when rows < 30), only while open; it takes rows from
  the visualizer.
- Remaining rows split 60/40 visualizer/sources. While sources has focus, visualizer drops to 25% (min 3 rows)
  and reverts when focus leaves. Sources minimum 8 rows (5 when rows < 30).

## Visualizer
- One cycle, `fx.Stock()` in order: plasma → tunnel → fire → metaballs → ridges → spectrum → timescope → vortex →
  water → synaesthesia → scope → fountain. Saved names no longer in the cycle fall back to the first.
  2026-09-26: the braille spectrum was removed, then aurora, ripples, rain and warp were replaced by Winamp-era
  effects — vortex (Geiss/MilkDrop feedback), water (AVS Water Bump), fountain (AVS Dot Fountain), spectrum
  (Winamp 2 bars with peak caps; a saved `visualizer = "spectrum"` now lands on it), timescope (AVS spectrogram)
  and synaesthesia (the XMMS plugin) — because they did not visibly follow the music. Candidates were picked from a
  browser mockup replaying the recorded captures.
  `TestEffectsFollowTheMusic` replays the recorded radio and fails any effect whose picture stops tracking the beat
  or loudness (ridges, scope, tunnel and timescope exempt: they answer through shape or history).
  Ridges and spectrum are `fx.GlyphEffect`s: the deck calls `RenderCells` and draws coloured braille dots instead of
  blitting pixels.
- Cycle: `v` anywhere; `←/→` when the visualizer has focus or has taken over; clickable `‹ n/N ›` on its frame.
- Sync: `{` / `}` from anywhere move the picture 10 ms earlier / later against the sound (0–1000 ms, default 130,
  saved as `sync_ms`). cliamp analyses audio as it enters its 250 ms speaker buffer, not as it is heard, so without
  the hold the bands arrive ~130 ms before the sound (issue #3; derived from cliamp v2.0.1 source 2026-09-26, not
  measured end to end). The deck queues each `spectrum.get` result and draws the newest one at least `sync_ms` old;
  the meter follows the same delayed frames. Polling faster buys nothing: cliamp re-runs its FFT at most every 33 ms
  (`TickAnalyze`). Bluetooth output adds its own delay, which is what the nudge is for.
- Takeover: click the visualizer, Enter when it has focus, or `V` anywhere. It fills the terminal; click / Enter / Esc / `V`
  returns. A one-line bar (state, track, station, `‹ name ›`) shows for 3 s after any key or mouse move, then fades.

## Focus, keys, mouse
- Layout changes (focus, EQ, takeover) ease over `layoutDuration` (ease-out) instead of snapping: panel edges
  interpolate from what was on screen; a panel appearing grows from its new top. Shape changes and resizes snap.
- Tab / Shift-Tab cycles focus: player → visualizer → sources. The focused frame is highlighted and its title drawn
  on a tint 30% toward the theme accent; the sources panel's active tab takes the same tint only while it has focus. The open EQ belongs to player focus:
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
  transport + volume. A stream drops `◄◄ ►►` (nothing to skip through). Volume is a braille bar (-30..+6 dB, no
  number) directly under the meter, same x and width. Meter, volume and EQ bands colour along `rampStops` (theme
  green → cyan → blue → magenta → red, 24-bit via `cRGB` cells). Playing marker is `»` (list rows and title).
- Meter (title and source rows, right): two braille rows, a bar per dot row — highs on top, lows at the bottom —
  from cliamp's bands split into `meterRanges` ranges, each in dB below its own decaying peak over `meterRangeDB`.
  Each bar leaves a white peak dot that holds `peakHold` frames then slides back `peakFall` per frame.
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
- Also the last thing playing (`last_track`, cliamp's TrackInfo as JSON, stream title and queue position dropped),
  saved whenever the playing path changes. On attach, if cliamp is neither playing nor paused, the deck sends
  `track.play` with it, so the last station starts again (added 2026-09-26).
- Also the visual sync delay (`sync_ms`), saved on every `{` / `}`.
- Not remembered: queue/history tab, takeover, EQ panel open.

## Deferred / upstream candidates
- cliamp daemon saving EQ like its TUI does (then drop deck-side EQ persistence).
- `spectrum.get` unsmoothed and/or more bands; stereo levels.
- Clickable transport and seek.
