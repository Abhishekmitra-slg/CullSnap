//go:build windows

package device

import (
	"bytes"
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"os/exec"
)

// powershellExe is the canonical path to Windows PowerShell v1.0.
// We use the full path to avoid PATH hijacking.
const powershellExe = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`

// powershellExePath verifies that the PowerShell binary exists and is a regular
// file. Returns the verified path or an error.
func powershellExePath() (string, error) {
	info, err := os.Stat(powershellExe)
	if err != nil {
		return "", fmt.Errorf("powershell: binary not found at %s: %w", powershellExe, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("powershell: path is a directory, not an executable: %s", powershellExe)
	}
	return powershellExe, nil
}

// runPowerShell executes a PowerShell script with hardened flags and no stdin.
// The script is encoded as base64 UTF-16LE to avoid quoting issues.
// Returns combined stdout output. Stderr is captured separately and logged on
// error to aid debugging.
func runPowerShell(ctx context.Context, script string) ([]byte, error) {
	if _, err := powershellExePath(); err != nil {
		return nil, err
	}

	encoded := encodePowerShellScript(script)
	logger.Log.Debug("runPowerShell: launching",
		"exe", powershellExe,
		"encodedLen", len(encoded),
	)

	stdout, stderr, err := execPowerShell(ctx, encoded, nil)
	if err != nil {
		logger.Log.Error("runPowerShell: execution failed",
			"err", err,
			"stderr", string(stderr),
		)
		if len(stderr) > 0 {
			return nil, fmt.Errorf("powershell: %w; stderr: %s", err, bytes.TrimSpace(stderr))
		}
		return nil, fmt.Errorf("powershell: %w", err)
	}

	if len(stderr) > 0 {
		logger.Log.Debug("runPowerShell: script produced stderr output",
			"stderr", string(stderr),
		)
	}

	return stdout, nil
}

// runPowerShellWithStdin executes a PowerShell script with hardened flags,
// piping stdinData into the process's stdin. This allows passing structured
// JSON parameters to scripts without putting them on the command line.
// Returns separate stdout and stderr buffers so callers can parse progress
// lines from stdout while preserving error detail from stderr.
func runPowerShellWithStdin(ctx context.Context, script string, stdinData []byte) (stdout, stderr []byte, err error) {
	if _, err2 := powershellExePath(); err2 != nil {
		return nil, nil, err2
	}

	encoded := encodePowerShellScript(script)
	logger.Log.Debug("runPowerShellWithStdin: launching",
		"exe", powershellExe,
		"encodedLen", len(encoded),
		"stdinLen", len(stdinData),
	)

	stdout, stderr, err = execPowerShell(ctx, encoded, stdinData)
	if err != nil {
		logger.Log.Error("runPowerShellWithStdin: execution failed",
			"err", err,
			"stderr", string(bytes.TrimSpace(stderr)),
		)
		return stdout, stderr, fmt.Errorf("powershell: %w", err)
	}

	if len(stderr) > 0 {
		logger.Log.Debug("runPowerShellWithStdin: script produced stderr output",
			"stderr", string(stderr),
		)
	}

	return stdout, stderr, nil
}

// execPowerShell is the low-level helper that assembles the hardened command
// line and runs the process. It is separated from the public functions to keep
// them focused on logging and error wrapping.
//
// Security notes:
//   - The executable path is always the package-level constant powershellExe;
//     it is never derived from user input, so there is no PATH-hijacking risk.
//   - encodedScript is the output of encodePowerShellScript, which produces
//     standard base64 (alphabet A-Z a-z 0-9 + / =). It cannot carry shell
//     metacharacters. exec.CommandContext passes each element as a separate
//     argv entry — the OS/runtime never re-parses them through a shell.
func execPowerShell(ctx context.Context, encodedScript string, stdinData []byte) (stdout, stderr []byte, err error) {
	// Use the hard-coded constant directly so static analysis can confirm the
	// executable path is not influenced by any variable data.
	args := []string{
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-EncodedCommand", encodedScript, // base64-only; see security note above
	}

	// #nosec G204 -- exe is the package-level constant powershellExe; args
	// contain only static flags plus a base64-encoded script (no shell metacharacters).
	cmd := exec.CommandContext(ctx, powershellExe, args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if len(stdinData) > 0 {
		cmd.Stdin = bytes.NewReader(stdinData)
	}

	if err = cmd.Run(); err != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
	}

	return stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
}
