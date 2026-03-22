package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

func baseDir() string {
	if custom := strings.TrimSpace(os.Getenv("AGENTDO_HOME")); custom != "" {
		return custom
	}
	return "/var/tmp/agentdo"
}

func requestsDir() string {
	return filepath.Join(baseDir(), requestsDirName)
}

func requestDir(id string) string {
	return filepath.Join(requestsDir(), id)
}

func requestPath(id string) string {
	return filepath.Join(requestDir(id), requestFileName)
}

func statusPath(id string) string {
	return filepath.Join(requestDir(id), statusFileName)
}

func stdoutPath(id string) string {
	return filepath.Join(requestDir(id), stdoutFileName)
}

func stderrPath(id string) string {
	return filepath.Join(requestDir(id), stderrFileName)
}

func lockPath(id string) string {
	return filepath.Join(requestDir(id), lockFileName)
}

func ensureLayout() error {
	if err := os.MkdirAll(baseDir(), 0o755); err != nil {
		return err
	}
	if err := os.Chmod(baseDir(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(requestsDir(), 0o777); err != nil {
		return err
	}
	return os.Chmod(requestsDir(), 0o1777)
}

func submitRequest(args []string) (*Request, error) {
	if len(args) == 0 {
		return nil, errors.New("missing command")
	}
	if err := ensureLayout(); err != nil {
		return nil, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}

	executable, err := resolveExecutable(args[0], cwd)
	if err != nil {
		return nil, err
	}

	currentUser, err := user.Current()
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	env := captureRequestEnv()
	req := &Request{
		ID:          id,
		Token:       token,
		SubmittedAt: time.Now().UTC(),
		Requester: Requester{
			User: currentUser.Username,
			UID:  os.Getuid(),
			GID:  os.Getgid(),
			PID:  os.Getpid(),
			Host: host,
		},
		Command: CommandSpec{
			Executable: executable,
			Args:       append([]string(nil), args[1:]...),
			Cwd:        cwd,
			Env:        env,
			Preview:    quoteCommand(executable, args[1:]),
		},
	}
	req.Fingerprint = fingerprintRequest(req)

	reqDir := requestDir(id)
	if err := os.Mkdir(reqDir, 0o700); err != nil {
		return nil, err
	}
	if err := writeJSONFile(requestPath(id), req, 0o600); err != nil {
		return nil, err
	}
	status := Status{
		ID:        id,
		State:     statePending,
		UpdatedAt: req.SubmittedAt,
	}
	if err := writeJSONFile(statusPath(id), &status, 0o600); err != nil {
		return nil, err
	}
	if err := writeTextFile(stdoutPath(id), "", 0o600); err != nil {
		return nil, err
	}
	if err := writeTextFile(stderrPath(id), "", 0o600); err != nil {
		return nil, err
	}
	return req, nil
}

func resolveExecutable(command, cwd string) (string, error) {
	if strings.ContainsRune(command, filepath.Separator) {
		abs := command
		var err error
		if !filepath.IsAbs(command) {
			abs, err = filepath.Abs(filepath.Join(cwd, command))
			if err != nil {
				return "", err
			}
		}
		return abs, ensureExecutable(abs)
	}
	found, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	if !filepath.IsAbs(found) {
		found, err = filepath.Abs(filepath.Join(cwd, found))
		if err != nil {
			return "", err
		}
	}
	return found, ensureExecutable(found)
}

func ensureExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", path)
	}
	return nil
}

func captureRequestEnv() map[string]string {
	keys := []string{"LANG", "LC_ALL", "LC_CTYPE", "TERM"}
	env := make(map[string]string)
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			env[key] = value
		}
	}
	return env
}

func fingerprintRequest(req *Request) string {
	payload := struct {
		Executable string            `json:"executable"`
		Args       []string          `json:"args"`
		Cwd        string            `json:"cwd"`
		Env        map[string]string `json:"env,omitempty"`
	}{
		Executable: req.Command.Executable,
		Args:       req.Command.Args,
		Cwd:        req.Command.Cwd,
		Env:        req.Command.Env,
	}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func randomID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(buf), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func writeJSONFile(path string, value any, mode fs.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func rewriteJSONFile(path string, value any) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		temp.Close()
		return err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := temp.Chown(int(stat.Uid), int(stat.Gid)); err != nil && !errors.Is(err, syscall.EPERM) {
			temp.Close()
			return err
		}
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func writeTextFile(path, content string, mode fs.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(content)
	return err
}

func loadRequest(id string) (*Request, error) {
	var req Request
	if err := readJSONFile(requestPath(id), &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func loadStatus(id string) (*Status, error) {
	var status Status
	if err := readJSONFile(statusPath(id), &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func listRequests(includeFinished bool) ([]*Request, error) {
	if err := ensureLayout(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(requestsDir())
	if err != nil {
		return nil, err
	}

	var requests []*Request
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		req, err := loadRequest(entry.Name())
		if err != nil {
			continue
		}
		status, err := loadStatus(entry.Name())
		if err != nil {
			continue
		}
		if !includeFinished && isTerminalState(status.State) {
			continue
		}
		if !isRoot() && req.Requester.UID != os.Getuid() {
			continue
		}
		requests = append(requests, req)
	}

	sort.Slice(requests, func(i, j int) bool {
		return requests[i].SubmittedAt.Before(requests[j].SubmittedAt)
	})
	return requests, nil
}

func withLock(id string, fn func() error) error {
	lockFile, err := os.OpenFile(lockPath(id), os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	return fn()
}

func isTerminalState(state string) bool {
	switch state {
	case stateCompleted, stateFailed, stateDenied:
		return true
	default:
		return false
	}
}
