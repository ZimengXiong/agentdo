package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func run(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return nil
	}

	command := args[0]
	switch command {
	case "help", "--help", "-h":
		printUsage(os.Stdout)
		return nil
	case "version", "--version":
		fmt.Println(version)
		return nil
	case "run":
		return handleRun(args[1:])
	case "list":
		return handleList(args[1:])
	case "show":
		return handleShow(args[1:])
	case "wait":
		return handleWait(args[1:])
	case "approve":
		return handleApprove(args[1:])
	case "deny":
		return handleDeny(args[1:])
	case "cleanup":
		return handleCleanup(args[1:])
	default:
		return handleRun(args)
	}
}

func handleRun(args []string) error {
	noWait := false
	var commandArgs []string
	for _, arg := range args {
		if arg == "--no-wait" {
			noWait = true
			continue
		}
		commandArgs = append(commandArgs, arg)
	}
	if len(commandArgs) == 0 {
		return errors.New("usage: agentdo run [--no-wait] <command> [args...]")
	}
	req, err := submitRequest(commandArgs)
	if err != nil {
		return err
	}
	fmt.Printf("queued %s\n", req.ID)
	fmt.Printf("pending: %s\n", req.Command.Preview)
	fmt.Printf("approve in another terminal: sudo agentdo approve %s\n", req.ID)
	if noWait {
		return nil
	}
	return waitForRequest(req.ID, os.Stdout, os.Stderr)
}

func handleList(args []string) error {
	includeFinished := false
	if len(args) > 1 {
		return errors.New("usage: agentdo list [--all]")
	}
	if len(args) == 1 {
		if args[0] != "--all" {
			return errors.New("usage: agentdo list [--all]")
		}
		includeFinished = true
	}
	requests, err := listRequests(includeFinished)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		fmt.Println("no requests")
		return nil
	}
	for _, req := range requests {
		status, err := loadStatus(req.ID)
		if err != nil {
			return err
		}
		fmt.Printf("%s  %-9s  %s  %s\n", req.ID, status.State, req.Requester.User, req.Command.Preview)
	}
	return nil
}

func handleShow(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: agentdo show <id>")
	}
	req, err := loadRequest(args[0])
	if err != nil {
		return err
	}
	status, err := loadStatus(args[0])
	if err != nil {
		return err
	}
	if !isRoot() && req.Requester.UID != os.Getuid() {
		return errors.New("not allowed to inspect this request")
	}

	fmt.Printf("id: %s\n", req.ID)
	fmt.Printf("state: %s\n", status.State)
	fmt.Printf("submitted: %s\n", req.SubmittedAt.Format(time.RFC3339))
	fmt.Printf("requester: %s (uid=%d pid=%d host=%s)\n", req.Requester.User, req.Requester.UID, req.Requester.PID, req.Requester.Host)
	fmt.Printf("cwd: %s\n", req.Command.Cwd)
	fmt.Printf("executable: %s\n", req.Command.Executable)
	fmt.Printf("args: %s\n", strings.Join(req.Command.Args, " "))
	fmt.Printf("fingerprint: %s\n", req.Fingerprint)
	if status.ApprovedBy != "" {
		fmt.Printf("approved_by: %s\n", status.ApprovedBy)
	}
	if status.Error != "" {
		fmt.Printf("error: %s\n", status.Error)
	}
	if status.DeniedReason != "" {
		fmt.Printf("denied_reason: %s\n", status.DeniedReason)
	}
	if status.ExitCode != nil {
		fmt.Printf("exit_code: %d\n", *status.ExitCode)
	}
	return nil
}

func handleWait(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: agentdo wait <id>")
	}
	return waitForRequest(args[0], os.Stdout, os.Stderr)
}

func handleApprove(args []string) error {
	if !isRoot() {
		return errors.New("approve requires root; run with sudo")
	}
	yes := false
	var target string
	for _, arg := range args {
		if arg == "-y" || arg == "--yes" {
			yes = true
			continue
		}
		if target == "" {
			target = arg
			continue
		}
		return errors.New("usage: agentdo approve [-y] <id|all>")
	}
	if target == "" {
		return errors.New("usage: agentdo approve [-y] <id|all>")
	}
	if target == "all" {
		requests, err := listRequests(false)
		if err != nil {
			return err
		}
		var ids []string
		for _, req := range requests {
			status, err := loadStatus(req.ID)
			if err == nil && status.State == statePending {
				ids = append(ids, req.ID)
			}
		}
		sort.Strings(ids)
		if len(ids) == 0 {
			fmt.Println("no pending requests")
			return nil
		}
		for _, id := range ids {
			if err := approveRequest(id, yes); err != nil {
				return err
			}
		}
		return nil
	}
	return approveRequest(target, yes)
}

func handleDeny(args []string) error {
	if !isRoot() {
		return errors.New("deny requires root; run with sudo")
	}
	if len(args) == 0 {
		return errors.New("usage: agentdo deny <id|all> [reason...]")
	}
	target := args[0]
	reason := strings.TrimSpace(strings.Join(args[1:], " "))
	if target == "all" {
		requests, err := listRequests(false)
		if err != nil {
			return err
		}
		for _, req := range requests {
			status, err := loadStatus(req.ID)
			if err == nil && status.State == statePending {
				if err := denyRequest(req.ID, reason); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return denyRequest(target, reason)
}

func handleCleanup(args []string) error {
	olderThan := 72 * time.Hour
	if len(args) > 1 {
		return errors.New("usage: agentdo cleanup [duration]")
	}
	if len(args) == 1 {
		parsed, err := time.ParseDuration(args[0])
		if err != nil {
			return fmt.Errorf("parse duration: %w", err)
		}
		olderThan = parsed
	}
	cutoff := time.Now().UTC().Add(-olderThan)
	requests, err := listRequests(true)
	if err != nil {
		return err
	}
	removed := 0
	for _, req := range requests {
		status, err := loadStatus(req.ID)
		if err != nil || !isTerminalState(status.State) || status.UpdatedAt.After(cutoff) {
			continue
		}
		if !isRoot() && req.Requester.UID != os.Getuid() {
			continue
		}
		if err := os.RemoveAll(requestDir(req.ID)); err != nil {
			return err
		}
		removed++
	}
	fmt.Printf("removed %d request(s)\n", removed)
	return nil
}

func waitForRequest(id string, stdout, stderr io.Writer) error {
	req, err := loadRequest(id)
	if err != nil {
		return err
	}
	if !isRoot() && req.Requester.UID != os.Getuid() {
		return errors.New("not allowed to wait on this request")
	}
	var outOffset int64
	var errOffset int64
	for {
		status, err := loadStatus(id)
		if err != nil {
			return err
		}
		if err := copyNewBytes(stdoutPath(id), stdout, &outOffset); err != nil {
			return err
		}
		if err := copyNewBytes(stderrPath(id), stderr, &errOffset); err != nil {
			return err
		}
		if isTerminalState(status.State) {
			if err := copyNewBytes(stdoutPath(id), stdout, &outOffset); err != nil {
				return err
			}
			if err := copyNewBytes(stderrPath(id), stderr, &errOffset); err != nil {
				return err
			}
			return exitForStatus(status)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func copyNewBytes(path string, dst io.Writer, offset *int64) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < *offset {
		*offset = 0
	}
	if _, err := file.Seek(*offset, io.SeekStart); err != nil {
		return err
	}
	written, err := io.Copy(dst, file)
	*offset += written
	return err
}

func approveRequest(id string, yes bool) error {
	return withLock(id, func() error {
		req, err := loadRequest(id)
		if err != nil {
			return err
		}
		status, err := loadStatus(id)
		if err != nil {
			return err
		}
		if status.State != statePending {
			return fmt.Errorf("%s is %s", id, status.State)
		}
		fmt.Printf("approve %s\n", id)
		fmt.Printf("requester: %s (uid=%d pid=%d)\n", req.Requester.User, req.Requester.UID, req.Requester.PID)
		fmt.Printf("cwd: %s\n", req.Command.Cwd)
		fmt.Printf("command: %s\n", req.Command.Preview)
		fmt.Printf("fingerprint: %s\n", req.Fingerprint)
		if !yes {
			ok, err := promptYesNo("run this as root?")
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("approval cancelled")
			}
		}

		now := time.Now().UTC()
		approvedBy := rootActor()
		status.State = stateRunning
		status.UpdatedAt = now
		status.ApprovedBy = approvedBy
		status.ApprovedAt = &now
		status.StartedAt = &now
		status.Error = ""
		status.DeniedReason = ""
		status.ExitCode = nil
		if err := rewriteJSONFile(statusPath(id), status); err != nil {
			return err
		}

		exitCode, execErr := executeRequest(req)
		finished := time.Now().UTC()
		status.UpdatedAt = finished
		status.FinishedAt = &finished
		status.ExitCode = &exitCode
		if execErr != nil {
			status.State = stateFailed
			status.Error = execErr.Error()
		} else {
			status.State = stateCompleted
		}
		if err := rewriteJSONFile(statusPath(id), status); err != nil {
			return err
		}
		fmt.Printf("finished %s with exit code %d\n", id, exitCode)
		return nil
	})
}

func executeRequest(req *Request) (int, error) {
	if err := ensureExecutable(req.Command.Executable); err != nil {
		return -1, err
	}
	if !filepath.IsAbs(req.Command.Cwd) {
		return -1, fmt.Errorf("cwd must be absolute: %s", req.Command.Cwd)
	}

	stdoutFile, err := os.OpenFile(stdoutPath(req.ID), os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return -1, err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.OpenFile(stderrPath(req.ID), os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return -1, err
	}
	defer stderrFile.Close()

	cmd := exec.Command(req.Command.Executable, req.Command.Args...)
	cmd.Dir = req.Command.Cwd
	cmd.Env = approvalEnv(req)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return -1, err
	}
	defer devNull.Close()
	cmd.Stdin = devNull

	err = cmd.Run()
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), err
	}
	_, _ = fmt.Fprintf(stderrFile, "agentdo: %v\n", err)
	return -1, err
}

func approvalEnv(req *Request) []string {
	env := map[string]string{
		"PATH":    "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME":    rootHome(),
		"USER":    "root",
		"LOGNAME": "root",
	}
	for key, value := range req.Command.Env {
		env[key] = value
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	output := make([]string, 0, len(keys))
	for _, key := range keys {
		output = append(output, key+"="+env[key])
	}
	return output
}

func denyRequest(id, reason string) error {
	return withLock(id, func() error {
		req, err := loadRequest(id)
		if err != nil {
			return err
		}
		status, err := loadStatus(id)
		if err != nil {
			return err
		}
		if status.State != statePending {
			return fmt.Errorf("%s is %s", id, status.State)
		}
		now := time.Now().UTC()
		status.State = stateDenied
		status.UpdatedAt = now
		status.FinishedAt = &now
		status.ApprovedBy = rootActor()
		status.DeniedReason = strings.TrimSpace(reason)
		if err := rewriteJSONFile(statusPath(id), status); err != nil {
			return err
		}
		fmt.Printf("denied %s from %s\n", id, req.Requester.User)
		return nil
	})
}

func exitForStatus(status *Status) error {
	switch status.State {
	case stateCompleted:
		if status.ExitCode != nil && *status.ExitCode != 0 {
			return exitCodeError(*status.ExitCode)
		}
		return nil
	case stateFailed:
		if status.ExitCode != nil && *status.ExitCode >= 0 {
			return exitCodeError(*status.ExitCode)
		}
		if status.Error != "" {
			return errors.New(status.Error)
		}
		return errors.New("request failed")
	case stateDenied:
		if status.DeniedReason != "" {
			return fmt.Errorf("request denied: %s", status.DeniedReason)
		}
		return errors.New("request denied")
	default:
		return fmt.Errorf("unexpected terminal state: %s", status.State)
	}
}

type exitCodeError int

func (e exitCodeError) Error() string {
	return "exit status " + strconv.Itoa(int(e))
}

func isRoot() bool {
	return os.Geteuid() == 0
}

func rootActor() string {
	if sudoUser := strings.TrimSpace(os.Getenv("SUDO_USER")); sudoUser != "" {
		return sudoUser + " via sudo"
	}
	currentUser, err := user.Current()
	if err == nil {
		return currentUser.Username
	}
	return "root"
}

func rootHome() string {
	if currentUser, err := user.Lookup("root"); err == nil && currentUser.HomeDir != "" {
		return currentUser.HomeDir
	}
	if runtimeHome := strings.TrimSpace(os.Getenv("HOME")); isRoot() && runtimeHome != "" {
		return runtimeHome
	}
	if runtime.GOOS == "darwin" {
		return "/var/root"
	}
	return "/root"
}

func promptYesNo(prompt string) (bool, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return false, errors.New("approval requires a tty unless -y is provided")
	}
	fmt.Fprintf(os.Stdout, "%s [y/N]: ", prompt)
	var answer string
	if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func quoteCommand(executable string, args []string) string {
	parts := []string{shellQuote(executable)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`!&|;()<>{}[]*?") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "agentdo queues privileged commands for later approval.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  agentdo <command> [args...]")
	fmt.Fprintln(w, "  agentdo run [--no-wait] <command> [args...]")
	fmt.Fprintln(w, "  agentdo list [--all]")
	fmt.Fprintln(w, "  agentdo show <id>")
	fmt.Fprintln(w, "  agentdo wait <id>")
	fmt.Fprintln(w, "  sudo agentdo approve [-y] <id|all>")
	fmt.Fprintln(w, "  sudo agentdo deny <id|all> [reason...]")
	fmt.Fprintln(w, "  agentdo cleanup [duration]")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Queue root: %s\n", requestsDir())
}
