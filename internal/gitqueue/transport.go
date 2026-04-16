// Package gitqueue implements an age-encrypted task queue over a private git repo.
package gitqueue

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func git(cwd string, args ...string) (stdout, stderr string, code int, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err = cmd.Run()
	stdout, stderr = so.String(), se.String()
	if err == nil {
		return stdout, stderr, 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return stdout, stderr, ee.ExitCode(), nil
	}
	return stdout, stderr, 1, err
}

func gitOrFail(cwd string, args ...string) (string, error) {
	stdout, stderr, code, err := git(cwd, args...)
	if err != nil || code != 0 {
		return stdout, fmt.Errorf("git %s: exit %d: %s: %w", strings.Join(args, " "), code, stderr, err)
	}
	return stdout, nil
}

// RepoURL returns the clone URL for a slug like "owner/repo", or passes through a full URL.
func RepoURL(slug string) string {
	if strings.HasPrefix(slug, "http") || strings.HasPrefix(slug, "git@") {
		return slug
	}
	return fmt.Sprintf("git@github.com:%s.git", slug)
}

// EnsureCloned clones the repo into cloneDir if missing, otherwise fetches.
func EnsureCloned(slug, cloneDir, branch string) error {
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cloneDir), 0o755); err != nil {
			return err
		}
		if _, err := gitOrFail("", "clone", RepoURL(slug), cloneDir); err != nil {
			return err
		}
	} else {
		if _, err := gitOrFail(cloneDir, "fetch", "origin"); err != nil {
			return err
		}
	}

	// Ensure branch exists / is checked out
	if _, err := gitOrFail(cloneDir, "checkout", branch); err != nil {
		if _, err2 := gitOrFail(cloneDir, "checkout", "-b", branch); err2 != nil {
			return fmt.Errorf("checkout %s: %w", branch, err2)
		}
	}
	return nil
}

// PullRebase pulls with rebase+autostash; tolerates missing remote refs.
func PullRebase(cloneDir, branch string) error {
	_, se, code, _ := git(cloneDir, "pull", "--rebase=true", "--autostash", "origin", branch)
	if code == 0 {
		return nil
	}
	if regexp.MustCompile(`couldn't find remote ref|no such ref`).MatchString(se) {
		return nil
	}
	return fmt.Errorf("git pull: %s", se)
}

// CommitAndPush stages the given paths, commits with message, pushes; retries
// once on non-fast-forward by pulling and retrying.
func CommitAndPush(cloneDir string, paths []string, message, branch, authorName, authorEmail string) error {
	if len(paths) == 0 {
		return nil
	}
	for _, p := range paths {
		if _, err := gitOrFail(cloneDir, "add", "--all", "--", p); err != nil {
			return err
		}
	}

	// Bail if nothing changed
	status, err := gitOrFail(cloneDir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}

	args := []string{}
	if authorName != "" && authorEmail != "" {
		args = append(args, "-c", "user.name="+authorName, "-c", "user.email="+authorEmail)
	}
	args = append(args, "commit", "-m", message)
	if _, err := gitOrFail(cloneDir, args...); err != nil {
		return err
	}

	_, se, code, _ := git(cloneDir, "push", "origin", branch)
	if code == 0 {
		return nil
	}
	if regexp.MustCompile(`non-fast-forward|rejected|fetch first`).MatchString(se) {
		if err := PullRebase(cloneDir, branch); err != nil {
			return err
		}
		if _, err := gitOrFail(cloneDir, "push", "origin", branch); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("git push: %s", se)
}

// ListFiles returns file names (not recursive) under subdir, excluding dotfiles.
func ListFiles(cloneDir, subdir string) ([]string, error) {
	dir := filepath.Join(cloneDir, subdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, filepath.Join(subdir, e.Name()))
	}
	return out, nil
}

func ReadFile(cloneDir, rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(cloneDir, rel))
}

func WriteFile(cloneDir, rel string, data []byte) error {
	full := filepath.Join(cloneDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func FileExists(cloneDir, rel string) bool {
	_, err := os.Stat(filepath.Join(cloneDir, rel))
	return err == nil
}

func GitRm(cloneDir, rel string) error {
	_, err := gitOrFail(cloneDir, "rm", "--", rel)
	return err
}
