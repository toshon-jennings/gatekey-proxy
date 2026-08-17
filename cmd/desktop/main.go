package main

import (
	"fmt"
	"os"

	"github.com/toshon-jennings/gatekey-proxy/buildinfo"
	"github.com/toshon-jennings/gatekey-proxy/desktop"
	"github.com/toshon-jennings/gatekey-proxy/updater"
)

func main() {
	applied, err := updater.ApplyStaged(buildinfo.Current())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not apply the staged Gatekey update: %v\n", err)
	}
	if applied {
		if err := updater.Relaunch(os.Args); err != nil {
			fmt.Fprintf(os.Stderr, "Gatekey was updated but could not restart automatically: %v\n", err)
		}
	}
	desktop.Run()
}
