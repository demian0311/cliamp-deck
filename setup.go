package main

import (
	"bytes"
	"fmt"
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

// headlessPID is cliamp's process when it runs as a headless daemon. headless
// is false for a cliamp TUI, which belongs to the user's terminal.
func headlessPID(c client) (pid int, headless bool, err error) {
	b, err := os.ReadFile(c.sock + ".pid")
	if err != nil {
		return 0, false, err
	}
	if pid, err = strconv.Atoi(strings.TrimSpace(string(b))); err != nil {
		return 0, false, err
	}
	cmdline, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	return pid, strings.Contains(string(cmdline), "--daemon") || strings.Contains(string(cmdline), "\x00-d"), nil
}

// stopDaemon asks a headless cliamp to exit and waits for its socket to go quiet.
func stopDaemon(c client, pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	for range 50 {
		if !c.alive() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("cliamp (pid %d) still answering after SIGTERM", pid)
}

// restartDaemon restarts cliamp so it loads newly configured sources. Only a
// headless daemon is restarted; a cliamp TUI belongs to the user's terminal.
func restartDaemon(c client) tea.Cmd {
	return func() tea.Msg {
		pid, headless, err := headlessPID(c)
		if err != nil {
			return daemonRestartMsg{err: err}
		}
		if !headless {
			return daemonRestartMsg{restarted: false}
		}
		if err := stopDaemon(c, pid); err != nil {
			return daemonRestartMsg{err: err}
		}
		return daemonRestartMsg{restarted: true, err: spawnDaemon(c)}
	}
}

// stopCliamp ends the music when the deck quits. A headless daemon exits with
// it (the next launch spawns a fresh one); a cliamp TUI keeps running in its
// own terminal and only has its playback stopped.
func stopCliamp(c client) error {
	if pid, headless, err := headlessPID(c); err == nil && headless {
		return stopDaemon(c, pid)
	}
	_, err := c.op("stop", nil)
	return err
}
