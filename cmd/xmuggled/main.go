package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	xmuggleDir = filepath.Join(homeDir(), ".xmuggle")
	configFile = filepath.Join(xmuggleDir, "daemon.json")
	pidFile    = filepath.Join(xmuggleDir, "daemon.pid")
	logFile    = filepath.Join(xmuggleDir, "daemon.log")
	queueDir   = filepath.Join(xmuggleDir, "queue-repo")
)

type RepoConfig struct {
	Path     string   `json:"path"`
	Pull     *bool    `json:"pull,omitempty"`
	Commands []string `json:"commands,omitempty"`
}

func (r *RepoConfig) UnmarshalJSON(data []byte) error {
	// Accept plain string: "/path/to/repo"
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		r.Path = s
		return nil
	}
	// Accept object: {"path": "/path/to/repo", ...}
	type alias RepoConfig
	return json.Unmarshal(data, (*alias)(r))
}

func (r RepoConfig) ShouldPull() bool {
	return r.Pull == nil || *r.Pull
}

type Config struct {
	Interval  int          `json:"interval"`
	QueueRepo string       `json:"queueRepo"`
	Repos     []RepoConfig `json:"repos"`
	Commands  []string     `json:"commands"`
}

func defaultConfig() Config {
	return Config{
		Interval: 30,
	}
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

// ── Config ──

func loadConfig() Config {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return defaultConfig()
	}
	cfg := defaultConfig()
	_ = json.Unmarshal(data, &cfg)
	if cfg.Interval < 1 {
		cfg.Interval = 30
	}
	return cfg
}

func saveConfig(cfg Config) {
	_ = os.MkdirAll(xmuggleDir, 0755)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(configFile, append(data, '\n'), 0644)
}

func ensureConfig() {
	if _, err := os.Stat(configFile); err != nil {
		saveConfig(defaultConfig())
	}
}

// ── Logging ──

var logWriter *os.File

func setupLog() {
	_ = os.MkdirAll(xmuggleDir, 0755)
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		logWriter = f
		log.SetOutput(f)
	}
	log.SetFlags(log.Ldate | log.Ltime)
}

func logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Println(msg)
	fmt.Println(msg)
}

// ── Git env ──

func gitEnv() []string {
	env := os.Environ()
	tokenFile := filepath.Join(xmuggleDir, "gh-token")
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		if data, err := os.ReadFile(tokenFile); err == nil {
			token = strings.TrimSpace(string(data))
		}
	}
	if token != "" {
		env = append(env,
			"GH_TOKEN="+token,
			"GIT_ASKPASS=echo",
			"GIT_TERMINAL_PROMPT=0",
		)
	}
	return env
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runShell(command, dir string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ── Queue repo sync ──

type taskMeta struct {
	Filenames []string `json:"filenames"`
	Project   string   `json:"project"`
	Message   string   `json:"message"`
	From      string   `json:"from"`
	Timestamp string   `json:"timestamp"`
}

func ensureQueueClone(cfg Config) bool {
	if cfg.QueueRepo == "" {
		return false
	}

	gitDir := filepath.Join(queueDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		logf("Cloning queue repo: %s", cfg.QueueRepo)
		_ = os.MkdirAll(queueDir, 0755)
		if out, err := runGit("", "clone", cfg.QueueRepo, queueDir); err != nil {
			logf("Queue clone failed: %s", out)
			return false
		}
	} else {
		if out, err := runGit(queueDir, "pull", "--rebase"); err != nil {
			logf("Queue pull failed: %s", out)
			return false
		}
	}
	return true
}

// resolveProject finds the local repo path for a project name.
// Checks configured repos first, then falls back to common dev directories.
func resolveProject(cfg Config, project string) string {
	name := filepath.Base(project) // handle "user/repo" or just "repo"

	// Check configured repos
	for _, r := range cfg.Repos {
		if filepath.Base(r.Path) == name {
			return r.Path
		}
	}

	// Check common locations
	home := homeDir()
	candidates := []string{
		filepath.Join(home, "development", "github.com", project),
		filepath.Join(home, "dev", project),
		filepath.Join(home, project),
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, ".git")); err == nil {
			return c
		}
	}

	return ""
}

func processQueue(cfg Config) {
	if !ensureQueueClone(cfg) {
		return
	}

	pendingDir := filepath.Join(queueDir, "pending")
	entries, err := os.ReadDir(pendingDir)
	if err != nil {
		return
	}

	host := hostname()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		taskDir := filepath.Join(pendingDir, entry.Name())
		metaFile := filepath.Join(taskDir, "meta.json")
		data, err := os.ReadFile(metaFile)
		if err != nil {
			continue
		}

		var m taskMeta
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}

		// Skip our own submissions
		if m.From == host {
			continue
		}

		// Resolve project to local path
		repoPath := resolveProject(cfg, m.Project)
		if repoPath == "" {
			logf("Unknown project %q in task %s, skipping", m.Project, entry.Name())
			continue
		}

		// Check if already done
		doneDir := filepath.Join(queueDir, "done", entry.Name())
		if _, err := os.Stat(doneDir); err == nil {
			continue
		}

		logf("Processing task %s for %s", entry.Name(), filepath.Base(repoPath))

		// Pull latest for the target repo
		if out, err := runGit(repoPath, "pull", "--ff-only"); err != nil {
			logf("  Pull failed for %s: %s", repoPath, out)
		}

		// Build prompt with image paths
		var imgPaths []string
		for _, f := range m.Filenames {
			p := filepath.Join(taskDir, f)
			if _, err := os.Stat(p); err == nil {
				imgPaths = append(imgPaths, p)
			}
		}

		if len(imgPaths) == 0 {
			logf("  No images found in task %s", entry.Name())
			continue
		}

		prompt := fmt.Sprintf(
			"Analyze the screenshot(s) at %s and fix any bugs or UI issues you find in this repo. %s",
			strings.Join(imgPaths, ", "), m.Message,
		)

		logf("  Spawning claude in %s", filepath.Base(repoPath))
		cmd := exec.Command("claude", "--print", "--dangerously-skip-permissions", prompt)
		cmd.Dir = repoPath
		cmd.Env = gitEnv()
		output, err := cmd.CombinedOutput()
		if err != nil {
			logf("  Claude failed: %v\n%s", err, string(output))
			continue
		}

		logf("  Claude finished")

		// Commit and push any code changes Claude made to the project repo
		if status, _ := runGit(repoPath, "status", "--porcelain"); status != "" {
			runGit(repoPath, "add", "-A")
			commitMsg := fmt.Sprintf("xmuggle: fix from task %s", entry.Name())
			if _, err := runGit(repoPath, "commit", "-m", commitMsg); err == nil {
				logf("  Pushing code changes")
				if out, err := runGit(repoPath, "push"); err != nil {
					logf("  Push failed: %s", out)
				}
			}
		}

		// Move task to done in queue repo
		_ = os.MkdirAll(filepath.Join(queueDir, "done"), 0755)
		_ = os.Rename(taskDir, doneDir)

		// Write result
		_ = os.WriteFile(filepath.Join(doneDir, "result.txt"), output, 0644)
		_ = os.WriteFile(filepath.Join(doneDir, "done.txt"),
			[]byte(time.Now().Format(time.RFC3339)+"\n"), 0644)

		// Commit and push queue repo
		runGit(queueDir, "add", "-A")
		queueMsg := fmt.Sprintf("done: %s", entry.Name())
		if _, err := runGit(queueDir, "commit", "-m", queueMsg); err == nil {
			runGit(queueDir, "pull", "--rebase")
			logf("  Pushing queue update")
			if out, err := runGit(queueDir, "push"); err != nil {
				logf("  Queue push failed: %s", out)
			}
		}
	}
}

// ── Repo sync ──

func syncRepos(cfg Config) {
	for _, repo := range cfg.Repos {
		if _, err := os.Stat(repo.Path); err != nil {
			logf("Repo not found: %s", repo.Path)
			continue
		}

		if repo.ShouldPull() {
			logf("Pulling %s", repo.Path)
			out, err := runGit(repo.Path, "pull", "--ff-only")
			if err != nil {
				logf("  Pull failed: %s", out)
			} else if out != "" && !strings.Contains(out, "Already up to date") {
				logf("  %s", out)
			}
		}

		for _, cmd := range repo.Commands {
			runCommand(cmd, repo.Path)
		}
	}
}

func runCommand(command, dir string) {
	logf("Running: %s", command)
	out, err := runShell(command, dir)
	if err != nil {
		logf("  Error: %s", out)
	} else if out != "" {
		logf("  %s", out)
	}
}

// ── Cycle ──

func runCycle() {
	cfg := loadConfig()

	processQueue(cfg)
	syncRepos(cfg)

	for _, cmd := range cfg.Commands {
		runCommand(cmd, "")
	}
}

// ── CLI ──

func main() {
	cmd := "help"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "start":
		ensureConfig()

		// Check if already running
		if pid, ok := readPid(); ok {
			fmt.Printf("Daemon already running (pid %d)\n", pid)
			return
		}

		// Re-exec as background process
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot find executable: %v\n", err)
			os.Exit(1)
		}
		child := exec.Command(exe, "_run-daemon")
		child.Env = os.Environ()
		// Detach from terminal
		child.Stdin = nil
		child.Stdout = nil
		child.Stderr = nil
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := child.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start daemon: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Daemon started (pid %d)\n", child.Process.Pid)

	case "_run-daemon":
		// Internal: the actual daemon loop, runs in background
		setupLog()
		cfg := loadConfig()

		_ = os.MkdirAll(xmuggleDir, 0755)
		_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)

		logf("Daemon starting (pid %d, interval %ds)", os.Getpid(), cfg.Interval)
		logf("Config: %s", configFile)

		// Graceful shutdown
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

		// Run immediately
		runCycle()

		ticker := time.NewTicker(time.Duration(cfg.Interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				runCycle()
			case s := <-sig:
				logf("Received %s, shutting down", s)
				_ = os.Remove(pidFile)
				return
			}
		}

	case "run":
		ensureConfig()
		setupLog()
		logf("Running single cycle")
		runCycle()
		logf("Done")

	case "stop":
		pid, ok := readPid()
		if !ok {
			fmt.Println("No daemon running")
			return
		}
		proc, _ := os.FindProcess(pid)
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			fmt.Printf("Could not stop pid %d: %v\n", pid, err)
		} else {
			_ = os.Remove(pidFile)
			fmt.Printf("Stopped daemon (pid %d)\n", pid)
		}

	case "status":
		if pid, ok := readPid(); ok {
			fmt.Printf("Daemon running (pid %d)\n", pid)
		} else {
			fmt.Println("Daemon not running")
		}
		cfg := loadConfig()
		fmt.Printf("Config:     %s\n", configFile)
		fmt.Printf("Interval:   %ds\n", cfg.Interval)
		fmt.Printf("Queue repo: %s\n", orDefault(cfg.QueueRepo, "(none)"))
		fmt.Printf("Repos:      %d\n", len(cfg.Repos))
		fmt.Printf("Commands:   %d\n", len(cfg.Commands))

	case "config":
		ensureConfig()
		data, _ := os.ReadFile(configFile)
		fmt.Print(string(data))

	case "edit":
		ensureConfig()
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		c := exec.Command(editor, configFile)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		_ = c.Run()

	case "add-repo":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: xmuggled add-repo <path> [command...]")
			os.Exit(1)
		}
		ensureConfig()
		cfg := loadConfig()
		abs, _ := filepath.Abs(os.Args[2])
		cmds := os.Args[3:]

		found := false
		for i, r := range cfg.Repos {
			if r.Path == abs {
				if len(cmds) > 0 {
					cfg.Repos[i].Commands = cmds
				}
				found = true
				fmt.Printf("Updated repo: %s\n", abs)
				break
			}
		}
		if !found {
			cfg.Repos = append(cfg.Repos, RepoConfig{Path: abs, Commands: cmds})
			fmt.Printf("Added repo: %s\n", abs)
		}
		saveConfig(cfg)

	case "set-queue":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: xmuggled set-queue <repo-url>")
			os.Exit(1)
		}
		ensureConfig()
		cfg := loadConfig()
		cfg.QueueRepo = os.Args[2]
		saveConfig(cfg)
		fmt.Printf("Queue repo set: %s\n", cfg.QueueRepo)

	case "add-command":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: xmuggled add-command <command>")
			os.Exit(1)
		}
		ensureConfig()
		cfg := loadConfig()
		cmd := strings.Join(os.Args[2:], " ")
		cfg.Commands = append(cfg.Commands, cmd)
		saveConfig(cfg)
		fmt.Printf("Added command: %s\n", cmd)

	case "log":
		n := 20
		if len(os.Args) > 2 {
			n, _ = strconv.Atoi(os.Args[2])
			if n < 1 {
				n = 20
			}
		}
		data, err := os.ReadFile(logFile)
		if err != nil {
			fmt.Println("No log file")
			return
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		start := len(lines) - n
		if start < 0 {
			start = 0
		}
		fmt.Println(strings.Join(lines[start:], "\n"))

	default:
		fmt.Print(`xmuggled — xmuggle sync daemon

Usage:
  xmuggled start                   Start the daemon (background)
  xmuggled run                     Run a single sync cycle
  xmuggled stop                    Stop the running daemon
  xmuggled status                  Show daemon status and config summary
  xmuggled config                  Print current config
  xmuggled edit                    Open config in $EDITOR
  xmuggled log [n]                 Show last n log lines (default 20)
  xmuggled set-queue <repo-url>    Set the queue repo URL
  xmuggled add-repo <path> [cmd]   Add a repo to sync
  xmuggled add-command <cmd>       Add a global command

Config: ~/.xmuggle/daemon.json
`)
	}
}

func readPid() (int, bool) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		_ = os.Remove(pidFile)
		return 0, false
	}
	return pid, true
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
