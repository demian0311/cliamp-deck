package main

import (
	"fmt"
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
	histLen       = 600
	hiRes         = 480             // dot columns the spectrum is smoothed at; panels sample from it
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
	hi    []float64 // bands resampled to hiRes dot columns, auto-gained
	gain  float64   // slow-moving loudest band, the auto-gain reference
	pk    peaks     // falling caps over hi
	hist  []float64
	marq  float64

	// Visualizer: mode 0 is the braille spectrum, 1.. index effects[mode-1].
	effects  []fx.Effect
	mode     int
	an       *fx.Analyzer
	frame    *fx.Frame
	start    time.Time
	lastSpec time.Time

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
	stations     []ipc.PlaylistInfo // the open provider's list, as loaded so far
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
	queueRev     uint64
	catalog      bool // the open provider is a paged catalog with more to load

	statePath string
	saved     deckState
	eqApplied bool // the saved EQ has been reapplied since attaching
	eqDirty   bool // an EQ change is waiting for fresh state to be saved

	note   string
	noteAt time.Time
}

func newModel(c client, themePath, statePath string) model {
	th, _ := fx.LoadTheme(themePath)
	m := model{c: c, effects: fx.Stock(), mode: 1, an: &fx.Analyzer{}, frame: &fx.Frame{},
		start: time.Now(), themePath: themePath, theme: th, themeMod: modTime(themePath),
		focus: focusVis, statePath: statePath, lastClickRow: -1}
	m.saved = loadState(statePath)
	for i, name := range m.modeNames() {
		if name == m.saved.Visualizer {
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

func (m model) ls() layoutState { return layoutState{take: m.take, eqOpen: m.eqOpen, focus: m.focus} }

func (m model) modeNames() []string {
	names := []string{"spectrum"}
	for _, e := range m.effects {
		names = append(names, e.Name())
	}
	return names
}

func (m model) modeName() string { return m.modeNames()[m.mode] }

func (m *model) cycle(d int) {
	n := len(m.effects) + 1
	m.mode = (m.mode + d + n) % n
	m.saved.Visualizer = m.modeName()
	m.persist()
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.provider == "" && !m.inResults {
			m.lists[tabSources].rows = m.topRows()
		}

	case playlistsMsg:
		m.loading = false
		if msg.err != nil {
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
		m.stations = msg.list
		if !msg.keepSel {
			m.inCountry = false
		}
		rows := m.playlistRows(msg.provider, msg.list)
		sel := firstSelectable(rows)
		for i, r := range rows {
			if selKey != "" && r.key == selKey {
				sel = i
			}
		}
		m.lists[tabSources] = listState{rows: rows, sel: sel}
		m.lists[tabSources].move(0)

	case queueMsg:
		if msg.err != nil {
			m.say("queue: " + msg.err.Error())
			return m, nil
		}
		sel := m.lists[tabQueue].sel
		m.lists[tabQueue] = listState{rows: m.queueRows(msg.tracks), sel: sel}
		m.lists[tabQueue].move(0)

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

func (m model) onSpectrum(msg specMsg) (tea.Model, tea.Cmd) {
	m.bands = msg.bands
	// Auto-gain for the braille spectrum: magnitudes swing with source loudness
	// and volume, so scale to a slowly decaying recent peak. The floor stops
	// silence being amplified into noise.
	top := 0.0
	for _, v := range msg.bands {
		top = max(top, v)
	}
	m.gain = max(top, m.gain*0.995, 0.003)
	scaled := make([]float64, len(msg.bands))
	lvl := 0.0
	for i, v := range msg.bands {
		scaled[i] = v / m.gain * 0.9
		lvl += scaled[i]
	}
	if len(scaled) > 0 {
		lvl /= float64(len(scaled))
	}
	m.hi = resample(scaled, hiRes)
	m.pk.update(m.hi)
	m.hist = append(m.hist, lvl)
	if len(m.hist) > histLen {
		m.hist = m.hist[len(m.hist)-histLen:]
	}
	m.marq += 0.2
	now := time.Now()
	dt := 1.0 / 30
	if !m.lastSpec.IsZero() {
		dt = min(0.1, now.Sub(m.lastSpec).Seconds())
	}
	m.lastSpec = now
	a := m.an.Update(msg.bands, dt) // raw: the analyzer normalises per signal
	if m.mode > 0 && m.w > 0 {
		if in := computeLayout(m.w, m.h, m.ls()).visInner(); !in.empty() {
			m.frame.Resize(in.w, in.h*2)
			m.effects[m.mode-1].Render(m.frame, a, now.Sub(m.start).Seconds(), dt, m.theme)
		}
	}
	return m, m.fetchSpec()
}

func (m model) onState(msg stateMsg) (tea.Model, tea.Cmd) {
	m.snap, m.snapAt, m.online = msg.snap, time.Now(), true
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
	}
	if m.tab == tabQueue && m.snap.PlaylistRevision != m.queueRev {
		m.queueRev = m.snap.PlaylistRevision
		cmds = append(cmds, fetchQueue(m.c))
	}
	if m.tab == tabSources && m.provider == "" && !m.inResults {
		sel := m.lists[tabSources].sel
		m.lists[tabSources] = listState{rows: m.topRows(), sel: sel}
		m.lists[tabSources].move(0)
	}
	return m, tea.Batch(cmds...)
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
	name, c := eqPresets[i], m.c
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
