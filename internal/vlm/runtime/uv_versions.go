package runtime

import "fmt"

// UVVersion is the pinned release of uv. Bump along with verified SHA-256s below.
const UVVersion = "0.4.30"

// UVDownloadInfo is the per-platform binary descriptor.
type UVDownloadInfo struct {
	URL    string
	SHA256 string // hex SHA-256 of the tarball
}

// uvDownloads pins uv release tarballs per platform. SHA-256s are cross-verified
// against the upstream .sha256 asset published alongside each tarball at
// https://github.com/astral-sh/uv/releases/tag/<UVVersion>. When bumping
// UVVersion, fetch the new .sha256 files (curl -sL <url>.sha256) and replace
// every entry's SHA256 field.
var uvDownloads = map[string]UVDownloadInfo{
	"darwin/arm64": {
		URL:    "https://github.com/astral-sh/uv/releases/download/" + UVVersion + "/uv-aarch64-apple-darwin.tar.gz",
		SHA256: "5fb068be1d0c77d0829a0a611a470f74318f114c4dc8671cfaf1e606ab81e40a",
	},
	"darwin/amd64": {
		URL:    "https://github.com/astral-sh/uv/releases/download/" + UVVersion + "/uv-x86_64-apple-darwin.tar.gz",
		SHA256: "a56b550c08e3315bfa450c134410bbe91318ae2f39a0ce2649b882a76cd9b601",
	},
	"linux/amd64": {
		URL:    "https://github.com/astral-sh/uv/releases/download/" + UVVersion + "/uv-x86_64-unknown-linux-gnu.tar.gz",
		SHA256: "5637be5d163ccdcb9f0fe625890d84bbc3596810320be1f56b53bb111edb5dd7",
	},
}

func uvDownloadInfoFor(goos, goarch string) (UVDownloadInfo, bool) {
	info, ok := uvDownloads[fmt.Sprintf("%s/%s", goos, goarch)]
	return info, ok
}
