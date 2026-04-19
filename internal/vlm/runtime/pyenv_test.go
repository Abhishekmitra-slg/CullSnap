package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsurePythonInvokesUV(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required for fake-uv shim")
	}
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
	if _, err := p.EnsurePython(context.Background(), "3.12.7", nil); err != nil {
		t.Fatalf("EnsurePython: %v", err)
	}
	log, _ := os.ReadFile(filepath.Join(dir, "uv-args.log"))
	if !strings.Contains(string(log), "python install 3.12.7") {
		t.Fatalf("uv not invoked correctly: %s", log)
	}
}

func TestEnsureVenvInvokesUV(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	_ = os.MkdirAll(bin, 0o755)
	fake := filepath.Join(bin, "uv")
	script := `#!/usr/bin/env bash
echo "$@" >> "` + dir + `/uv-args.log"
mkdir -p "` + dir + `/mlx-venv-mlx_vlm/bin"
touch "` + dir + `/mlx-venv-mlx_vlm/bin/python3"
chmod +x "` + dir + `/mlx-venv-mlx_vlm/bin/python3"
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Provisioner{cullsnapDir: dir}
	venv, err := p.EnsureVenv(context.Background(), "mlx_vlm", "3.12.7")
	if err != nil {
		t.Fatalf("EnsureVenv: %v", err)
	}
	if filepath.Base(venv) != "mlx-venv-mlx_vlm" {
		t.Fatalf("venv: %s", venv)
	}
}
