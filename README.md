# agentdo

`agentdo` lets a local agent queue a privileged command in one terminal and wait for a human to approve it from another terminal.

## Status

This is an MVP. It is designed for a single machine and a trusted local user who reviews the exact command before approval. It is not hardened like `sudo`, `polkit`, or commercial privilege brokers.

Requests live in `/var/tmp/agentdo/requests`. Each request stores the exact executable path, args, cwd, selected environment variables, and a fingerprint. The caller waits by polling request state and logs until `sudo agentdo approve <id>` runs the stored command with a minimal root environment.

## Workflow

Queue a request from an unprivileged terminal:

```bash
agentdo firewall-cmd --reload
```

Review pending requests:

```bash
agentdo list
sudo agentdo list
```

Approve and run a request as root from another terminal:

```bash
sudo agentdo approve 20260322-120000-deadbeef
```

Deny instead:

```bash
sudo agentdo deny 20260322-120000-deadbeef
```

## Commands

- `agentdo <command> [args...]`: queue a command and wait for approval by default
- `agentdo run [--no-wait] <command> [args...]`: explicit form of the same behavior
- `agentdo list [--all]`: list visible requests; root sees all users
- `agentdo show <id>`: inspect a request
- `agentdo wait <id>`: attach to a queued request and stream its logs
- `sudo agentdo approve [-y] <id|all>`: approve and execute one or all pending requests
- `sudo agentdo deny <id|all> [reason...]`: deny one or all pending requests
- `agentdo cleanup [duration]`: remove finished requests older than the given duration

## Quick Install

```bash
make install
```

## Security Notes

- The approval boundary is human review of the exact command right before execution.
- `agentdo` is safer when used for exact commands rather than shell snippets.
- The environment passed to approved commands is intentionally minimal.
- For production-grade endpoint privilege management, use mature native mechanisms like `sudo`, `polkit`, or an audited privileged helper architecture.
