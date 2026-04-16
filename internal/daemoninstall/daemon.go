// Package daemoninstall installs and loads the xmuggled launchd plist.
//
// The plist template is embedded so the CLI is self-sufficient — it doesn't
// depend on the source repo being around after `make install`.
package daemoninstall

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed daemon.plist
var plistTemplate string

// PlistPath returns the destination path for the xmuggle daemon plist.
func PlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.xmuggle.daemon.plist")
}

// Install writes the launchd plist with substituted paths and loads it.
// Idempotent — unloads any existing plist at the destination first.
func Install() error {
	daemonBin, err := exec.LookPath("xmuggled")
	if err != nil {
		return fmt.Errorf("xmuggled not on PATH — did you run `make install`? (%w)", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	content := strings.ReplaceAll(plistTemplate, "__DAEMON_BIN__", daemonBin)
	content = strings.ReplaceAll(content, "__HOME__", home)

	dst := PlistPath()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	// Unload first (ignore errors — it may not be loaded yet).
	_ = exec.Command("launchctl", "unload", dst).Run()

	if err := exec.Command("launchctl", "load", dst).Run(); err != nil {
		return fmt.Errorf("launchctl load %s: %w", dst, err)
	}
	return nil
}
