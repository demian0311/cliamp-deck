package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bjarneo/cliamp/ipc"
)

// deckState is what the deck remembers between runs (docs/design/layout.md,
// Persistence). cliamp's daemon applies EQ changes live but never saves them,
// so the deck keeps the EQ and reapplies it when it attaches.
type deckState struct {
	Visualizer string
	EQPreset   string
	EQBands    []float64 // 10 values in dB; used when EQPreset is "" or "Custom"

	// Where the sources list was left: the open provider, the radio country
	// open in it (InCountry, since "" is the no-country group), and the
	// selected row's key.
	Source, SourceCountry, SourceSelected string
	SourceInCountry                       bool

	// LastTrack is what was last playing (a radio station's stream, or a
	// track), so the deck can start it again when it attaches to an idle
	// cliamp. Stored as JSON, since cliamp's track.play takes the TrackInfo.
	LastTrack *ipc.TrackInfo
}

func defaultStatePath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "cliamp-deck", "state.toml")
}

func loadState(path string) deckState {
	var s deckState
	f, err := os.Open(path)
	if err != nil {
		return s
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "visualizer":
			s.Visualizer = strings.Trim(v, `"`)
		case "eq_preset":
			s.EQPreset = strings.Trim(v, `"`)
		case "source":
			s.Source = unquote(v)
		case "source_country":
			s.SourceCountry = unquote(v)
		case "source_in_country":
			s.SourceInCountry = v == "true"
		case "source_selected":
			s.SourceSelected = unquote(v)
		case "last_track":
			var t ipc.TrackInfo
			if json.Unmarshal([]byte(unquote(v)), &t) == nil && t.Path != "" {
				s.LastTrack = &t
			}
		case "eq_bands":
			for _, f := range strings.Split(strings.Trim(v, "[]"), ",") {
				if n, err := strconv.ParseFloat(strings.TrimSpace(f), 64); err == nil {
					s.EQBands = append(s.EQBands, n)
				}
			}
		}
	}
	return s
}

func saveState(path string, s deckState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	bands := make([]string, len(s.EQBands))
	for i, b := range s.EQBands {
		bands[i] = strconv.FormatFloat(b, 'f', -1, 64)
	}
	last := ""
	if s.LastTrack != nil {
		b, _ := json.Marshal(s.LastTrack)
		last = string(b)
	}
	body := fmt.Sprintf("# cliamp-deck state, rewritten by the deck\nvisualizer = %q\neq_preset = %q\neq_bands = [%s]\n"+
		"source = %q\nsource_country = %q\nsource_in_country = %t\nsource_selected = %q\nlast_track = %q\n",
		s.Visualizer, s.EQPreset, strings.Join(bands, ", "),
		s.Source, s.SourceCountry, s.SourceInCountry, s.SourceSelected, last)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// unquote reads a value written with %q, falling back to trimming quotes.
func unquote(v string) string {
	if u, err := strconv.Unquote(v); err == nil {
		return u
	}
	return strings.Trim(v, `"`)
}
