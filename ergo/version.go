package main

import (
	"runtime/debug"

	"ergo.services/ergo/gen"
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				Version.Commit = setting.Value
				break
			}
		}

		if Version.Commit == "" {
			Version.Commit = info.Main.Sum
		}
	}
}

var (
	Version = gen.Version{
		Name:    "ergo: a cli-tool for boilerplate code generation for Ergo Framework",
		Release: "0.1.0",
		License: gen.LicenseMIT,
	}
)
