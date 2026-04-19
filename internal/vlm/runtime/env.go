package runtime

// sanitizedEnv returns the env vars to pass to a uv/python subprocess.
// Strips PYTHONPATH, PIP_*, NIX_*, CONDA_*, and any user-provided LANG.
// Inherits only PATH and HOME from input.
func sanitizedEnv(venvPath string, inherit map[string]string) []string {
	out := []string{
		"VIRTUAL_ENV=" + venvPath,
		"PATH=" + venvPath + "/bin:" + inherit["PATH"],
		"HOME=" + inherit["HOME"],
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"UV_NO_PROGRESS=1",
		"PYTHONUNBUFFERED=1",
	}
	return out
}
