package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/bjarneo/cliamp/ipc"
)

// requiredOps are the V2 operations the deck calls. A cliamp missing any of
// them would leave keys or the sources panel silently dead, so refuse to
// start instead and say what to update.
var requiredOps = []string{
	"toggle", "play", "pause", "next", "prev", "stop", "seek", "volume.adjust", "shuffle",
	"provider.list", "provider.playlists", "provider.load",
}

func (c client) capabilities() ([]string, error) {
	resp, err := c.send(ipc.V2Request{Method: "capabilities"})
	if err != nil {
		return nil, err
	}
	var caps []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp.Result, &caps); err != nil {
		return nil, fmt.Errorf("decode capabilities: %w", err)
	}
	names := make([]string, len(caps))
	for i, c := range caps {
		names[i] = c.Name
	}
	return names, nil
}

func missingOps(have []string) []string {
	var missing []string
	for _, op := range requiredOps {
		if !slices.Contains(have, op) {
			missing = append(missing, op)
		}
	}
	return missing
}

// checkCompat asks the running cliamp what it supports. The error is written
// for the person who installed the deck: what is wrong and what to do.
func checkCompat(c client) error {
	have, err := c.capabilities()
	if err != nil {
		return fmt.Errorf("the cliamp on %s doesn't speak the V2 remote API (%v).\n"+
			"cliamp-deck needs a newer cliamp%s: update it (Omarchy: `omarchy update`; AUR: `yay -S cliamp`)",
			c.sock, err, installedVersion())
	}
	if missing := missingOps(have); len(missing) > 0 {
		return fmt.Errorf("the running cliamp%s lacks operations cliamp-deck uses: %s.\n"+
			"Update cliamp (Omarchy: `omarchy update`; AUR: `yay -S cliamp`), then restart it",
			installedVersion(), strings.Join(missing, ", "))
	}
	return nil
}

// installedVersion is " (you have v2.0.1)" from `cliamp --version`, or "".
// It names the binary on PATH, which is normally the one that is running.
func installedVersion() string {
	out, err := exec.Command("cliamp", "--version").Output()
	if err != nil {
		return ""
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return ""
	}
	return " (you have " + f[len(f)-1] + ")"
}
