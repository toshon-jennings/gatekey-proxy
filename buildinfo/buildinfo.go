package buildinfo

import "runtime/debug"

// Version is replaced with the release tag at build time.
var Version = "dev"

const Repository = "toshon-jennings/gatekey-proxy"

func Current() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}
