//go:build linux

package device

import (
	"strings"
	"testing"
)

func TestParseOSRelease_Ubuntu(t *testing.T) {
	content := `NAME="Ubuntu"
VERSION="24.04 LTS (Noble Numbat)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 24.04 LTS"
VERSION_ID="24.04"
`
	info := parseOSRelease([]byte(content))
	if info.ID != "ubuntu" {
		t.Errorf("ID = %q, want %q", info.ID, "ubuntu")
	}
	if info.IDLike != "debian" {
		t.Errorf("IDLike = %q, want %q", info.IDLike, "debian")
	}
	if info.PrettyName != "Ubuntu 24.04 LTS" {
		t.Errorf("PrettyName = %q, want %q", info.PrettyName, "Ubuntu 24.04 LTS")
	}
}

func TestParseOSRelease_Fedora(t *testing.T) {
	content := `NAME="Fedora Linux"
VERSION="41 (Workstation Edition)"
ID=fedora
PRETTY_NAME="Fedora Linux 41 (Workstation Edition)"
`
	info := parseOSRelease([]byte(content))
	if info.ID != "fedora" {
		t.Errorf("ID = %q, want %q", info.ID, "fedora")
	}
}

func TestParseOSRelease_Arch(t *testing.T) {
	content := `NAME="Arch Linux"
ID=arch
PRETTY_NAME="Arch Linux"
`
	info := parseOSRelease([]byte(content))
	if info.ID != "arch" {
		t.Errorf("ID = %q, want %q", info.ID, "arch")
	}
}

func TestParseOSRelease_OpenSUSE(t *testing.T) {
	content := `NAME="openSUSE Tumbleweed"
ID="opensuse-tumbleweed"
ID_LIKE="opensuse suse"
PRETTY_NAME="openSUSE Tumbleweed"
`
	info := parseOSRelease([]byte(content))
	if info.ID != "opensuse-tumbleweed" {
		t.Errorf("ID = %q, want %q", info.ID, "opensuse-tumbleweed")
	}
	if info.IDLike != "opensuse suse" {
		t.Errorf("IDLike = %q, want %q", info.IDLike, "opensuse suse")
	}
}

func TestParseOSRelease_Empty(t *testing.T) {
	info := parseOSRelease([]byte(""))
	if info.ID != "" {
		t.Errorf("ID = %q, want empty", info.ID)
	}
}

func TestDetectDistroFamily(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		idLike     string
		wantFamily string
	}{
		{"ubuntu", "ubuntu", "debian", "debian"},
		{"debian direct", "debian", "", "debian"},
		{"linuxmint", "linuxmint", "ubuntu debian", "debian"},
		{"pop os", "pop", "ubuntu debian", "debian"},
		{"fedora", "fedora", "", "fedora"},
		{"centos", "centos", "rhel fedora", "fedora"},
		{"rocky", "rocky", "rhel centos fedora", "fedora"},
		{"alma", "almalinux", "rhel centos fedora", "fedora"},
		{"arch", "arch", "", "arch"},
		{"manjaro", "manjaro", "arch", "arch"},
		{"endeavouros", "endeavouros", "arch", "arch"},
		{"opensuse tw", "opensuse-tumbleweed", "opensuse suse", "suse"},
		{"opensuse leap", "opensuse-leap", "suse opensuse", "suse"},
		{"suse direct", "sles", "suse", "suse"},
		{"unknown", "gentoo", "linux", "unknown"},
		{"empty", "", "", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectDistroFamily(tt.id, tt.idLike)
			if got != tt.wantFamily {
				t.Errorf("detectDistroFamily(%q, %q) = %q, want %q", tt.id, tt.idLike, got, tt.wantFamily)
			}
		})
	}
}

func TestInstallCommandForFamily(t *testing.T) {
	tests := []struct {
		family      string
		wantContain string
	}{
		{"debian", "apt install"},
		{"fedora", "dnf install"},
		{"arch", "pacman -S"},
		{"suse", "zypper install"},
	}
	for _, tt := range tests {
		t.Run(tt.family, func(t *testing.T) {
			got := installCommandForFamily(tt.family)
			if got == "" {
				t.Fatalf("expected non-empty for %q", tt.family)
			}
			if !strings.Contains(got, tt.wantContain) {
				t.Errorf("expected %q to contain %q", got, tt.wantContain)
			}
			if !strings.Contains(got, "gphoto2") {
				t.Errorf("expected %q to contain 'gphoto2'", got)
			}
		})
	}

	// Unknown should return empty
	if got := installCommandForFamily("unknown"); got != "" {
		t.Errorf("expected empty for unknown, got %q", got)
	}
}
