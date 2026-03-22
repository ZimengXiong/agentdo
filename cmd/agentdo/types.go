package main

import "time"

const (
	requestsDirName = "requests"
	requestFileName = "request.json"
	statusFileName  = "status.json"
	stdoutFileName  = "stdout.log"
	stderrFileName  = "stderr.log"
	lockFileName    = ".lock"
	version         = "0.1.0"
)

const (
	statePending   = "pending"
	stateRunning   = "running"
	stateCompleted = "completed"
	stateFailed    = "failed"
	stateDenied    = "denied"
)

type Request struct {
	ID          string      `json:"id"`
	Token       string      `json:"token"`
	Fingerprint string      `json:"fingerprint"`
	SubmittedAt time.Time   `json:"submitted_at"`
	Requester   Requester   `json:"requester"`
	Command     CommandSpec `json:"command"`
}

type Requester struct {
	User string `json:"user"`
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
	PID  int    `json:"pid"`
	Host string `json:"host"`
}

type CommandSpec struct {
	Executable string            `json:"executable"`
	Args       []string          `json:"args"`
	Cwd        string            `json:"cwd"`
	Env        map[string]string `json:"env,omitempty"`
	Preview    string            `json:"preview"`
}

type Status struct {
	ID           string     `json:"id"`
	State        string     `json:"state"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ApprovedBy   string     `json:"approved_by,omitempty"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	ExitCode     *int       `json:"exit_code,omitempty"`
	Error        string     `json:"error,omitempty"`
	DeniedReason string     `json:"denied_reason,omitempty"`
}
