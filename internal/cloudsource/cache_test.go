package cloudsource

import (
	"testing"
)

func TestNewCacheManager(t *testing.T) {
	cm := NewCacheManager("/tmp/test", nil, 512)
	if cm.baseDir != "/tmp/test" {
		t.Errorf("baseDir = %q, want /tmp/test", cm.baseDir)
	}
	if cm.maxCacheMB != 512 {
		t.Errorf("maxCacheMB = %d, want 512", cm.maxCacheMB)
	}
}

func TestSetMaxCacheMB(t *testing.T) {
	cm := NewCacheManager("/tmp/test", nil, 512)
	cm.SetMaxCacheMB(1024)
	if cm.maxCacheMB != 1024 {
		t.Errorf("maxCacheMB = %d, want 1024", cm.maxCacheMB)
	}
}
