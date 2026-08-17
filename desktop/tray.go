package desktop

import (
	"context"
	"fmt"
	"log"
	"os/exec"

	"github.com/getlantern/systray"
	"github.com/toshon-jennings/gatekey-proxy/buildinfo"
	"github.com/toshon-jennings/gatekey-proxy/server"
)

const (
	defaultPort = "8181"
	dashboardURL = "http://127.0.0.1:8181/"
)

// Run starts the Gatekey Proxy menu bar desktop app.
func Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := server.NewProxyServer(defaultPort)
	go func() {
		if err := srv.StartWithContext(ctx); err != nil {
			log.Printf("Proxy server stopped: %v", err)
		}
	}()

	onReady := func() {
		if len(trayIconBytes) > 0 {
			systray.SetTemplateIcon(trayIconBytes, trayIconBytes)
		}
		systray.SetTooltip("Gatekey Proxy")

		mOpen := systray.AddMenuItem("Open Dashboard", "Open Gatekey Dashboard in your default browser")

		systray.AddSeparator()

		mStatus := systray.AddMenuItem(fmt.Sprintf("Status: Running on 127.0.0.1:%s", defaultPort), "Proxy status")
		mStatus.Disable()

		mVersion := systray.AddMenuItem(fmt.Sprintf("Gatekey Proxy %s", buildinfo.Current()), "Installed version")
		mVersion.Disable()

		systray.AddSeparator()

		mAutostart := systray.AddMenuItemCheckbox("Launch at Login", "Start Gatekey Proxy automatically at login", IsLaunchAtLoginEnabled())

		systray.AddSeparator()

		mQuit := systray.AddMenuItem("Quit Gatekey", "Stop proxy and quit")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					_ = openURL(dashboardURL)

				case <-mAutostart.ClickedCh:
					if mAutostart.Checked() {
						if err := SetLaunchAtLogin(false); err == nil {
							mAutostart.Uncheck()
						} else {
							log.Printf("Failed to disable launch at login: %v", err)
						}
					} else {
						if err := SetLaunchAtLogin(true); err == nil {
							mAutostart.Check()
						} else {
							log.Printf("Failed to enable launch at login: %v", err)
						}
					}

				case <-mQuit.ClickedCh:
					cancel()
					systray.Quit()
					return

				case <-ctx.Done():
					systray.Quit()
					return
				}
			}
		}()
	}

	onExit := func() {
		cancel()
	}

	systray.Run(onReady, onExit)
}

func openURL(url string) error {
	return exec.Command("open", url).Start()
}
