package main

import (
	"fmt"
	"os"

	"github.com/walnuthq/dpm-trace/internal/plugin"
)

// runInstallPlugin registers this binary as a DPM component so it runs as
// `dpm trace`. Ports install_plugin_main.
func runInstallPlugin(args []string) int {
	opts := plugin.Options{ComponentVersion: version}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if i+1 >= len(args) {
			fmt.Fprintf(os.Stderr, "error: %s requires a value\n", arg)
			return 2
		}
		i++
		switch arg {
		case "--dpm-home":
			opts.DPMHome = args[i]
		case "--sdk-version":
			opts.SDKVersion = args[i]
		case "--component-version":
			opts.ComponentVersion = args[i]
		default:
			fmt.Fprintf(os.Stderr, "error: unknown flag %q\n", arg)
			return 2
		}
	}
	if opts.ComponentVersion == "dev" {
		// A dev build has no release version; match the Python fallback rather
		// than registering a component version DPM cannot resolve.
		opts.ComponentVersion = "0.1.0"
	}
	if err := plugin.Install(os.Stdout, opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	return 0
}
