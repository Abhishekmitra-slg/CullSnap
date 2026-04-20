package runtime

// sanitizedEnv returns the env vars to pass to a uv/python subprocess.
// Strips PYTHONPATH, PIP_*, NIX_*, CONDA_*, and any user-provided LANG.
// Inherits only PATH and HOME from input.
// When venvPath is empty (e.g. during EnsurePython / EnsureVenv before a venv
// exists) the venv-bin prefix is omitted so PATH is not corrupted with a
// spurious "/bin:" entry.
func sanitizedEnv(venvPath string, inherit map[string]string) []string {
	pathEntry := "PATH=" + inherit["PATH"]
	if venvPath != "" {
		pathEntry = "PATH=" + venvPath + "/bin:" + inherit["PATH"]
	}
	out := []string{
		"VIRTUAL_ENV=" + venvPath,
		pathEntry,
		"HOME=" + inherit["HOME"],
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"UV_NO_PROGRESS=1",
		"PYTHONUNBUFFERED=1",
	}
	return out
}
