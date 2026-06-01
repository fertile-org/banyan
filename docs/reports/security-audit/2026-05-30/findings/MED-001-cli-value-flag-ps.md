# [MED-001] CLI `--value` flag exposes secrets in process listing

**Severity**: Medium
**Responsibility**: Mitigation Gap
**Component**: CLI
**File(s)**: `cmd/banyan-cli/cmd/secret.go:58,68-79`

## Description

The `secret create` command accepts `--value` flag which causes the secret to appear in `ps aux` output — visible to anyone who can run `ps` on the system.

## Evidence

```go
// secret.go:58 - Flag definition with weak warning
secretCreateCmd.Flags().String("value", "", "Secret value (visible in shell history — prefer interactive prompt or --from-file)")

// Lines 68-79 - Flag is used without protection
valueFlag, _ := cmd.Flags().GetString("value")
switch {
case fromFile != "":
    // ...
case valueFlag != "":
    value = []byte(valueFlag)  // Secret in memory from flag
default:
    // Interactive prompt with hidden input (secure)
    fmt.Print("Enter secret value: ")
    raw, err := term.ReadPassword(int(os.Stdin.Fd()))
```

When running `banyan-cli secret create mysecret --value "supersecret"`, the entire command line — including the secret — is visible in `ps aux`.

## Impact

**Who can exploit**: Local user with access to run `ps` on the system where the command is executed.

**What they gain**: Steal secrets entered via `--value` flag.

**Blast radius**: Single user on the system — limited to local access.

## Recommendation

1. **Option A (recommended)**: Remove `--value` flag entirely — force users to use interactive prompt or `--from-file`.

2. **Option B**: Add prominent warning when the flag is used:
```go
if valueFlag != "" {
    slog.Warn("WARNING: Using --value exposes secret in process listing. Use --from-file or interactive prompt.")
    slog.Warn("Run 'ps aux' to see if your secret is visible.")
    value = []byte(valueFlag)
}
```

## Secure Default Consideration

**Checklist S4**: "Password not accepted as CLI flag in production — WARN — --password flag is visible in process listing. Warn users and recommend interactive prompt or environment variable."

Same applies to secrets — the flag should either be removed or produce a warning.