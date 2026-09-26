package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

// setupProviders are the sources `cliamp setup` can configure, from its
// wizard list (cmd/setup.go at cliamp 2342c0d, 2026-09-25; the --help text
// lists fewer). provider.list only reports configured ones, so the deck
// carries this list to show what is still to set up. keys are the
// provider.list keys that mean "already configured".
var setupProviders = []struct {
	name string
	keys []string
}{
	{"Spotify", []string{"spotify"}},
	{"YouTube Music", []string{"ytmusic", "youtube", "yt"}},
	{"Navidrome", []string{"navidrome"}},
	{"Plex", []string{"plex"}},
	{"Jellyfin", []string{"jellyfin"}},
	{"Emby", []string{"emby"}},
	{"Tidal", []string{"tidal"}},
	{"Qobuz", []string{"qobuz"}},
	{"Mixcloud", []string{"mixcloud"}},
	{"NetEase", []string{"netease"}},
	{"Lyrion", []string{"lyrion"}},
	{"Audiobookshelf", []string{"audiobookshelf"}},
}

type (
	setupDoneMsg struct {
		err     error
		changed bool // the wizard rewrote cliamp's config.toml
	}
	daemonRestartMsg struct {
		restarted bool
		err       error
	}
)

// runSetup hands the terminal to cliamp's own setup wizard. The socket lives
// in cliamp's config directory, so config.toml sits beside it; comparing it
// before and after tells a finished setup from one that was quit.
func runSetup(c client) tea.Cmd {
	cfg := filepath.Join(filepath.Dir(c.sock), "config.toml")
	before, _ := os.ReadFile(cfg)
	return tea.ExecProcess(exec.Command("cliamp", "setup"), func(err error) tea.Msg {
		after, _ := os.ReadFile(cfg)
		return setupDoneMsg{err: err, changed: !bytes.Equal(before, after)}
	})
}

// restartDaemon restarts cliamp so it loads newly configured sources. Only a
// headless daemon is restarted; a cliamp TUI belongs to the user's terminal.
func restartDaemon(c client) tea.Cmd {
	return func() tea.Msg {
		b, err := os.ReadFile(c.sock + ".pid")
		if err != nil {
			return daemonRestartMsg{err: err}
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			return daemonRestartMsg{err: err}
		}
		cmdline, _ := os.ReadFile("/proc/" + strings.TrimSpace(string(b)) + "/cmdline")
		if !strings.Contains(string(cmdline), "--daemon") && !strings.Contains(string(cmdline), "\x00-d") {
			return daemonRestartMsg{restarted: false}
		}
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			return daemonRestartMsg{err: err}
		}
		for range 50 {
			if !c.alive() {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		return daemonRestartMsg{restarted: true, err: spawnDaemon(c)}
	}
}
