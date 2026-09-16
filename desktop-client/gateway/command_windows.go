//go:build windows

package gateway

// This file creates a hidden Windows command for the Client Portal Gateway batch launcher.

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
)

func gatewayCommand(directory string) (*exec.Cmd, error) {
	launcher := filepath.Join(directory, "bin", "run.bat")
	configuration := filepath.Join(directory, "root", "conf.yaml")
	command := exec.Command("cmd.exe")
	command.Dir = directory
	command.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       fmt.Sprintf(`cmd.exe /D /S /C ""%s" "%s""`, launcher, configuration),
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	return command, nil
}
