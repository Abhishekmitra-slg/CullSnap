//go:build darwin

package icloud

import (
	"bytes"
	"context"
	"cullsnap/internal/cloudsource"
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Provider implements cloudsource.CloudSource for iCloud Photos on macOS.
// It communicates with Photos.app via osascript (AppleScript).
// iCloud Photos uses TCC (Transparency, Consent, and Control) for access — no
// token storage is needed.
type Provider struct{}

// New creates an iCloud Photos provider. The TokenStore parameter is accepted
// for interface compatibility but is not used — iCloud access is governed by
// macOS TCC permissions, not application-managed tokens.
func New(_ *cloudsource.TokenStore) *Provider {
	return &Provider{}
}

func (p *Provider) ID() string            { return "icloud" }
func (p *Provider) DisplayName() string   { return "iCloud Photos" }
func (p *Provider) IsAvailable() bool     { return true }
func (p *Provider) IsAuthenticated() bool { return true }

// Authenticate is a no-op for iCloud — Photos.app handles TCC permissions.
func (p *Provider) Authenticate(_ context.Context) error {
	logger.Log.Debug("icloud: authenticate is a no-op (TCC handles permissions)")
	return nil
}

// ListAlbums queries Photos.app for all user albums via osascript.
func (p *Provider) ListAlbums(ctx context.Context) ([]cloudsource.Album, error) {
	script := `
tell application "Photos"
	set albumList to {}
	repeat with a in albums
		set albumName to name of a
		set albumID to id of a
		set albumCount to count of media items of a
		set end of albumList to albumName & "|||" & albumID & "|||" & (albumCount as text)
	end repeat
	set AppleScript's text item delimiters to "###"
	return albumList as text
end tell`

	output, err := runOsascript(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("icloud: failed to list albums: %w", err)
	}

	albums, err := parseAlbumOutput(output)
	if err != nil {
		return nil, fmt.Errorf("icloud: failed to parse album output: %w", err)
	}

	logger.Log.Debug("icloud: listed albums", "count", len(albums))
	return albums, nil
}

// ListMediaInAlbum queries Photos.app for media items in a specific album.
func (p *Provider) ListMediaInAlbum(ctx context.Context, albumID string) ([]cloudsource.RemoteMedia, error) {
	script := fmt.Sprintf(`
tell application "Photos"
	set targetAlbum to album id %q
	set mediaList to {}
	repeat with m in media items of targetAlbum
		set mFilename to filename of m
		set mID to id of m
		set mDate to date of m as text
		set mSize to size of m
		set end of mediaList to mFilename & "|||" & mID & "|||" & (mDate as text) & "|||" & (mSize as text)
	end repeat
	set AppleScript's text item delimiters to "###"
	return mediaList as text
end tell`, albumID)

	output, err := runOsascript(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("icloud: failed to list media in album %s: %w", albumID, err)
	}

	media, err := parseMediaOutput(output)
	if err != nil {
		return nil, fmt.Errorf("icloud: failed to parse media output: %w", err)
	}

	logger.Log.Debug("icloud: listed media", "albumID", albumID, "count", len(media))
	return media, nil
}

// Download checks if the file already exists at localPath (placed there by
// ExportAlbum), and returns nil if so. If the file does not exist, it returns
// an error indicating that ExportAlbum should be called first.
func (p *Provider) Download(_ context.Context, media cloudsource.RemoteMedia, localPath string, _ func(int64, int64)) error {
	if _, err := os.Stat(localPath); err == nil {
		logger.Log.Debug("icloud: file already exists (from bulk export)", "path", localPath)
		return nil
	}
	return fmt.Errorf("icloud: file %q not found at %s — call ExportAlbum first", media.Filename, localPath)
}

// ExportAlbum performs a bulk export of all media items in an album to destDir
// using Photos.app's native export. This is the primary export mechanism for
// iCloud since Photos.app exports are inherently bulk operations.
func (p *Provider) ExportAlbum(ctx context.Context, albumID, destDir string) error {
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return fmt.Errorf("icloud: failed to create export dir: %w", err)
	}

	absDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("icloud: failed to resolve export dir: %w", err)
	}

	script := fmt.Sprintf(`
tell application "Photos"
	set targetAlbum to album id %q
	set targetItems to media items of targetAlbum
	if (count of targetItems) = 0 then
		return "0"
	end if
	export targetItems to POSIX file %q with using originals
	return (count of targetItems) as text
end tell`, albumID, absDir)

	output, err := runOsascript(ctx, script)
	if err != nil {
		return fmt.Errorf("icloud: export failed for album %s: %w", albumID, err)
	}

	count := strings.TrimSpace(output)
	logger.Log.Info("icloud: exported album", "albumID", albumID, "destDir", absDir, "count", count)
	return nil
}

func (p *Provider) Disconnect() error {
	logger.Log.Debug("icloud: disconnect is a no-op")
	return nil
}

// runOsascript executes an AppleScript via osascript with context support.
func runOsascript(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logger.Log.Debug("icloud: running osascript", "scriptLen", len(script))

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		logger.Log.Debug("icloud: osascript failed", "stderr", errMsg)

		// macOS TCC error -1743: app lacks Automation permission for Photos.app
		if strings.Contains(errMsg, "-1743") || strings.Contains(errMsg, "Not authorized") {
			return "", fmt.Errorf("CullSnap needs Automation permission to access Photos. " +
				"Open System Settings \u2192 Privacy & Security \u2192 Automation and enable Photos for CullSnap, then try again")
		}
		return "", fmt.Errorf("osascript: %s", errMsg)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// parseAlbumOutput parses the osascript output for album listing.
// Expected format: "name|||id|||count###name|||id|||count###..."
func parseAlbumOutput(output string) ([]cloudsource.Album, error) {
	if output == "" {
		return nil, nil
	}

	entries := strings.Split(output, "###")
	albums := make([]cloudsource.Album, 0, len(entries))

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "|||", 3)
		if len(parts) != 3 {
			logger.Log.Debug("icloud: skipping malformed album entry", "entry", entry)
			continue
		}

		count, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
		albums = append(albums, cloudsource.Album{
			ID:         strings.TrimSpace(parts[1]),
			Title:      strings.TrimSpace(parts[0]),
			MediaCount: count,
		})
	}

	return albums, nil
}

// parseMediaOutput parses the osascript output for media listing.
// Expected format: "filename|||id|||date|||size###..."
func parseMediaOutput(output string) ([]cloudsource.RemoteMedia, error) {
	if output == "" {
		return nil, nil
	}

	entries := strings.Split(output, "###")
	media := make([]cloudsource.RemoteMedia, 0, len(entries))

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "|||", 4)
		if len(parts) != 4 {
			logger.Log.Debug("icloud: skipping malformed media entry", "entry", entry)
			continue
		}

		filename := strings.TrimSpace(parts[0])
		id := strings.TrimSpace(parts[1])
		dateStr := strings.TrimSpace(parts[2])
		sizeStr := strings.TrimSpace(parts[3])

		sizeBytes, _ := strconv.ParseInt(sizeStr, 10, 64)

		// AppleScript dates come in locale-dependent format; best-effort parse
		createdAt := parseAppleScriptDate(dateStr)

		media = append(media, cloudsource.RemoteMedia{
			ID:        id,
			Filename:  filename,
			SizeBytes: sizeBytes,
			CreatedAt: createdAt,
		})
	}

	return media, nil
}

// parseAppleScriptDate attempts to parse AppleScript date strings.
// AppleScript dates are locale-dependent, so we try multiple formats.
func parseAppleScriptDate(s string) time.Time {
	formats := []string{
		"Monday, January 2, 2006 at 3:04:05 PM",
		"January 2, 2006 at 3:04:05 PM",
		"1/2/2006 3:04:05 PM",
		"2006-01-02 15:04:05",
		"Monday, 2 January 2006 at 15:04:05",
		"2 January 2006 at 15:04:05",
		time.RFC3339,
	}

	for _, fmt := range formats {
		if t, err := time.Parse(fmt, s); err == nil {
			return t
		}
	}

	logger.Log.Debug("icloud: could not parse date", "dateStr", s)
	return time.Time{}
}
