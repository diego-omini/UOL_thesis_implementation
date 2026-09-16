//go:build !windows

package gateway

// This file creates a detached Unix command for the Client Portal Gateway shell launcher.

import (
	"os/exec"
	"path/filepath"
)

func gatewayCommand(directory string) (*exec.Cmd, error) {
	command := exec.Command(
		filepath.Join(directory, "bin", "run.sh"),
		filepath.Join(directory, "root", "conf.yaml"),
	)
	command.Dir = directory
	return command, nil
}
