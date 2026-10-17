package agentsview_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareSidecarBuildsWithoutPricingDownload(t *testing.T) {
	requireUnixShell(t)

	root := t.TempDir()
	installRepoFile(t, root, "desktop/scripts/prepare-sidecar.sh", 0o755)
	writeDesktopDevWorkspace(t, root)
	stubs := installUnixBuildStubs(t, root)

	out, err := runInWorkspace(
		t,
		root,
		stubs.env("AGENTSVIEW_VERSION=v1.2.3"),
		"bash",
		"desktop/scripts/prepare-sidecar.sh",
	)
	require.NoError(t, err, "%s", out)
	assertNoEventContains(t, stubs.events(t), "go run")

	assertEventOrder(t, stubs.events(t),
		"npm run build",
		"go build -tags fts5",
	)
	require.FileExists(t, filepath.Join(
		root,
		"desktop",
		"src-tauri",
		"binaries",
		"agentsview-x86_64-unknown-linux-gnu",
	))
}
