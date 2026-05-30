# [HIGH-DOC-001] `status` command documented in CLI reference but missing from implementation

**Severity**: High
**Category**: DOC
**Component**: cmd/banyan-cli
**File(s)**: `website/src/content/docs/reference/cli.md:15`, `cmd/banyan-cli/cmd/root.go:1-51`

## Description

The CLI reference documentation at `reference/cli.md` states that `banyan-cli status` is a valid command (line 15: "up, down, scale, secret, engine, agent, deployment, container, events, logs, dashboard"). However, there is no `status` command implementation — no `status.go` file and no `statusCmd` registered in `root.go`.

The `engine` command provides status-like functionality (showing engine health and cluster summary), but a dedicated `status` command is not implemented.

## Evidence

**Documentation claim** (`website/src/content/docs/reference/cli.md:15`):
```
| `banyan-cli` | Client (up, down, scale, secret, engine, agent, deployment, container, events, logs, dashboard) | Any machine | `init` and `login` |
```

**Search for status command implementation:**
```bash
$ grep -r "StatusCommand\|statusCmd\|StatusCmd" /home/work/freelancer/banyan/cmd/banyan-cli/
# No matches found

$ ls /home/work/freelancer/banyan/cmd/banyan-cli/cmd/
# No status.go file exists
```

**root.go registered commands:**
```go
// cmd/banyan-cli/cmd/root.go - commands registered via AddCommand:
initCmd, deployCmd, downCmd, logsCmd, scaleCmd, secretCmd,
engineCmd, agentCmd, deploymentCmd, containerCmd, eventsCmd,
dashboardCmd, loginCmd, logoutCmd, whoamiCmd, userCmd, reconnectCmd
```

Notice: **no statusCmd**.

## Impact

**User impact**: Users who read the documentation and try `banyan-cli status` will receive an error: `Error: unknown command "status"`. This breaks the documented workflow and causes confusion.

**Developer impact**: The documentation is out of sync with implementation. Anyone following the docs will fail.

**System impact**: Minor — the `engine` command provides similar functionality, but the documentation should accurately reflect what exists.

## Recommendation

1. **Option A (Recommended)**: Remove `status` from the documentation table in `reference/cli.md` since it doesn't exist. The `engine` command provides equivalent functionality.
2. **Option B**: Implement a `status` command that shows cluster status. This would be a thin wrapper around the existing `engine` command functionality.

The docs at line 15 should be corrected to remove `status` from the list, or a `status` command should be implemented to match the documentation.