// Package gateway starts and validates a locally installed IBKR Client Portal Gateway.
package gateway

// This file coordinates gateway directory validation and prevents duplicate process starts.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Controller owns startup state for a gateway process launched by this application.
type Controller struct {
	mu        sync.Mutex
	directory string
	running   bool
}

// NewController creates a gateway controller for the configured installation directory.
func NewController(directory string) *Controller {
	return &Controller{directory: directory}
}

// Directory returns the currently configured gateway installation directory.
func (c *Controller) Directory() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.directory
}

// SetDirectory replaces the gateway installation directory after validating its files.
func (c *Controller) SetDirectory(directory string) error {
	if err := ValidateDirectory(directory); err != nil {
		return err
	}

	c.mu.Lock()
	c.directory = filepath.Clean(directory)
	c.mu.Unlock()
	return nil
}

// Start launches the configured gateway unless a process started by this controller is still running.
func (c *Controller) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}
	if err := ValidateDirectory(c.directory); err != nil {
		return err
	}

	cmd, err := gatewayCommand(c.directory)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Client Portal Gateway: %w", err)
	}

	c.running = true
	go func() {
		_ = cmd.Wait()
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()
	return nil
}

// ValidateDirectory confirms that the selected directory contains the platform launcher and gateway configuration.
func ValidateDirectory(directory string) error {
	if directory == "" {
		return fmt.Errorf("select the extracted clientportal.gw directory")
	}

	launcher := "run.sh"
	if runtime.GOOS == "windows" {
		launcher = "run.bat"
	}

	required := []string{
		filepath.Join(directory, "bin", launcher),
		filepath.Join(directory, "root", "conf.yaml"),
	}
	for _, path := range required {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("gateway file %s is unavailable: %w", path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("gateway path %s is not a file", path)
		}
	}

	return nil
}
