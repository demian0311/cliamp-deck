// cliamp-deck is an alternative terminal face for cliamp. It renders nothing
// of its own audio: playback, providers and the spectrum all come from a
// running cliamp over its V2 IPC socket, so every upstream fix arrives free.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/ipc"

	"github.com/demian0311/cliamp-deck/fx"
)

func main() {
	sock := flag.String("socket", ipc.DefaultSocketPath(), "cliamp IPC socket")
	theme := flag.String("theme", fx.OmarchyColors(), "Omarchy colors.toml the visualizers paint with")
	statePath := flag.String("state", defaultStatePath(), "where the deck keeps its EQ and visualizer between runs")
	spawn := flag.Bool("spawn", true, "start `cliamp --daemon` when nothing answers on the socket")
	keep := flag.Bool("keep-playing", false, "leave cliamp playing after the deck quits")
	icons := flag.String("icons", "auto", "control icons: nerd (needs a Nerd Font), plain, or auto (nerd on Omarchy)")
	flag.Parse()
	pickGlyphs(*icons, *theme)

	c := client{sock: *sock}
	if !c.alive() && *spawn {
		if err := spawnDaemon(c); err != nil {
			fmt.Fprintln(os.Stderr, "cliamp-deck:", err)
			os.Exit(1)
		}
	}
	if c.alive() {
		if err := checkCompat(c); err != nil {
			fmt.Fprintln(os.Stderr, "cliamp-deck:", err)
			os.Exit(1)
		}
	}
	if _, err := tea.NewProgram(newModel(c, *theme, *statePath)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cliamp-deck:", err)
		os.Exit(1)
	}
	if !*keep && c.alive() {
		if err := stopCliamp(c); err != nil {
			fmt.Fprintln(os.Stderr, "cliamp-deck: stopping cliamp:", err)
		}
	}
}
