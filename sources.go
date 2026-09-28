package main

import (
	"cmp"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/ipc"
)

// The list area has two tabs. "sources" drills providers → playlists → a
// playlist's tracks (cliamp's live playlist, once it is the one loaded) and
// also holds search results; history is cliamp's own list.
const (
	tabSources = iota
	tabHistory
	tabCount
)

var tabNames = [tabCount]string{"sources", "history"}

type rowKind int

const (
	rowProvider   rowKind = iota // a configured source; Enter opens it
	rowPlaylist                  // a playlist or station; Enter loads it, → goes inside a playlist
	rowTrack                     // a single track; Enter plays, a appends, A plays next
	rowHeader                    // a group label, not selectable for actions
	rowSetup                     // a source cliamp can set up; s or Enter runs cliamp setup
	rowCountry                   // a country of radio stations; Enter or → opens it
	rowNowPlaying                // what cliamp has loaded; Enter or → opens it
)

type row struct {
	kind          rowKind
	label, right  string
	key, provider string
	track         *ipc.TrackInfo
	index         int  // position in cliamp's live playlist, when live
	live          bool // a track of cliamp's live playlist
	current       bool // what is playing now
	color         cls  // the label's colour when not selected or playing; cNone = default
	byline        int  // runes at the label's end naming the artist, drawn faded
}

type listState struct {
	rows []row
	sel  int
}

// move steps the selection by d, stepping over group headers in the
// direction of travel (or back, when a header ends the list).
func (l *listState) move(d int) {
	if len(l.rows) == 0 {
		l.sel = 0
		return
	}
	l.sel = max(0, min(len(l.rows)-1, l.sel+d))
	step := 1
	if d < 0 {
		step = -1
	}
	for _, s := range []int{step, -step} {
		for i := l.sel; i >= 0 && i < len(l.rows); i += s {
			if l.rows[i].kind != rowHeader {
				l.sel = i
				return
			}
		}
	}
}

// listStart is the first visible row so the selection stays in view.
func listStart(n, sel, rows int) int {
	if n <= rows || rows <= 0 {
		return 0
	}
	return max(0, min(n-rows, sel-rows/2))
}

type (
	// tracksMsg is an open playlist's rows: its tracks, or cliamp's live
	// playlist (live) once it is the one loaded. key is the fetch it answers.
	tracksMsg struct {
		key    string
		tracks []ipc.TrackInfo
		live   bool
		err    error
	}
	historyMsg struct {
		items []ipc.HistoryInfo
		err   error
	}
	searchGroup struct {
		name   string
		tracks []ipc.TrackInfo
		err    error
	}
	searchMsg struct {
		query  string
		groups []searchGroup
	}
)

func (c client) response(op string, params any) (ipc.Response, error) {
	var r ipc.Response
	raw, err := c.op(op, params)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(raw, &r)
	return r, err
}

// allTracks reads a paged track list (queue.list, provider.tracks) to its end.
func allTracks(c client, op string, params map[string]any) ([]ipc.TrackInfo, error) {
	const page = 200
	var out []ipc.TrackInfo
	for {
		p := map[string]any{"offset": len(out), "limit": page}
		maps.Copy(p, params)
		r, err := c.response(op, p)
		if err != nil {
			return nil, err
		}
		out = append(out, r.Tracks...)
		if len(r.Tracks) < page || (r.Total > 0 && len(out) >= r.Total) {
			return out, nil
		}
	}
}

func fetchHistory(c client) tea.Cmd {
	return func() tea.Msg {
		r, err := c.response("history", map[string]int{"limit": 100})
		return historyMsg{r.History, err}
	}
}

// search asks every provider in parallel, so slow ones (YouTube, Spotify) only
// delay their own group.
func search(c client, query string, providers []ipc.ProviderInfo) tea.Cmd {
	return func() tea.Msg {
		groups := make([]searchGroup, len(providers))
		var wg sync.WaitGroup
		for i, p := range providers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := c.response("provider.search", map[string]any{"provider": p.Key, "query": query, "limit": 25})
				groups[i] = searchGroup{p.Name, r.Tracks, err}
			}()
		}
		wg.Wait()
		return searchMsg{query, groups}
	}
}

func (m *model) topRows() []row {
	var rows []row
	if name, ok := m.nowPlayingName(); ok {
		rows = append(rows, row{kind: rowNowPlaying, label: "now playing › " + name, color: cGreen})
	}
	configured := map[string]bool{}
	for _, p := range m.providers {
		configured[p.Key] = true
		rows = append(rows, row{kind: rowProvider, label: p.Name, key: p.Key,
			current: m.snap != nil && strings.HasPrefix(m.snap.Playlist, p.Key+":")})
	}
	var setup []row
	for _, sp := range setupProviders {
		done := false
		for _, k := range sp.keys {
			done = done || configured[k]
		}
		if !done {
			setup = append(setup, row{kind: rowSetup, label: sp.name, right: "s to connect"})
		}
	}
	if len(setup) > 0 && len(m.providers) > 0 {
		rows = append(rows, row{kind: rowHeader, label: "not set up"})
		rows = append(rows, setup...)
	}
	return rows
}

// playlistRows lists the open provider's playlists. Radio directory stations
// (catalog, favorite and search IDs) arrive named "Name [128k] · Country"; the
// bitrate moves to the dim right column. Catalog stations are not listed flat:
// the provider level shows their countries, most stations first, and opening
// a country lists its stations.
func (m *model) playlistRows(provider string, list []ipc.PlaylistInfo) []row {
	var rows []row
	var countries []string
	byCountry := map[string][]row{}
	for _, p := range list {
		r := row{kind: rowPlaylist, label: p.Name, right: p.Section, key: p.ID, provider: provider,
			current: m.snap != nil && m.snap.Playlist == provider+":"+p.ID}
		prefix, _, _ := strings.Cut(p.ID, ":")
		if prefix != "c" && prefix != "f" && prefix != "s" {
			rows = append(rows, r)
			continue
		}
		name, bitrate, country := splitStation(p.Name)
		r.label, r.right, r.color = name, bitrate, cCyan
		if prefix != "c" {
			rows = append(rows, r)
			continue
		}
		if _, seen := byCountry[country]; !seen {
			countries = append(countries, country)
		}
		byCountry[country] = append(byCountry[country], r)
	}
	if m.inCountry {
		return byCountry[m.country]
	}
	// Most stations first, ties alphabetical; stations with no country go
	// last, however many.
	slices.SortFunc(countries, func(a, b string) int {
		if (a == "") != (b == "") {
			return cmp.Compare(b, a)
		}
		if n := cmp.Compare(len(byCountry[b]), len(byCountry[a])); n != 0 {
			return n
		}
		return strings.Compare(a, b)
	})
	if len(countries) > 0 && len(rows) > 0 {
		rows = append(rows, row{kind: rowHeader, label: "countries"})
	}
	for _, c := range countries {
		stations := byCountry[c]
		playing := slices.ContainsFunc(stations, func(r row) bool { return r.current })
		rows = append(rows, row{kind: rowCountry, label: countryLabel(c), right: strconv.Itoa(len(stations)) + " ›",
			key: countryKey + c, provider: provider, current: playing, color: cYellow})
	}
	return rows
}

// isStation reports a radio station ID (local, catalog, favorite or search):
// one stream, so there are no tracks to go inside to.
func isStation(id string) bool {
	prefix, _, ok := strings.Cut(id, ":")
	return ok && (prefix == "l" || prefix == "c" || prefix == "f" || prefix == "s")
}

// loadedPlaylist is the provider playlist cliamp has loaded, when it is one
// with tracks rather than a station.
func (m *model) loadedPlaylist() (provider, playlist string, ok bool) {
	if m.snap == nil {
		return "", "", false
	}
	provider, playlist, _ = strings.Cut(m.snap.Playlist, ":")
	return provider, playlist, playlist != "" && !isStation(playlist)
}

// nowPlayingName names what cliamp has loaded, for the now-playing row: the
// provider playlist, else the track or station. ok is false when nothing is.
func (m *model) nowPlayingName() (string, bool) {
	if provider, playlist, ok := m.loadedPlaylist(); ok {
		s := m.saved
		switch {
		case m.provider == provider && m.playlist == playlist && m.playlistName != "":
			return m.playlistName, true
		case s.LastProvider == provider && s.LastPlaylist == playlist && s.LastPlaylistName != "":
			return s.LastPlaylistName, true
		case m.stationsOf == provider:
			for _, p := range m.stations {
				if p.ID == playlist {
					return p.Name, true
				}
			}
		}
		return playlist, true
	}
	if m.snap == nil || m.snap.Total == 0 {
		return "", false
	}
	if t := m.snap.Track; t != nil {
		return cmp.Or(t.Station, trackTitle(t.Artist, t.Title), t.Path), true
	}
	return "queue", true
}

// countryKey prefixes a country row's key so it never matches a playlist ID.
const countryKey = "country:"

func countryLabel(c string) string {
	if c == "" {
		return "elsewhere"
	}
	return c
}

// shortCountry trims the directory's official country names to what people
// call them, so a header does not read "Russian Federation".
var shortCountry = map[string]string{
	"United States Of America":                             "United States",
	"United Kingdom Of Great Britain And Northern Ireland": "United Kingdom",
	"Russian Federation":                                   "Russia",
	"Islamic Republic Of Iran":                             "Iran",
	"Republic Of Korea":                                    "South Korea",
	"Democratic People's Republic Of Korea":                "North Korea",
	"Plurinational State Of Bolivia":                       "Bolivia",
	"Bolivarian Republic Of Venezuela":                     "Venezuela",
	"Republic Of Moldova":                                  "Moldova",
	"United Republic Of Tanzania":                          "Tanzania",
	"Syrian Arab Republic":                                 "Syria",
	"Lao People's Democratic Republic":                     "Laos",
	"Viet Nam":                                             "Vietnam",
	"Taiwan, Republic Of China":                            "Taiwan",
	"The Democratic Republic Of The Congo":                 "DR Congo",
}

var bitrateSuffix = regexp.MustCompile(` \[(\d+k)\]$`)

// splitStation takes cliamp's "Name [128k] · Country" apart; either suffix
// may be missing.
func splitStation(s string) (name, bitrate, country string) {
	name = strings.TrimSpace(s)
	if i := strings.LastIndex(name, " · "); i >= 0 {
		name, country = name[:i], name[i+len(" · "):]
		if short, ok := shortCountry[country]; ok {
			country = short
		}
	}
	if m := bitrateSuffix.FindStringSubmatchIndex(name); m != nil {
		name, bitrate = name[:m[0]], name[m[2]:m[3]]
	}
	return stripFormat(strings.TrimSpace(name)), bitrate, country
}

var (
	bitrateWord = regexp.MustCompile(`(?i)^\d+\s?k(b|bps|bit)?(/s)?$`)
	codecWord   = map[string]bool{"aac": true, "aac+": true, "he-aac": true, "mp3": true, "opus": true, "ogg": true, "flac": true, "vorbis": true}
	qualityWord = map[string]bool{"hd": true, "hq": true, "stereo": true, "mono": true}
	bracketed   = regexp.MustCompile(`\s*[(\[]([^()\[\]]*)[)\]]`)
)

// formatWords reports whether words describe only the stream's encoding
// ("128k MP3", "AAC HD 256k"): all bitrate, codec or quality words, with at
// least one bitrate or codec so a bare "HD" in a station's name survives.
func formatWords(words []string) bool {
	real := false
	for _, w := range words {
		lw := strings.ToLower(w)
		switch {
		case bitrateWord.MatchString(w) || codecWord[lw]:
			real = true
		case !qualityWord[lw]:
			return false
		}
	}
	return real
}

// stripFormat drops the encoding some stations put in their name, since the
// bitrate has its own column: bracketed groups like "(128k MP3)" and a
// trailing run like "- AAC HD 256k". A quality word opening the trailing run
// stays, so "Classic Vinyl HD Opus" keeps the HD that tells it apart.
func stripFormat(name string) string {
	out := bracketed.ReplaceAllStringFunc(name, func(g string) string {
		if formatWords(strings.Fields(bracketed.FindStringSubmatch(g)[1])) {
			return ""
		}
		return g
	})
	words := strings.Fields(out)
	i := len(words)
	for i > 0 && (formatWords(words[i-1:i]) || qualityWord[strings.ToLower(words[i-1])]) {
		i--
	}
	if !formatWords(words[i:]) {
		i = len(words)
	}
	for i < len(words) && qualityWord[strings.ToLower(words[i])] {
		i++
	}
	if i < len(words) {
		out = strings.TrimRight(strings.Join(words[:i], " "), " -|,:·")
	}
	if out = strings.TrimSpace(out); out == "" {
		return name
	}
	return out
}

func trackRow(t ipc.TrackInfo, right string) row {
	r := row{kind: rowTrack, right: right, track: &t}
	title, artist := trackParts(t.Artist, t.Title)
	r.label = title
	if artist != "" {
		by := " · " + artist
		r.label, r.byline = title+by, len([]rune(by))
	}
	return r
}

// playlistTracks are an open playlist's rows. Live rows are cliamp's own
// playlist: play-next tracks are tagged +N (their place in line) in cyan, and
// markCurrent keeps the » on the one playing.
func playlistTracks(tracks []ipc.TrackInfo, live bool) []row {
	rows := make([]row, 0, len(tracks))
	for i, t := range tracks {
		r := trackRow(t, durationText(t.DurationSecs))
		r.index, r.live = i, live
		if live && t.QueuePosition > 0 {
			r.right = strings.TrimSpace("+" + strconv.Itoa(t.QueuePosition) + " " + r.right)
			r.color = cCyan
		}
		rows = append(rows, r)
	}
	return rows
}

func historyRows(items []ipc.HistoryInfo) []row {
	rows := make([]row, 0, len(items))
	for _, h := range items {
		when := ""
		if t, err := time.Parse(time.RFC3339, h.PlayedAt); err == nil {
			when = t.Local().Format("Jan 2 15:04")
		}
		rows = append(rows, trackRow(h.Track, when))
	}
	return rows
}

func searchRows(groups []searchGroup) []row {
	var rows []row
	for _, g := range groups {
		switch {
		case g.err != nil:
			rows = append(rows, row{kind: rowHeader, label: g.name + " · " + g.err.Error()})
		case len(g.tracks) == 0:
			rows = append(rows, row{kind: rowHeader, label: g.name + " · no matches"})
		default:
			rows = append(rows, row{kind: rowHeader, label: g.name})
			for _, t := range g.tracks {
				rows = append(rows, trackRow(t, durationText(t.DurationSecs)))
			}
		}
	}
	return rows
}

func durationText(secs int) string {
	if secs <= 0 {
		return ""
	}
	return mmss(float64(secs))
}

// pendingRow is a row whose Enter is still in flight: a source opening, or a
// playlist, station or track that hasn't started playing yet. It gets a
// braille throbber in place of its marker until it settles or times out.
type pendingRow struct {
	tab      int
	key      string // provider or playlist key; "" for a track
	path     string // a track's path
	playlist string // provider:playlist cliamp reports once a playlist plays
	since    time.Time
}

// pendingTimeout gives up the throbber when nothing confirms the load.
const pendingTimeout = 20 * time.Second

var throbber = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func throbberFrame(now time.Time) string {
	return string(throbber[now.UnixMilli()/80%int64(len(throbber))])
}

// matches reports whether r on tab is the row being loaded.
func (p *pendingRow) matches(tab int, r row) bool {
	if p == nil || tab != p.tab {
		return false
	}
	if r.track != nil {
		return p.key == "" && r.track.Path == p.path
	}
	return p.key != "" && r.key == p.key
}

// playing reports whether cliamp is now actually playing what p asked for.
func (p *pendingRow) playing(s *ipc.RuntimeSnapshot) bool {
	if s == nil || s.State != "playing" || s.Position <= 0 {
		return false
	}
	if p.playlist != "" {
		return s.Playlist == p.playlist
	}
	return s.Track != nil && p.path != "" && s.Track.Path == p.path
}
