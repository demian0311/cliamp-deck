package main

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/ipc"
)

const (
	specInterval  = 33 * time.Millisecond // spectrum.get rate, ~30 fps
	stateInterval = 500 * time.Millisecond
	histLen       = 600
	hiRes         = 480 // dot columns the spectrum is smoothed at; panels sample from it
)

type (
	specMsg      struct{ bands []float64 }
	stateMsg     struct{ snap *ipc.RuntimeSnapshot }
	connMsg      struct{ err error }
	providersMsg struct {
		list []ipc.ProviderInfo
		err  error
	}
	playlistsMsg struct {
		provider string
		list     []ipc.PlaylistInfo
		err      error
	}
	opMsg struct {
		label string
		err   error
	}
)

// entry is one row of the sources panel: a provider at the top level, a
// playlist or station once a provider is open.
type entry struct {
	key, name, section string
}

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

	providers []ipc.ProviderInfo
	provider  string // open provider key; "" = provider list
	items     []entry
	sel       int
	loading   bool

	note   string
	noteAt time.Time
}

func newModel(c client) model { return model{c: c} }

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchState(), m.fetchSpec(), m.fetchProviders())
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
		return opMsg{label, err}
	}
}

func (m *model) say(s string) { m.note, m.noteAt = s, time.Now() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height

	case specMsg:
		m.bands = msg.bands
		// Auto-gain: cliamp's band magnitudes swing with source loudness and
		// volume, so scale to a slowly decaying recent peak to keep the graph
		// using its height. The floor stops silence being amplified into noise.
		top := 0.0
		for _, v := range msg.bands {
			top = max(top, v)
		}
		m.gain = max(top, m.gain*0.995, 0.003)
		scaled := make([]float64, len(msg.bands))
		for i, v := range msg.bands {
			scaled[i] = v / m.gain * 0.9
		}
		m.hi = resample(scaled, hiRes)
		m.pk.update(m.hi)
		lvl := 0.0
		for _, v := range scaled {
			lvl += v
		}
		if len(scaled) > 0 {
			lvl /= float64(len(scaled))
		}
		m.hist = append(m.hist, lvl)
		if len(m.hist) > histLen {
			m.hist = m.hist[len(m.hist)-histLen:]
		}
		m.marq += 0.2
		return m, m.fetchSpec()

	case stateMsg:
		m.snap, m.snapAt, m.online = msg.snap, time.Now(), true
		return m, tea.Tick(stateInterval, func(time.Time) tea.Msg { return m.fetchState()() })

	case connMsg:
		m.online = false
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return m.fetchState()() })

	case providersMsg:
		if msg.err != nil {
			m.say("sources: " + msg.err.Error())
			return m, nil
		}
		m.providers = msg.list
		if m.provider == "" {
			m.showProviders()
		}

	case playlistsMsg:
		m.loading = false
		if msg.err != nil {
			m.say(msg.provider + ": " + msg.err.Error())
			return m, nil
		}
		m.provider = msg.provider
		m.items = nil
		for _, p := range msg.list {
			m.items = append(m.items, entry{key: p.ID, name: p.Name, section: p.Section})
		}
		m.sel = 0

	case opMsg:
		if msg.err != nil {
			m.say(msg.label + ": " + msg.err.Error())
		} else if msg.label != "" {
			m.say(msg.label)
		}
		return m, m.fetchState()

	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

func (m *model) showProviders() {
	m.provider, m.sel = "", 0
	m.items = nil
	for _, p := range m.providers {
		m.items = append(m.items, entry{key: p.Key, name: p.Name})
	}
}

func (m model) key(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "space":
		return m, m.run("", "toggle", nil)
	case "n":
		return m, m.run("next", "next", nil)
	case "p":
		return m, m.run("previous", "prev", nil)
	case "s":
		return m, m.run("stopped", "stop", nil)
	case "+", "=":
		return m, m.run("", "volume.adjust", map[string]float64{"value": 2})
	case "-", "_":
		return m, m.run("", "volume.adjust", map[string]float64{"value": -2})
	case "right", "l":
		return m, m.run("", "seek", map[string]float64{"value": 5})
	case "left", "h":
		return m, m.run("", "seek", map[string]float64{"value": -5})
	case "down", "j":
		m.sel = min(len(m.items)-1, m.sel+1)
	case "up", "k":
		m.sel = max(0, m.sel-1)
	case "pgdown":
		m.sel = min(len(m.items)-1, m.sel+10)
	case "pgup":
		m.sel = max(0, m.sel-10)
	case "esc", "backspace":
		if m.provider != "" {
			m.showProviders()
		}
	case "enter":
		if m.sel < 0 || m.sel >= len(m.items) || m.loading {
			return m, nil
		}
		it := m.items[m.sel]
		if m.provider == "" {
			m.loading = true
			m.say("opening " + it.name + "…")
			c := m.c
			return m, func() tea.Msg {
				l, err := c.playlists(it.key)
				return playlistsMsg{it.key, l, err}
			}
		}
		m.say("loading " + it.name + "…")
		return m, m.run("playing "+it.name, "provider.load",
			map[string]string{"provider": m.provider, "playlist": it.key})
	}
	return m, nil
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
