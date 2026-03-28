//go:build darwin

package app

import (
	"cullsnap/internal/logger"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ensureBundleSigned verifies the app bundle signature and repairs it if needed.
// The auto-updater replaces the binary without re-signing the bundle, which
// breaks TCC attribution (macOS can't identify the app for Automation
// permissions). This function detects that state and re-signs the bundle.
func ensureBundleSigned() {
	bundlePath := findAppBundle()
	if bundlePath == "" {
		logger.Log.Debug("codesign: not running from an app bundle, skipping")
		return
	}

	plistPath := filepath.Join(bundlePath, "Contents", "Info.plist")
	ensureAppleEventsUsageDescription(plistPath)

	if err := exec.Command("codesign", "--verify", "--deep", "--strict", bundlePath).Run(); err == nil {
		logger.Log.Debug("codesign: bundle signature is valid")
		return
	}

	logger.Log.Info("codesign: bundle signature is invalid, re-signing", "bundle", bundlePath)

	out, err := exec.Command("codesign", "--force", "--deep", "--sign", "-", bundlePath).CombinedOutput()
	if err != nil {
		logger.Log.Warn("codesign: failed to re-sign bundle", "error", err, "output", string(out))
		return
	}

	logger.Log.Info("codesign: bundle re-signed successfully")
}

// ensureAppleEventsUsageDescription adds NSAppleEventsUsageDescription to
// Info.plist if missing. This key is required for macOS to show the Automation
// permission dialog when the app sends Apple Events to Photos.app.
func ensureAppleEventsUsageDescription(plistPath string) {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		logger.Log.Debug("codesign: could not read Info.plist", "error", err)
		return
	}

	if strings.Contains(string(data), "NSAppleEventsUsageDescription") {
		return
	}

	logger.Log.Info("codesign: adding NSAppleEventsUsageDescription to Info.plist")

	description := "CullSnap needs to communicate with Photos to browse and export your iCloud photo albums."
	out, err := exec.Command(
		"/usr/libexec/PlistBuddy",
		"-c", "Add :NSAppleEventsUsageDescription string "+description,
		plistPath,
	).CombinedOutput()
	if err != nil {
		logger.Log.Warn("codesign: failed to update Info.plist", "error", err, "output", string(out))
	}
}

// findAppBundle locates the .app bundle path from the running executable.
func findAppBundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return ""
	}

	// Walk up from .../CullSnap.app/Contents/MacOS/CullSnap
	dir := exe
	for range 4 {
		dir = filepath.Dir(dir)
		if strings.HasSuffix(dir, ".app") {
			return dir
		}
	}
	return ""
}
