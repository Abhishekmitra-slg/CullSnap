package runtime

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnsurePython runs `uv python install <version>` (idempotent).
func (p *Provisioner) EnsurePython(ctx context.Context, version string, progressFn func(line string)) (string, error) {
	if logger.Log != nil {
		logger.Log.Debug("vlm/runtime: ensuring python", "version", version)
	}
	cmd := exec.CommandContext(ctx, p.UVPath(), "python", "install", version) // #nosec G204 -- UVPath is provisioned by CullSnap, not user input // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Env = sanitizedEnv("", map[string]string{"PATH": os.Getenv("PATH"), "HOME": os.Getenv("HOME")})
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := string(out)
		if len(tail) > 4096 {
			tail = tail[len(tail)-4096:]
		}
		return "", fmt.Errorf("vlm/runtime: uv python install %s failed: %w (tail: %s)", version, err, tail)
	}
	if progressFn != nil {
		for _, ln := range strings.Split(string(out), "\n") {
			progressFn(ln)
		}
	}
	return version, nil
}

// EnsureVenv creates ~/.cullsnap/mlx-venv-<engine>/ via uv. Idempotent if the
// venv already exists with a working python3 inside.
func (p *Provisioner) EnsureVenv(ctx context.Context, engine, pythonVersion string) (string, error) {
	if logger.Log != nil {
		logger.Log.Debug("vlm/runtime: ensuring venv", "engine", engine, "version", pythonVersion)
	}
	venvDir := filepath.Join(p.cullsnapDir, "mlx-venv-"+engine)
	python3 := filepath.Join(venvDir, "bin", "python3")
	if _, err := os.Stat(python3); err == nil {
		return venvDir, nil
	}
	cmd := exec.CommandContext(ctx, p.UVPath(), "venv", venvDir, "--python", pythonVersion) // #nosec G204 -- UVPath is provisioned by CullSnap, not user input // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Env = sanitizedEnv("", map[string]string{"PATH": os.Getenv("PATH"), "HOME": os.Getenv("HOME")})
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := string(out)
		if len(tail) > 4096 {
			tail = tail[len(tail)-4096:]
		}
		return "", fmt.Errorf("vlm/runtime: uv venv failed: %w (tail: %s)", err, tail)
	}
	return venvDir, nil
}
