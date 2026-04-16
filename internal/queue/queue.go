// Package queue handles on-disk task and result files.
package queue

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusProcessing TaskStatus = "processing"
	StatusDone       TaskStatus = "done"
	StatusError      TaskStatus = "error"
)

type Task struct {
	Repo      string     `json:"repo"`
	Message   string     `json:"message,omitempty"`
	Timestamp int64      `json:"timestamp"`
	Status    TaskStatus `json:"status"`
}

type Result struct {
	Status    string `json:"status"` // "success" or "error"
	PRUrl     string `json:"pr_url,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Summary   string `json:"summary"`
	Timestamp int64  `json:"timestamp"`
}

// NewTaskID returns a collision-resistant task id (ms timestamp + 2 random bytes).
func NewTaskID() string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}

// WriteTask creates <baseDir>/<taskID>/{task.json, screenshot.<ext>}.
func WriteTask(baseDir, taskID string, t Task, screenshotPath string) (string, error) {
	dir := filepath.Join(baseDir, taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "task.json"), data, 0o644); err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(screenshotPath))
	if ext == "" {
		ext = ".png"
	}
	if err := copyFile(screenshotPath, filepath.Join(dir, "screenshot"+ext)); err != nil {
		return "", err
	}
	return dir, nil
}

// WriteTaskFromBytes writes a task with screenshot bytes in-memory.
func WriteTaskFromBytes(baseDir, taskID string, t Task, shotName string, shotData []byte) (string, error) {
	dir := filepath.Join(baseDir, taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "task.json"), data, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, shotName), shotData, 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

func ReadTask(taskDir string) (*Task, error) {
	data, err := os.ReadFile(filepath.Join(taskDir, "task.json"))
	if err != nil {
		return nil, err
	}
	t := &Task{}
	if err := json.Unmarshal(data, t); err != nil {
		return nil, err
	}
	return t, nil
}

func UpdateTaskStatus(taskDir string, status TaskStatus) error {
	t, err := ReadTask(taskDir)
	if err != nil {
		return err
	}
	t.Status = status
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(taskDir, "task.json"), data, 0o644)
}

func WriteResult(resultsDir, taskID string, r Result) error {
	dir := filepath.Join(resultsDir, taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "result.json"), data, 0o644)
}

func ReadResult(resultDir string) (*Result, error) {
	data, err := os.ReadFile(filepath.Join(resultDir, "result.json"))
	if err != nil {
		return nil, err
	}
	r := &Result{}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, err
	}
	return r, nil
}

// FindScreenshot returns the screenshot file path within a task dir, if any.
func FindScreenshot(taskDir string) string {
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "screenshot.") {
			return filepath.Join(taskDir, e.Name())
		}
	}
	return ""
}

// ListPending returns pending task dirs in queueDir, oldest-first.
func ListPending(queueDir string) []string {
	entries, err := os.ReadDir(queueDir)
	if err != nil {
		return nil
	}
	type entry struct {
		dir string
		ts  int64
	}
	var pending []entry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(queueDir, e.Name())
		t, err := ReadTask(dir)
		if err != nil {
			continue
		}
		if t.Status == StatusPending {
			pending = append(pending, entry{dir, t.Timestamp})
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].ts < pending[j].ts })
	out := make([]string, len(pending))
	for i, p := range pending {
		out[i] = p.dir
	}
	return out
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
