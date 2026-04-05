//go:build !windows

package scoring

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOnnxRuntimeLibName(t *testing.T) {
	name := onnxRuntimeLibName()
	if runtime.GOOS == "darwin" {
		if name != "libonnxruntime.dylib" {
			t.Errorf("expected libonnxruntime.dylib on darwin, got %s", name)
		}
	} else {
		if name != "libonnxruntime.so" {
			t.Errorf("expected libonnxruntime.so on linux, got %s", name)
		}
	}
}

func TestOnnxRuntimeDownloadURL(t *testing.T) {
	url, err := onnxRuntimeDownloadURL()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" {
		t.Fatal("URL should not be empty")
	}
	if len(url) < 50 {
		t.Errorf("URL seems too short: %s", url)
	}
}

func TestProvisionONNXRuntime_AlreadyExists(t *testing.T) {
	tmp := t.TempDir()
	libDir := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}

	libName := onnxRuntimeLibName()
	libPath := filepath.Join(libDir, libName)
	if err := os.WriteFile(libPath, []byte("fake lib"), 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := ProvisionONNXRuntime(context.Background(), tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != libPath {
		t.Errorf("expected %s, got %s", libPath, path)
	}
}

func TestExtractLibFromTgz(t *testing.T) {
	libName := onnxRuntimeLibName()
	libContent := []byte("fake onnxruntime library content for testing")

	// Create a .tgz with the expected structure.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	header := &tar.Header{
		Name: "onnxruntime-test-1.23.0/lib/" + libName,
		Mode: 0o755,
		Size: int64(len(libContent)),
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(libContent); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gw.Close()

	destDir := t.TempDir()
	if err := extractLibFromTgz(&buf, destDir, libName); err != nil {
		t.Fatalf("extractLibFromTgz failed: %v", err)
	}

	extracted, err := os.ReadFile(filepath.Join(destDir, libName))
	if err != nil {
		t.Fatalf("extracted file not found: %v", err)
	}
	if !bytes.Equal(extracted, libContent) {
		t.Error("extracted content doesn't match")
	}
}

func TestExtractLibFromTgz_NotFound(t *testing.T) {
	// Empty archive.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	tw.Close()
	gw.Close()

	destDir := t.TempDir()
	err := extractLibFromTgz(&buf, destDir, "libonnxruntime.dylib")
	if err == nil {
		t.Error("should fail when library not found in archive")
	}
}
