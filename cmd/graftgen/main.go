package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/calumari/graft/internal/generator"
)

// deriveVersion inspects build info for module version or vcs revision.
// preference order: module semantic version -> short commit hash -> "devel".
func deriveVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
		var revision string
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				revision = s.Value
				break
			}
		}
		if len(revision) >= 12 { // short hash for readability
			return revision[:12]
		}
		if revision != "" {
			return revision
		}
	}
	return "devel"
}

func main() {
	var interfacesCSV string
	var output string
	var dir string
	var debug bool
	var customFuncsCSV string
	flag.StringVar(&interfacesCSV, "interface", "", "Comma-separated list of mapper interface names to implement (required)")
	flag.StringVar(&output, "output", "graft_gen.go", "Output filename for generated code")
	flag.StringVar(&dir, "dir", ".", "Directory to scan for interface definitions (relative to current directory)")
	flag.BoolVar(&debug, "debug", false, "Emit debug comments linking generated code to template nodes")
	flag.StringVar(&customFuncsCSV, "custom_funcs", "", "Comma-separated list of custom mapping function names")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nGraftgen generates type-safe struct mappers from interface definitions.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -interface=UserMapper,ProductMapper -output=mappers_gen.go\n", os.Args[0])
	}
	flag.Parse()

	if interfacesCSV == "" {
		fmt.Fprintf(os.Stderr, "Error: -interface is required\n\n")
		flag.Usage()
		os.Exit(1)
	}
	interfaces := strings.Split(interfacesCSV, ",")
	for i := range interfaces {
		interfaces[i] = strings.TrimSpace(interfaces[i])
	}
	var customFuncs []string
	if customFuncsCSV != "" {
		parts := strings.SplitSeq(customFuncsCSV, ",")
		for p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				customFuncs = append(customFuncs, p)
			}
		}
	}

	// build a simplified canonical command representation instead of raw argv (which may include build cache paths)
	cmdParts := []string{"graftgen", "-interface=" + strings.Join(interfaces, ","), "-output=" + output}
	if dir != "." {
		cmdParts = append(cmdParts, "-dir="+dir)
	}
	if debug {
		cmdParts = append(cmdParts, "-debug")
	}
	if len(customFuncs) > 0 {
		cmdParts = append(cmdParts, "-custom_funcs="+strings.Join(customFuncs, ","))
	}
	displayCmd := strings.Join(cmdParts, " ")
	buildVersion := deriveVersion()
	cfg := generator.Config{Dir: dir, Interfaces: interfaces, Output: output, Debug: debug, CustomFuncs: customFuncs, Command: displayCmd, Version: buildVersion}
	if err := generator.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "graft: %v\n", err)
		os.Exit(1)
	}
}
