package cli

import (
	"fmt"
	"os"

	"github.com/toshonjennings/ai-proxy/config"
	"github.com/toshonjennings/ai-proxy/server"
)

func Run() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "start":
		srv := server.NewProxyServer("8181")
		if err := srv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
			os.Exit(1)
		}
	case "keys":
		if len(os.Args) < 3 {
			fmt.Println("Usage: ai-proxy keys [add|list]")
			os.Exit(1)
		}
		subcmd := os.Args[2]
		switch subcmd {
		case "add":
			if len(os.Args) != 5 {
				fmt.Println("Usage: ai-proxy keys add <provider> <key>")
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

func printUsage() {
	fmt.Println("AI Proxy CLI")
	fmt.Println("Usage:")
	fmt.Println("  start                        Start the AI Proxy server on port 8181")
	fmt.Println("  keys add <provider> <key>    Securely store an API key")
	fmt.Println("  keys list                    List all configured providers")
}
