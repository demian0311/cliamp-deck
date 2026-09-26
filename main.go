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

	"cliamp-deck/fx"
)

func main() {
	sock := flag.String("socket", ipc.DefaultSocketPath(), "cliamp IPC socket")
	theme := flag.String("theme", fx.OmarchyColors(), "Omarchy colors.toml the visualizers paint with")
	spawn := flag.Bool("spawn", true, "start `cliamp --daemon` when nothing answers on the socket")
	flag.Parse()

	c := client{sock: *sock}
	if !c.alive() && *spawn {
		if err := spawnDaemon(c); err != nil {
			fmt.Fprintln(os.Stderr, "cliamp-deck:", err)
			os.Exit(1)
		}
	}
	if _, err := tea.NewProgram(newModel(c, *theme)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cliamp-deck:", err)
		os.Exit(1)
	}
}
