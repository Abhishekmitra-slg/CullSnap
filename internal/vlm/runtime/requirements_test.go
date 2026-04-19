package runtime

import (
	"context"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallRequirementsInvokesUV(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	_ = os.MkdirAll(bin, 0o755)
	fake := filepath.Join(bin, "uv")
	script := `#!/usr/bin/env bash
echo "$@" >> "` + dir + `/uv-args.log"
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Provisioner{cullsnapDir: dir}
	if err := p.InstallRequirements(context.Background(), filepath.Join(dir, "venv"), "mlx_vlm", nil); err != nil {
		t.Fatalf("InstallRequirements: %v", err)
	}
	log, _ := os.ReadFile(filepath.Join(dir, "uv-args.log"))
	if !strings.Contains(string(log), "pip install --require-hashes") {
		t.Fatalf("uv args: %s", log)
	}
}

// embedFS is intentionally referenced to ensure go:embed compiles.
var _ embed.FS
