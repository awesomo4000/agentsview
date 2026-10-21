package main

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"go.kenn.io/agentsview/internal/parser"
)

func restoreTestLogger(t *testing.T) {
	t.Helper()
	oldWriter := log.Writer()
	t.Cleanup(func() {
		if file, ok := log.Writer().(*os.File); ok && file != os.Stderr && file != os.Stdout {
			_ = file.Close()
		}
		log.SetOutput(oldWriter)
	})
}

func clearConfiguredAgentEnvVars(t *testing.T) {
	t.Helper()
	for _, def := range parser.Registry {
		for _, name := range []string{def.EnvVar, def.NativeEnvVar, def.DefaultRootEnvVar} {
			if name != "" {
				t.Setenv(name, "")
			}
		}
	}
}

func isolateDefaultAgentDirs(t *testing.T, root string) {
	t.Helper()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("APPDATA", root)
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("HOMEDRIVE", filepath.VolumeName(root))
	t.Setenv("HOMEPATH", `\`)
}
