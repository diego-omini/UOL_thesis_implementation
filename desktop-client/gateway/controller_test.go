package gateway

// This file verifies Client Portal Gateway installation directory validation.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateDirectory(t *testing.T) {
	directory := t.TempDir()
	launcher := "run.sh"
	if runtime.GOOS == "windows" {
		launcher = "run.bat"
	}

	if err := os.MkdirAll(filepath.Join(directory, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "root"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "bin", launcher), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "root", "conf.yaml"), nil, 0600); err != nil {
		t.Fatal(err)
	}

	if err := ValidateDirectory(directory); err != nil {
		t.Fatalf("ValidateDirectory() returned an error: %v", err)
	}
}

func TestValidateDirectoryRejectsIncompleteInstallation(t *testing.T) {
	if err := ValidateDirectory(t.TempDir()); err == nil {
		t.Fatal("expected an incomplete gateway directory to be rejected")
	}
}
