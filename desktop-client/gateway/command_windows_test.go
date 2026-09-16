//go:build windows

package gateway

// This file verifies that the Windows launcher supports gateway directories containing spaces.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatewayCommandSupportsSpacesInPath(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "gateway with spaces")
	if err := os.MkdirAll(filepath.Join(directory, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "root"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "bin", "run.bat"), []byte("@echo off\r\necho %~1\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "root", "conf.yaml")
	if err := os.WriteFile(configPath, nil, 0600); err != nil {
		t.Fatal(err)
	}

	command, err := gatewayCommand(directory)
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.Output()
	if err != nil {
		t.Fatalf("gateway command failed: %v", err)
	}
	if strings.TrimSpace(string(output)) != configPath {
		t.Fatalf("gateway config argument = %q, want %q", strings.TrimSpace(string(output)), configPath)
	}
}
