package main

import (
	"cmp"
	"fmt"
	"math"
	"os"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/ipc"

	"github.com/demian0311/cliamp-deck/fx"
)

const (
	specInterval  = 33 * time.Millisecond // spectrum.get rate, ~30 fps
	stateInterval = 500 * time.Millisecond
	barLinger     = 3 * time.Second // takeover bar stays this long after a key or mouse move
	doubleClick   = 400 * time.Millisecond
)

// eqPresets are cliamp's built-in EQ presets in its own order (ui/model/eq_presets.go, v2.0.1).
// The daemon applies them by name; it does not list them over IPC.
var eqPresets = []string{"Flat", "Rock", "Pop", "Jazz", "Classical", "Bass Boost", "Treble Boost", "Vocal",
	"Electronic", "Acoustic", "Hip-Hop", "R&B", "Loudness", "Late Night", "Podcast", "Small Speakers"}

type (
	specMsg      struct{ bands []float64 }
	stateMsg     struct{ snap *ipc.RuntimeSnapshot }
	connMsg      struct{ err error }
	providersMsg struct {
		list []ipc.ProviderInfo
		err  error
	}
	playlistsMsg struct {
		provider, name string
		list           []ipc.PlaylistInfo
		err            error
		catalog        bool // a paged catalog; added is how many the last page brought
		added          int
		keepSel        bool // a further page of the list already shown
	}
	opMsg struct {
		label string
		err   error
		eq    bool // an EQ change: save the resulting EQ once state refreshes
	}
	themeMsg struct {
		th  *fx.Theme
		mod time.Time
	}
)

type model struct {
	c    client
	w, h int

	snap   *ipc.RuntimeSnapshot
	snapAt time.Time
	online bool

	bands []float64 // latest raw bands from cliamp
	// The player's meter: four frequency ranges, lows to highs, each
	// auto-gained on its own so the quiet highs move as visibly as the bass.
	levels, levelGain, levelPeak [meterRanges]float64
	peakWait                     [meterRanges]int
	marq                         float64

	// Visualizer: mode indexes effects. A GlyphEffect draws into cells rather
	// than frame.
	effects  []fx.Effect
	mode     int
	an       *fx.Analyzer
	frame    *fx.Frame
	cells    *fx.Cells
	start    time.Time
	lastSpec time.Time
	specQ    []specFrame // spectrum frames waiting out the sync delay

	themePath string
	theme     *fx.Theme
	themeMod  time.Time

	focus   focusArea
	take    bool
	lastAct time.Time
	eqOpen  bool
	eqBand  int

	providers    []ipc.ProviderInfo
	provider     string // open provider key in the sources tab; "" = top level
	providerName string
	playlist     string // the playlist open inside provider; "" = its list of playlists
	playlistName string
	nowPlaying   bool               // the now-playing level: cliamp's live playlist, no provider open
	stations     []ipc.PlaylistInfo // the open provider's list, as loaded so far
	stationsOf   string             // the provider stations belongs to
	reselect     string             // the playlist to select once the provider's list reloads
	rowsKey      string             // the fetch an open playlist's rows come from (rowsWant)
	follow       bool               // an open playlist's selection follows the playing track until moved
	inCountry    bool               // a radio country is open; country is which
	country      string
	tab          int
	lists        [tabCount]listState
	searching    bool   // typing a query
	query        string // the query being typed, or the one whose results are shown
	inResults    bool   // the sources tab shows search results
	loading      bool
	lastClick    time.Time
	lastClickRow int
	catalog      bool // the open provider is a paged catalog with more to load

	statePath string
	saved     deckState
	eqApplied bool // the saved EQ has been reapplied since attaching
	// shuffleSynced: cliamp's shuffle has matched the saved one since
	// attaching, so a change from here on is the listener's and is saved.
	shuffleSynced bool
	attachedAt    time.Time
	eqDirty       bool // an EQ change is waiting for fresh state to be saved

	// Reopening the sources list where it was left: restorePending until the
	// providers arrive, restoring while that provider's list loads.
	restorePending, restoring bool

	pending *pendingRow // the row whose Enter is still loading, if any

	anim layoutAnim // the panels easing from one layout to the next

	note   string
	noteAt time.Time
}

func newModel(c client, themePath, statePath string) model {
	th, _ := fx.LoadTheme(themePath)
	m := model{c: c, effects: fx.Stock(), an: &fx.Analyzer{}, frame: &fx.Frame{}, cells: &fx.Cells{},
		start: time.Now(), themePath: themePath, theme: th, themeMod: modTime(themePath),
		focus: focusVis, statePath: statePath, lastClickRow: -1}
	m.saved = loadState(statePath)
	m.restorePending = m.saved.Source != ""
	restored := false
	for i, name := range m.modeNames() {
		if name == m.saved.Visualizer {
			m.mode, restored = i, true
		}
		if name == "spectrum" && !restored { // the default, unless a saved name that still exists overrides it
			m.mode = i
		}
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchState(), m.fetchSpec(), m.fetchProviders(), m.watchTheme())
}

func modTime(path string) time.Time {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}

// watchTheme re-reads colors.toml when `omarchy theme set` replaces it.
func (m model) watchTheme() tea.Cmd {
	path, seen := m.themePath, m.themeMod
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		mod := modTime(path)
		if mod.Equal(seen) {
			return themeMsg{mod: seen}
		}
		th, _ := fx.LoadTheme(path)
		return themeMsg{th, mod}
	})
}

func (m model) fetchSpec() tea.Cmd {
	return tea.Tick(specInterval, func(time.Time) tea.Msg {
		b, err := m.c.spectrum()
		if err != nil {
			return specMsg{}
		}
		return specMsg{b}
	})
}

func (m model) fetchState() tea.Cmd {
	return func() tea.Msg {
		s, err := m.c.state()
		if err != nil {
			return connMsg{err}
		}
		return stateMsg{s}
	}
}

func (m model) fetchProviders() tea.Cmd {
	return func() tea.Msg {
		l, err := m.c.providers()
		return providersMsg{l, err}
	}
}

func (m model) run(label, op string, params any) tea.Cmd {
	return func() tea.Msg {
		_, err := m.c.op(op, params)
		return opMsg{label: label, err: err}
	}
}

func (m *model) say(s string) { m.note, m.noteAt = s, time.Now() }

func (m model) ls() layoutState {
	return layoutState{take: m.take, eqOpen: m.eqOpen, focus: m.focus, compact: m.spectrumShown()}
}

// spectrumShown is true while spectrum is the visualizer: it is drawn small,
// and the player swaps its own level meter for the volume.
func (m model) spectrumShown() bool { return len(m.effects) > 0 && m.modeName() == "spectrum" }

func (m model) modeNames() []string {
	var names []string
	for _, e := range m.effects {
		names = append(names, e.Name())
	}
	return names
}

func (m model) modeName() string { return m.modeNames()[m.mode] }

func (m *model) cycle(d int) {
	n := len(m.effects)
	m.mode = (m.mode + d + n) % n
	m.saved.Visualizer = m.modeName()
	m.persist()
}

// rememberSource records where the sources list is, so the next run reopens
// it there.
func (m *model) rememberSource() {
	if m.restorePending || m.restoring {
		return // the remembered place hasn't been reached yet; keep it
	}
	m.saved.Source, m.saved.SourceInCountry, m.saved.SourceCountry = m.provider, m.inCountry, m.country
	m.saved.SourcePlaylist, m.saved.SourcePlaylistName = m.playlist, m.playlistName
	m.saved.SourceSelected = m.playlist
	if l := m.lists[tabSources]; l.sel < len(l.rows) && !m.inResults && !m.inPlaylist() {
		m.saved.SourceSelected = l.rows[l.sel].key
	}
	m.persist()
}

// inPlaylist is true while the sources tab is inside a playlist or the
// now-playing level, listing tracks.
func (m *model) inPlaylist() bool { return m.playlist != "" || m.nowPlaying }

// liveOpen is true when the open playlist's rows are cliamp's live playlist:
// the now-playing level, or the provider playlist cliamp has loaded.
func (m *model) liveOpen() bool {
	return m.snap != nil && (m.nowPlaying || (m.playlist != "" && m.snap.Playlist == m.provider+":"+m.playlist))
}

// openPlaylist goes inside a provider playlist. Its rows arrive with the next
// refreshRows.
func (m *model) openPlaylist(provider, name, playlist, playlistName string) {
	m.provider, m.providerName, m.playlist, m.playlistName = provider, name, playlist, playlistName
	m.nowPlaying = false
	m.enterLevel()
}

// openNowPlaying lists cliamp's live playlist when what it has loaded is not
// a provider playlist (a lone track, search result or station).
func (m *model) openNowPlaying() {
	m.provider, m.providerName, m.playlist, m.playlistName = "", "", "", ""
	m.nowPlaying = true
	m.enterLevel()
}

func (m *model) enterLevel() {
	m.inCountry, m.inResults = false, false
	m.restorePending, m.restoring = false, false // this is where the list now is
	m.lists[tabSources] = listState{}
	m.rowsKey, m.follow, m.loading = "", true, true
	m.rememberSource()
}

// rowsWant names the fetch an open playlist's rows should come from: cliamp's
// live playlist at its current revision once it is the one loaded, else the
// provider's own track list.
func (m *model) rowsWant() string {
	at := m.provider + ":" + m.playlist
	if m.liveOpen() {
		return fmt.Sprintf("%s queue@%d", at, m.snap.PlaylistRevision)
	}
	return at + " tracks"
}

// refreshRows fetches the open playlist's rows unless they already come from
// the fetch rowsWant names. It is how the list follows cliamp: a new playlist
// revision, or the open playlist becoming the loaded one.
func (m *model) refreshRows() tea.Cmd {
	key := m.rowsWant()
	if key == m.rowsKey {
		return nil
	}
	m.rowsKey = key
	c, live := m.c, m.liveOpen()
	params := map[string]any{"provider": m.provider, "playlist": m.playlist}
	return func() tea.Msg {
		if live {
			t, err := allTracks(c, "queue.list", nil)
			return tracksMsg{key: key, tracks: t, live: true, err: err}
		}
		t, err := allTracks(c, "provider.tracks", params)
		return tracksMsg{key: key, tracks: t, err: err}
	}
}

// markCurrent puts the » on the live row cliamp is playing, as tracks
// advance, and the selection with it while it follows.
func (m *model) markCurrent() {
	l := &m.lists[tabSources]
	for i := range l.rows {
		r := &l.rows[i]
		if !r.live {
			continue
		}
		r.current = m.snap != nil && m.snap.Index == r.index
		if r.current && m.follow {
			l.sel = i
		}
	}
}

// providerNamed is a provider's display name, or its key until the list of
// providers arrives.
func (m *model) providerNamed(key string) string {
	for _, p := range m.providers {
		if p.Key == key {
			return p.Name
		}
	}
	return ""
}

func (m *model) persist() {
	if m.statePath == "" {
		return
	}
	m.saved.Visualizer = m.modeName()
	if err := saveState(m.statePath, m.saved); err != nil {
		m.say("couldn't save deck state: " + err.Error())
	}
}

// Update handles msg, then starts a layout animation if it moved the panels.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	now := time.Now()
	shown := m.layoutAt(now) // what is on screen before msg
	next, cmd := m.update(msg)
	nm := next.(model)
	if ls := nm.ls(); ls != m.ls() {
		nm.anim = layoutAnim{from: shown, to: ls, at: now, w: nm.w, h: nm.h}
	}
	return nm, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height

	case specMsg:
		return m.onSpectrum(msg)

	case stateMsg:
		return m.onState(msg)

	case connMsg:
		m.online = false
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return m.fetchState()() })

	case themeMsg:
		if msg.th != nil {
			m.theme, m.themeMod = msg.th, msg.mod
			m.say("theme: " + msg.th.Name)
		}
		return m, m.watchTheme()

	case providersMsg:
		if msg.err != nil {
			m.say("sources: " + msg.err.Error())
			return m, nil
		}
		m.providers = msg.list
		if m.provider != "" && m.providerName == "" { // opened by resume before the names came
			m.providerName = m.providerNamed(m.provider)
		}
		if m.provider == "" && !m.inResults && !m.nowPlaying {
			m.lists[tabSources].rows = m.topRows()
			if m.restorePending {
				m.restorePending = false
				for _, p := range m.providers {
					if p.Key == m.saved.Source {
						m.loading, m.restoring = true, true
						return m, m.openProvider(p.Key, p.Name, false)
					}
				}
			}
		}

	case playlistsMsg:
		if m.inPlaylist() { // a restore overtaken by resume, or a late page
			return m, nil
		}
		m.loading = false
		if m.pending != nil && m.pending.key == msg.provider {
			m.pending = nil
		}
		if msg.err != nil {
			m.restoring = false
			m.say(msg.name + ": " + msg.err.Error())
			return m, nil
		}
		// A further page regroups the list, so the selection follows its row
		// rather than its index.
		selKey := ""
		if l := m.lists[tabSources]; msg.keepSel && l.sel < len(l.rows) {
			selKey = l.rows[l.sel].key
		}
		m.provider, m.providerName, m.inResults = msg.provider, msg.name, false
		m.catalog = msg.catalog && msg.added > 0
		m.stations, m.stationsOf = msg.list, msg.provider
		if !msg.keepSel {
			m.inCountry = false
		}
		if m.reselect != "" {
			selKey, m.reselect = m.reselect, ""
		}
		playlist := ""
		if m.restoring {
			m.restoring = false
			m.inCountry, m.country = m.saved.SourceInCountry, m.saved.SourceCountry
			selKey, playlist = m.saved.SourceSelected, m.saved.SourcePlaylist
		}
		rows := m.playlistRows(msg.provider, msg.list)
		if m.inCountry && len(rows) == 0 { // the remembered country is gone
			m.inCountry = false
			rows = m.playlistRows(msg.provider, msg.list)
		}
		sel := firstSelectable(rows)
		for i, r := range rows {
			if selKey != "" && r.key == selKey {
				sel = i
			}
		}
		m.lists[tabSources] = listState{rows: rows, sel: sel}
		m.lists[tabSources].move(0)
		if playlist != "" && !isStation(playlist) { // reopen inside the playlist left open
			m.openPlaylist(msg.provider, msg.name, playlist, m.saved.SourcePlaylistName)
			return m, m.refreshRows()
		}
		m.rememberSource()

	case tracksMsg:
		if msg.key != m.rowsKey || !m.inPlaylist() || m.inResults {
			return m, nil // superseded, or the playlist was left
		}
		m.loading = false
		if msg.err != nil {
			m.say("tracks: " + msg.err.Error())
			return m, nil
		}
		// The selection stays on its track as the rows change under it, e.g.
		// from the playlist's own tracks to cliamp's once it loads.
		path := ""
		if l := m.lists[tabSources]; l.sel < len(l.rows) && l.rows[l.sel].track != nil {
			path = l.rows[l.sel].track.Path
		}
		rows := playlistTracks(msg.tracks, msg.live)
		sel := 0
		for i, r := range rows {
			if r.track.Path == path {
				sel = i
				break
			}
		}
		m.lists[tabSources] = listState{rows: rows, sel: sel}
		m.markCurrent()

	case historyMsg:
		if msg.err != nil {
			m.say("history: " + msg.err.Error())
			return m, nil
		}
		m.lists[tabHistory] = listState{rows: historyRows(msg.items)}

	case searchMsg:
		m.loading = false
		if msg.query != m.query {
			return m, nil // superseded by a newer search
		}
		m.inResults = true
		m.lists[tabSources] = listState{rows: searchRows(msg.groups)}
		m.lists[tabSources].sel = firstSelectable(m.lists[tabSources].rows)

	case opMsg:
		if msg.err != nil {
			m.pending = nil
			m.say(msg.label + ": " + msg.err.Error())
		} else if msg.label != "" {
			m.say(msg.label)
		}
		m.eqDirty = m.eqDirty || (msg.eq && msg.err == nil)
		return m, m.fetchState()

	case setupDoneMsg:
		if msg.err != nil {
			m.say("cliamp setup: " + msg.err.Error())
			return m, nil
		}
		if !msg.changed {
			m.say("setup closed without changes")
			return m, nil
		}
		m.say("restarting cliamp to load new sources…")
		return m, restartDaemon(m.c)

	case daemonRestartMsg:
		switch {
		case msg.err != nil:
			m.say("restart cliamp: " + msg.err.Error())
		case !msg.restarted:
			m.say("cliamp is running as a player, not a daemon: restart it to load new sources")
		default:
			m.say("cliamp restarted")
			m.eqApplied = false
		}
		return m, tea.Batch(m.fetchState(), m.fetchProviders())

	case tea.KeyPressMsg:
		m.lastAct = time.Now()
		return m.key(msg.String())

	case tea.MouseClickMsg:
		m.lastAct = time.Now()
		return m.click(msg.Mouse())

	case tea.MouseWheelMsg:
		m.lastAct = time.Now()
		return m.wheel(msg.Mouse())

	case tea.MouseMotionMsg:
		m.lastAct = time.Now()
	}
	return m, nil
}

// specFrame is a spectrum.get result and when it arrived.
type specFrame struct {
	at    time.Time
	bands []float64
}

// delayed queues bands that arrived at now and returns the newest queued frame
// at least SyncMs old, dropping it and everything before it. ok is false
// while nothing has waited long enough.
func (m *model) delayed(now time.Time, bands []float64) (out []float64, ok bool) {
	m.specQ = append(m.specQ, specFrame{now, bands})
	wait := time.Duration(m.saved.SyncMs) * time.Millisecond
	i := -1
	for j, f := range m.specQ {
		if now.Sub(f.at) >= wait {
			i = j
		}
	}
	if i < 0 {
		return nil, false
	}
	out = m.specQ[i].bands
	m.specQ = append(m.specQ[:0], m.specQ[i+1:]...)
	return out, true
}

// nudgeSync moves the picture later (d > 0) or earlier against the sound.
func (m *model) nudgeSync(d int) {
	m.saved.SyncMs = max(0, min(m.saved.SyncMs+d, syncMaxMs))
	m.say(fmt.Sprintf("visual delay %d ms  ({ earlier · } later)", m.saved.SyncMs))
	m.persist()
}

func (m model) onSpectrum(msg specMsg) (tea.Model, tea.Cmd) {
	now := time.Now()
	bands, ok := m.delayed(now, msg.bands)
	if !ok {
		return m, m.fetchSpec()
	}
	m.bands = bands
	m.updateLevels(bands)
	m.marq += 0.2
	dt := 1.0 / 30
	if !m.lastSpec.IsZero() {
		dt = min(0.1, now.Sub(m.lastSpec).Seconds())
	}
	m.lastSpec = now
	a := m.an.Update(bands, dt) // raw: the analyzer normalises per signal
	if m.w > 0 && m.mode < len(m.effects) {
		if in := m.layout().visInner(); !in.empty() {
			t := now.Sub(m.start).Seconds()
			if ge, ok := m.effects[m.mode].(fx.GlyphEffect); ok {
				m.cells.Resize(in.w, in.h)
				ge.RenderCells(m.cells, a, t, dt, m.theme)
			} else {
				m.frame.Resize(in.w, in.h*2)
				m.effects[m.mode].Render(m.frame, a, t, dt, m.theme)
			}
		}
	}
	return m, m.fetchSpec()
}

func (m model) onState(msg stateMsg) (tea.Model, tea.Cmd) {
	m.snap, m.snapAt, m.online = msg.snap, time.Now(), true
	if p := m.pending; p != nil && (p.playing(m.snap) || time.Since(p.since) > pendingTimeout) {
		m.pending = nil
	}
	cmds := []tea.Cmd{tea.Tick(stateInterval, func(time.Time) tea.Msg { return m.fetchState()() })}
	if m.eqDirty {
		m.eqDirty = false
		m.saved.EQPreset, m.saved.EQBands = m.snap.EQPreset, slices.Clone(m.snap.EQBands)
		m.persist()
	}
	if !m.eqApplied {
		m.eqApplied = true
		if c := m.reapplyEQ(); c != nil {
			cmds = append(cmds, c)
		}
		if c := m.resume(); c != nil {
			cmds = append(cmds, c)
		}
		m.attachedAt = time.Now()
		if c := m.reapplyShuffle(); c != nil {
			cmds = append(cmds, c)
		}
	}
	m.rememberShuffle()
	m.rememberTrack()
	m.rememberPosition(false)
	if m.inPlaylist() && !m.inResults {
		if c := m.refreshRows(); c != nil {
			cmds = append(cmds, c)
		}
		m.markCurrent()
	}
	if m.tab == tabSources && m.provider == "" && !m.inResults && !m.nowPlaying {
		// The now-playing row comes and goes; the selection keeps its row.
		l := m.lists[tabSources]
		rows, sel := m.topRows(), l.sel
		if l.sel < len(l.rows) {
			was := l.rows[l.sel]
			for i, r := range rows {
				if r.kind == was.kind && r.key == was.key && r.label == was.label {
					sel = i
				}
			}
		}
		m.lists[tabSources] = listState{rows: rows, sel: sel}
		m.lists[tabSources].move(0)
	}
	return m, tea.Batch(cmds...)
}

// resume starts what was last playing when the deck attaches to a cliamp that
// has nothing on: a freshly spawned daemon, or one left stopped. Anything
// already playing or paused is left alone.
func (m *model) resume() tea.Cmd {
	t := m.saved.LastTrack
	if t == nil || m.snap.State == "playing" || m.snap.State == "paused" {
		return nil
	}
	label := cmp.Or(t.Station, t.Title, t.Path)
	s, c := m.saved, m.c
	if s.LastPlaylist != "" {
		m.say("resuming " + cmp.Or(s.LastPlaylistName, label) + "…")
		// The list lands inside the playlist, on the track as it plays.
		m.openPlaylist(s.LastProvider, m.providerNamed(s.LastProvider), s.LastPlaylist, s.LastPlaylistName)
		return playInPlaylist(c, s.LastProvider, s.LastPlaylist, t.Path, label, s.LastPosition)
	}
	m.say("resuming " + label + "…")
	return func() tea.Msg {
		_, err := c.op("track.play", map[string]*ipc.TrackInfo{"track": t})
		if err == nil {
			seekWhenReady(c, t.Path, s.LastPosition)
		}
		return opMsg{label: "playing " + label, err: err}
	}
}

// playInPlaylist loads a provider playlist and moves to one of its tracks, pos
// seconds in, so next, previous and shuffle carry on through the playlist:
// resume, and Enter on a track of a playlist not yet loaded. A big playlist
// arrives in pages after the load returns, so the track is looked for until
// resumeWait runs out; if it never turns up the playlist just plays from the
// top.
func playInPlaylist(c client, provider, playlist, path, label string, pos float64) tea.Cmd {
	return func() tea.Msg {
		if _, err := c.op("provider.load", map[string]string{"provider": provider, "playlist": playlist}); err != nil {
			return opMsg{label: "load", err: err}
		}
		for deadline := time.Now().Add(resumeWait); ; time.Sleep(resumePoll) {
			if i, ok := queueIndex(c, path); ok {
				_, err := c.op("queue.play", map[string]int{"index": i})
				if err == nil {
					seekWhenReady(c, path, pos)
				}
				return opMsg{label: "playing " + label, err: err}
			}
			if time.Now().After(deadline) {
				return opMsg{label: "playing from the top"}
			}
		}
	}
}

const (
	resumeWait = 10 * time.Second
	resumePoll = 500 * time.Millisecond
)

// seekWhenReady moves a just-started track to pos once cliamp reports it
// playing and seekable (a Spotify track takes a moment to open). Positions
// within the first or last few seconds are not worth a jump.
func seekWhenReady(c client, path string, pos float64) {
	if pos < resumeMinPos {
		return
	}
	for deadline := time.Now().Add(resumeWait); time.Now().Before(deadline); time.Sleep(resumeSeekPoll) {
		s, err := c.state()
		if err != nil {
			return
		}
		if s == nil || s.Track == nil || s.Track.Path != path || !s.Seekable {
			continue
		}
		if s.Duration > 0 && pos > s.Duration-resumeMinPos {
			return
		}
		c.op("seek.absolute", map[string]float64{"value": pos})
		return
	}
}

const (
	resumeMinPos   = 5.0 // seconds
	resumeSeekPoll = 200 * time.Millisecond
	positionEvery  = 10.0 // seconds of playback between saves of LastPosition
)

// rememberPosition saves how far into the track playback is, every
// positionEvery seconds while playing, and always when force is set (quit).
func (m *model) rememberPosition(force bool) {
	if m.snap == nil {
		return
	}
	t := m.snap.Track
	if t == nil || m.saved.LastTrack == nil || t.Path != m.saved.LastTrack.Path {
		return
	}
	pos := m.snap.Position
	if t.Stream || !m.snap.Seekable {
		pos = 0
	}
	if pos == m.saved.LastPosition || (!force && math.Abs(pos-m.saved.LastPosition) < positionEvery) {
		return
	}
	m.saved.LastPosition = pos
	m.persist()
}

// queueIndex finds a track in cliamp's live playlist by path.
func queueIndex(c client, path string) (int, bool) {
	tracks, err := allTracks(c, "queue.list", nil)
	if err != nil {
		return 0, false
	}
	for i, t := range tracks {
		if t.Path == path {
			return i, true
		}
	}
	return 0, false
}

// rememberPlaylist records the provider playlist being started, or clears it
// (empty provider) when a single track or station is.
func (m *model) rememberPlaylist(provider, playlist, name string) {
	s := &m.saved
	if s.LastProvider == provider && s.LastPlaylist == playlist {
		return
	}
	s.LastProvider, s.LastPlaylist, s.LastPlaylistName = provider, playlist, name
	m.persist()
}

// rememberTrack saves what is playing whenever it changes, for resume. A
// stream's now-playing title changes every song, so only the path counts.
func (m *model) rememberTrack() {
	t := m.snap.Track
	if m.snap.State != "playing" || t == nil || t.Path == "" ||
		(m.saved.LastTrack != nil && m.saved.LastTrack.Path == t.Path) {
		return
	}
	keep := *t
	keep.StreamTitle, keep.Index, keep.QueuePosition = "", 0, 0 // stale by next run
	m.saved.LastTrack, m.saved.LastPosition = &keep, 0
	m.persist()
}

// reapplyShuffle restores the saved shuffle mode, which cliamp's daemon
// forgets on restart.
func (m model) reapplyShuffle() tea.Cmd {
	on := m.snap.Shuffle
	if on == nil || *on == m.saved.Shuffle {
		return nil
	}
	mode := map[bool]string{true: "on", false: "off"}[m.saved.Shuffle]
	return m.run("", "shuffle", map[string]string{"name": mode})
}

// rememberShuffle saves shuffle changes made after attaching, from z, a click
// or `cliamp shuffle`. Until cliamp matches the saved mode (reapplyShuffle
// is in flight) its old mode is not mistaken for a change; if it never
// matches, the deck stops waiting after pendingTimeout.
func (m *model) rememberShuffle() {
	if m.snap.Shuffle == nil {
		return
	}
	on := *m.snap.Shuffle
	switch {
	case !m.shuffleSynced:
		m.shuffleSynced = on == m.saved.Shuffle || time.Since(m.attachedAt) > pendingTimeout
	case on != m.saved.Shuffle:
		m.saved.Shuffle = on
		m.persist()
	}
}

// reapplyEQ restores the EQ the deck saved, since cliamp's daemon forgets it.
func (m model) reapplyEQ() tea.Cmd {
	s, snap, c := m.saved, m.snap, m.c
	if s.EQPreset != "" && s.EQPreset != "Custom" {
		if snap.EQPreset == s.EQPreset {
			return nil
		}
		return func() tea.Msg {
			_, err := c.op("eq", map[string]string{"name": s.EQPreset})
			return opMsg{label: "eq " + s.EQPreset, err: err}
		}
	}
	if len(s.EQBands) != 10 || slices.Equal(s.EQBands, snap.EQBands) {
		return nil
	}
	return func() tea.Msg {
		for i, v := range s.EQBands {
			if _, err := c.op("eq", map[string]any{"band": i, "value": v}); err != nil {
				return opMsg{label: "restore eq", err: err}
			}
		}
		return opMsg{label: "eq restored"}
	}
}

// setEQBand changes one band. It updates the shown EQ at once, so quick
// repeated presses build on each other instead of on the last polled state.
func (m *model) setEQBand(band int, delta float64) tea.Cmd {
	if m.snap == nil {
		return nil
	}
	snap := *m.snap
	snap.EQBands = make([]float64, 10)
	copy(snap.EQBands, m.snap.EQBands)
	v := max(-12, min(12, snap.EQBands[band]+delta))
	snap.EQBands[band], snap.EQPreset = v, "Custom"
	m.snap = &snap
	c := m.c
	return func() tea.Msg {
		_, err := c.op("eq", map[string]any{"band": band, "value": v})
		return opMsg{label: "", err: err, eq: true}
	}
}

func (m model) nextPreset() tea.Cmd {
	i := 0
	if m.snap != nil {
		if j := slices.Index(eqPresets, m.snap.EQPreset); j >= 0 {
			i = (j + 1) % len(eqPresets)
		}
	}
	return m.setPreset(eqPresets[i])
}

// setPreset applies one of cliamp's EQ presets; "Flat" turns the EQ off.
func (m model) setPreset(name string) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		_, err := c.op("eq", map[string]string{"name": name})
		return opMsg{label: "eq " + name, err: err, eq: true}
	}
}

// position extrapolates between state polls so the clock ticks smoothly.
func (m model) position() float64 {
	if m.snap == nil {
		return 0
	}
	p := m.snap.Position
	if m.snap.State == "playing" {
		sp := m.snap.Speed
		if sp <= 0 {
			sp = 1
		}
		p += time.Since(m.snapAt).Seconds() * sp
	}
	if m.snap.Duration > 0 {
		p = min(p, m.snap.Duration)
	}
	return p
}

func mmss(s float64) string {
	if s < 0 {
		s = 0
	}
	t := int(s)
	if t >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", t/3600, t/60%60, t%60)
	}
	return fmt.Sprintf("%02d:%02d", t/60, t%60)
}

func firstSelectable(rows []row) int {
	for i, r := range rows {
		if r.kind != rowHeader {
			return i
		}
	}
	return 0
}
