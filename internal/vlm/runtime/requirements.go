package runtime

import (
	"context"
	"cullsnap/internal/logger"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed reqs/requirements_mlx_vlm.txt reqs/requirements_vllm_mlx.txt
var embeddedReqs embed.FS

func reqsForEngine(engine string) (string, error) {
	switch engine {
	case "mlx_vlm":
		return "reqs/requirements_mlx_vlm.txt", nil
	case "vllm_mlx":
		return "reqs/requirements_vllm_mlx.txt", nil
	}
	return "", fmt.Errorf("vlm/runtime: unknown engine %q", engine)
}

// InstallRequirements writes the embedded reqs file to a temp path and runs
// `uv pip install --require-hashes -r <reqs> --python <venv>/bin/python`.
// progressFn is accepted for API compatibility with future streaming; it is not
// called in this implementation (CombinedOutput blocks until completion).
func (p *Provisioner) InstallRequirements(ctx context.Context, venvPath, engine string, progressFn func(line string)) error {
	if logger.Log != nil {
		logger.Log.Debug("vlm/runtime: installing requirements", "engine", engine, "venv", venvPath)
	}
	name, err := reqsForEngine(engine)
	if err != nil {
		return err
	}
	data, err := embeddedReqs.ReadFile(name)
	if err != nil {
		return fmt.Errorf("vlm/runtime: read embedded reqs: %w", err)
	}
	tmp, err := os.CreateTemp("", "cullsnap-reqs-*.txt")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	python := filepath.Join(venvPath, "bin", "python")
	cmd := exec.CommandContext(ctx, p.UVPath(), // #nosec G204 -- UVPath is provisioned by CullSnap, not user input // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		"pip", "install", "--require-hashes",
		"--python", python,
		"-r", tmp.Name(),
	)
	cmd.Env = sanitizedEnv(venvPath, map[string]string{"PATH": os.Getenv("PATH"), "HOME": os.Getenv("HOME")})
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := string(out)
		if len(tail) > 4096 {
			tail = tail[len(tail)-4096:]
		}
		return fmt.Errorf("vlm/runtime: uv pip install failed: %w (tail: %s)", err, tail)
	}
	return nil
}
