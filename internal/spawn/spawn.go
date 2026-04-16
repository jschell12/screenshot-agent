// Package spawn runs the Claude Code CLI.
package spawn

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var ErrClaudeMissing = errors.New("claude CLI not found on PATH. Install: npm i -g @anthropic-ai/claude-code")

// Interactive runs `claude -p <prompt> --dangerously-skip-permissions`
// with inherited stdio and returns the exit code.
func Interactive(prompt, cwd string) (int, error) {
	cmd := exec.Command("claude", "-p", prompt, "--dangerously-skip-permissions")
	cmd.Dir = cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	var pathErr *exec.Error
	if errors.As(err, &pathErr) && pathErr.Err != nil {
		if os.IsNotExist(pathErr.Err) {
			return 1, ErrClaudeMissing
		}
	}
	return 1, err
}

// Captured runs claude and collects stdout+stderr for headless use (daemon).
type Captured struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func Capture(prompt, cwd string) (*Captured, error) {
	cmd := exec.Command("claude", "-p", prompt, "--dangerously-skip-permissions")
	cmd.Dir = cwd
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	cmd.Env = os.Environ()

	err := cmd.Run()
	c := &Captured{Stdout: so.String(), Stderr: se.String()}
	if err == nil {
		c.ExitCode = 0
		return c, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		c.ExitCode = ee.ExitCode()
		return c, nil
	}
	return nil, fmt.Errorf("spawn claude: %w", err)
}
