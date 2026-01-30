package selfupdate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/kardianos/osext"
)

// UpdateWithRestart downloads and applies update, then restarts the process
func (u *Updater) UpdateWithRestart() error {
	// Download new binary
	bin, err := u.downloadBinary()
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	
	// Get current executable path
	path, err := osext.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	
	// Apply the update
	err, errRecover := up.FromStream(bytes.NewBuffer(bin))
	if errRecover != nil {
		return fmt.Errorf("update and recovery errors: %q %q", err, errRecover)
	}
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	
	// Determine restart method
	if isRunningAsService() {
		return restartService()
	} else {
		return gracefulRestart(path)
	}
}

// downloadBinary downloads the binary from BinURL
func (u *Updater) downloadBinary() ([]byte, error) {
	resp, err := http.Get(u.BinURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}
	
	return io.ReadAll(resp.Body)
}

// isRunningAsService checks if the process is running as a systemd service
func isRunningAsService() bool {
	// Check for systemd-specific environment variables
	if os.Getenv("INVOCATION_ID") != "" {
		return true
	}
	
	// Check if running under systemd
	if _, err := exec.LookPath("systemctl"); err == nil {
		// Check if our process is managed by systemd
		cmd := exec.Command("systemctl", "is-active", "redock")
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	
	return false
}

// restartService restarts the systemd service
func restartService() error {
	if runtime.GOOS != "linux" {
		return errors.New("service restart only supported on Linux")
	}
	
	// Use systemctl to restart the service
	cmd := exec.Command("systemctl", "restart", "redock")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}
	
	// Exit this process (systemd will restart it)
	os.Exit(0)
	return nil
}

// gracefulRestart starts a new process and exits the current one
func gracefulRestart(execPath string) error {
	// Start new process with same arguments
	cmd := exec.Command(execPath, os.Args[1:]...)
	
	// Add environment variable to skip update check on restart
	env := os.Environ()
	env = append(env, "SKIP_UPDATE_CHECK=1")
	cmd.Env = env
	
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new process: %w", err)
	}
	
	// Give the new process time to start
	time.Sleep(5 * time.Second)
	
	// Check if new process is still running
	// On macOS, Signal(nil) doesn't work, so we use kill -0 as fallback
	checkCmd := exec.Command("kill", "-0", fmt.Sprintf("%d", cmd.Process.Pid))
	if err := checkCmd.Run(); err != nil {
		// Process doesn't exist or is not accessible
		log.Printf("❌ New process health check failed: process %d not found", cmd.Process.Pid)
		cmd.Process.Kill()
		return fmt.Errorf("new process failed to start properly: process not running")
	}
	
	// Give connections time to drain (optional)
	time.Sleep(2 * time.Second)
	
	// Exit old process
	os.Exit(0)
	
	return nil
}

// RollbackUpdate restores the previous version from backup
func RollbackUpdate() error {
	path, err := osext.Executable()
	if err != nil {
		return err
	}
	
	backupPath := path + ".backup"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return errors.New("no backup found")
	}
	
	// Restore backup
	if err := os.Rename(backupPath, path); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}
	
	return nil
}
