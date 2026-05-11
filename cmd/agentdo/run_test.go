package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubmitRequestCreatesSpoolFiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("AGENTDO_HOME", tempDir)

	req, err := submitRequest([]string{"/bin/echo", "hello"})
	if err != nil {
		t.Fatalf("submitRequest returned error: %v", err)
	}

	for _, path := range []string{
		requestPath(req.ID),
		statusPath(req.ID),
		stdoutPath(req.ID),
		stderrPath(req.ID),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected spool file %s: %v", path, err)
		}
	}

	status, err := loadStatus(req.ID)
	if err != nil {
		t.Fatalf("loadStatus returned error: %v", err)
	}
	if status.State != statePending {
		t.Fatalf("expected pending state, got %s", status.State)
	}
}

func TestValidateRequestIDRejectsUnsafeIDs(t *testing.T) {
	for _, id := range []string{
		"",
		".",
		"..",
		"../escape",
		"safe/../escape",
		"/tmp/escape",
		`..\escape`,
		"has space",
		"semi;colon",
	} {
		t.Run(id, func(t *testing.T) {
			if err := validateRequestID(id); err == nil {
				t.Fatalf("expected %q to be rejected", id)
			}
		})
	}
}

func TestListRequestsSkipsMismatchedIDs(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("AGENTDO_HOME", tempDir)

	if err := ensureLayout(); err != nil {
		t.Fatalf("ensureLayout returned error: %v", err)
	}

	id := "20260322-120000-deadbeef"
	if err := os.Mkdir(requestDir(id), 0o700); err != nil {
		t.Fatalf("mkdir request dir: %v", err)
	}
	req := &Request{ID: "../escape"}
	if err := writeJSONFile(requestPath(id), req, 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}
	status := &Status{ID: id, State: statePending}
	if err := writeJSONFile(statusPath(id), status, 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}

	requests, err := listRequests(true)
	if err != nil {
		t.Fatalf("listRequests returned error: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("expected mismatched request to be skipped, got %d request(s)", len(requests))
	}
}

func TestExecuteRequestWritesLogs(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("AGENTDO_HOME", tempDir)

	workDir := filepath.Join(tempDir, "cwd")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work dir: %v", err)
	}

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(previousWD)
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	req, err := submitRequest([]string{"/bin/echo", "approved"})
	if err != nil {
		t.Fatalf("submitRequest returned error: %v", err)
	}

	exitCode, err := executeRequest(req)
	if err != nil {
		t.Fatalf("executeRequest returned error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	stdoutBytes, err := os.ReadFile(stdoutPath(req.ID))
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if got := strings.TrimSpace(string(stdoutBytes)); got != "approved" {
		t.Fatalf("unexpected stdout log: %q", got)
	}
}
