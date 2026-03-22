# AGENT_INSTALL.md

Use this file as the install recipe for `agentdo`.

Repository:

- `https://github.com/ZimengXiong/agentdo`

Raw file:

- `https://raw.githubusercontent.com/ZimengXiong/agentdo/main/AGENT_INSTALL.md`

## Shell Install

Clone the repository, install the binary, and verify it:

```bash
git clone https://github.com/ZimengXiong/agentdo.git
cd agentdo
make install
agentdo version
```

If you need a user-local install:

```bash
git clone https://github.com/ZimengXiong/agentdo.git
cd agentdo
make install BINDIR="$HOME/.local/bin"
agentdo version
```

## Codex Non-Interactive

Feed this file directly into a non-interactive Codex run:

```bash
curl -fsSL https://raw.githubusercontent.com/ZimengXiong/agentdo/main/AGENT_INSTALL.md | codex exec --skip-git-repo-check -
```

## Codex JSON Streaming

Run the same install instructions, but emit JSONL events:

```bash
curl -fsSL https://raw.githubusercontent.com/ZimengXiong/agentdo/main/AGENT_INSTALL.md | codex exec --json --skip-git-repo-check -
```

## Agent Task

When this file is provided to an agent, do this:

1. Clone `https://github.com/ZimengXiong/agentdo.git`.
2. Change into the repository root.
3. Run `make install`.
4. Run `agentdo version`.
5. Report whether install succeeded and include the installed binary path from `which agentdo`.
