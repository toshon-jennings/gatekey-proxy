package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/toshon-jennings/gatekey-proxy/buildinfo"
	"github.com/toshon-jennings/gatekey-proxy/config"
	"github.com/toshon-jennings/gatekey-proxy/server"
	"github.com/toshon-jennings/gatekey-proxy/updater"
)

func Run() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "version":
		fmt.Printf("gatekey-proxy %s\n", buildinfo.Current())
	case "update":
		if err := update(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	case "start":
		srv := server.NewProxyServer("8181")
		if err := srv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
			os.Exit(1)
		}
	case "keys":
		if len(os.Args) < 3 {
			fmt.Println("Usage: gatekey-proxy keys [add|list]")
			os.Exit(1)
		}
		subcmd := os.Args[2]
		switch subcmd {
		case "add":
			if len(os.Args) != 5 {
				fmt.Println("Usage: gatekey-proxy keys add <provider> <key>")
				os.Exit(1)
			}
			provider := os.Args[3]
			key := os.Args[4]
			if err := config.SetKey(provider, key); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save key: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Successfully secured key for %s. Configuration is secured with 0600 permissions.\n", provider)
		case "list":
			cfg, err := config.LoadConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to load keys: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Configured providers:")
			for provider := range cfg {
				fmt.Printf("- %s\n", provider)
			}
		default:
			fmt.Println("Unknown keys command.")
			printUsage()
			os.Exit(1)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func update() error {
	manager := updater.New(buildinfo.Current())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	status, err := manager.Check(ctx)
	if err != nil {
		return fmt.Errorf("update check failed: %w", err)
	}
	if !status.InstallSupported {
		fmt.Printf("%s\n", status.Message)
		return nil
	}
	if !status.UpdateAvailable {
		fmt.Printf("Gatekey Proxy %s is up to date.\n", status.CurrentVersion)
		return nil
	}

	status, err = manager.Stage(ctx)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	fmt.Printf("Gatekey Proxy %s is downloaded and verified. Restart Gatekey to apply it.\n", status.StagedVersion)
	return nil
}

func printUsage() {
	fmt.Println("Gatekey Proxy CLI")
	fmt.Println("Usage:")
	fmt.Println("  start                        Start the Gatekey Proxy server on port 8181")
	fmt.Println("  version                      Print the installed Gatekey version")
	fmt.Println("  update                       Check, download, and stage the latest release")
	fmt.Println("  keys add <provider> <key>    Securely store an API key")
	fmt.Println("  keys list                    List all configured providers")
}
