package runtime

import "testing"

func TestUVDownloadInfo(t *testing.T) {
	cases := []struct{ os, arch string }{
		{"darwin", "arm64"}, {"darwin", "amd64"}, {"linux", "amd64"},
	}
	for _, c := range cases {
		info, ok := uvDownloadInfoFor(c.os, c.arch)
		if !ok {
			t.Fatalf("no uv info for %s/%s", c.os, c.arch)
		}
		if info.URL == "" || len(info.SHA256) != 64 {
			t.Fatalf("incomplete info for %s/%s: %+v", c.os, c.arch, info)
		}
	}
}
