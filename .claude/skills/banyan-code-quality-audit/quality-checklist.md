# Banyan Quality Checklist

Specific checks organized by category. Use this as the reference during both full and change audits.

## DOC — Doc-Code Integrity

### DOC-1: Landing Page Claims

For each feature bullet on `website/src/content/docs/index.mdx`:

- [ ] "The YAML you already know" — verify manifest parser supports: `services`, `build`, `image`, `ports`, `environment`, `depends_on`
- [ ] "Three binaries, nothing else" — verify all three binaries build and the install script installs exactly these three
- [ ] "Built-in image registry" — verify `build:` in manifest triggers build+push to engine registry, and agents pull from it
- [ ] "Containers talk across servers" — verify VPC overlay is initialized AND containers on different agents can resolve each other by service name
- [ ] "Monitor from your terminal" — verify `status` and `logs` commands work; if `monitor` is mentioned, verify it's marked as "coming soon"
- [ ] "Open source, self-hosted" — verify LICENSE file is Apache 2.0

### DOC-2: README Claims

For each feature in README.md "What you get" section, verify the same as DOC-1 plus:

- [ ] Install commands in README actually work with current install.sh
- [ ] Getting started commands match current CLI interface
- [ ] Architecture diagram matches actual component layout

### DOC-3: Quickstart Accuracy

Walk through `getting-started/quickstart.md` as if you're a new user:

- [ ] Every command shown produces the expected output shown
- [ ] The order of commands is correct (no missing steps)
- [ ] The deployment example uses a manifest format that the parser accepts
- [ ] The "what just happened" explanations match what the code actually does

### DOC-4: CLI Reference

For `reference/cli.md`:

- [ ] Every command listed exists in `cmd/banyan-cli/cmd/`
- [ ] Every flag listed is registered on the command
- [ ] Every flag description matches what the code does with it
- [ ] No commands exist in code that are missing from the reference

### DOC-5: Manifest Reference

For `reference/manifest.md`:

- [ ] Every YAML field listed is parsed by `pkg/types/manifest.go`
- [ ] Every parsed field is actually used (not just parsed and ignored)
- [ ] Field descriptions match actual behavior
- [ ] Required vs optional matches validation logic

### DOC-6: Roadmap Accuracy

For `roadmap.md`:

- [ ] Every item marked "Done" has code that works end-to-end
- [ ] No item marked "Done" is actually partially implemented
- [ ] No unimplemented feature is documented as if it exists elsewhere in the docs
- [ ] Planned features are not mentioned as current capabilities anywhere

---

## INT — Integration Integrity

### INT-1: Package Import Chain

For every directory under `pkg/`:

- [ ] The package is imported by at least one file in the production import chain (see component-map.md)
- [ ] If imported only by test files, flag as dead code candidate
- [ ] If imported but the init/setup function is never called, flag as unintegrated

### INT-2: Engine Startup Completeness

Verify `engine.New()` and the engine startup path:

- [ ] etcd connection established (`storage.NewStoreWithOptions()`)
- [ ] VPC network initialized (`vpc.InitializeNetwork()`)
- [ ] Image registry started (`startRegistry()`)
- [ ] gRPC server started with auth interceptors (`startEngineGRPC()`)
- [ ] Orchestration loop started (`engineLoop()`)
- [ ] All new components (since last audit) are initialized

### INT-3: Agent Startup Completeness

Verify `agent.New()` and the agent startup path:

- [ ] Engine client connected (`NewEngineClient()`)
- [ ] gRPC readiness verified (`waitForEngineGRPC()`)
- [ ] Agent registered with engine (`client.Register()`)
- [ ] Task polling loop started (`agentLoop()`)
- [ ] Heartbeat loop started (`agentHeartbeat()`)
- [ ] Container health loop started (`containerHealthLoop()`)
- [ ] Agent gRPC server started for log streaming (`startAgentGRPC()`)

### INT-4: CLI Command Completeness

For every subcommand in `cmd/banyan-cli/cmd/`:

- [ ] The command is registered on `rootCmd` via `AddCommand`
- [ ] The command handler loads config, creates client, calls the appropriate RPC
- [ ] The command handles and displays errors from the RPC
- [ ] The command's `--help` text is accurate

### INT-5: gRPC Handler Coverage

- [ ] Every RPC in `engine.proto` has a handler in `pkg/engine/grpc_server.go`
- [ ] Every RPC in `agent.proto` has a handler in `pkg/agent/grpc_server.go`
- [ ] Every handler is reachable through CLI or agent code
- [ ] No handlers exist without corresponding proto definitions

### INT-6: Config Field Usage

For every field in config structs (`pkg/types/config.go`):

- [ ] The field is populated during init (from CLI flags or user input)
- [ ] The field is read during startup (passed to the component that needs it)
- [ ] No config fields are parsed/stored but never consumed

---

## DEAD — Dead Code

### DEAD-1: Unused Packages

- [ ] Every `pkg/` directory is in the production import chain
- [ ] No package exists solely because "we might need it later"
- [ ] Packages marked as unused in previous audits have been removed or integrated

### DEAD-2: Unused Exports

For public functions/types (capitalized names) in each package:

- [ ] Each is referenced outside its own package (or in tests)
- [ ] No public functions exist that are only called within the same package (should be private)
- [ ] No types are defined that nothing uses

### DEAD-3: Commented-Out Code

Search for large blocks of commented code (`//` or `/* */`):

- [ ] No functional code is commented out (use version control, not comments)
- [ ] No "temporary" commented blocks older than the current sprint

### DEAD-4: Stale TODOs

Search for `TODO`, `FIXME`, `HACK`, `XXX`, `TEMP`:

- [ ] Each has context (who, what, when, or a ticket/issue reference)
- [ ] None are older than 2 sprints without progress
- [ ] None reference features that have since been implemented differently

### DEAD-5: Unused Proto Fields

For each `.proto` file:

- [ ] Every message field is read or written somewhere in Go code
- [ ] No deprecated fields remain without the `[deprecated = true]` annotation
- [ ] No fields are populated but never read (or read but never populated)

### DEAD-6: Orphaned Test Fixtures

- [ ] Test helper functions are called by at least one test
- [ ] Test data files in `testdata/` are referenced by at least one test
- [ ] Mock implementations are used by at least one test

---

## SMELL — Code Smells

### SMELL-1: Error Handling

- [ ] No error returns are silently ignored (`_ = fn()`) without an explicit comment explaining why
- [ ] Errors are wrapped with context (`fmt.Errorf("doing X: %w", err)`)
- [ ] No errors are both logged and returned (pick one)
- [ ] `panic()` is not used in library code (`pkg/`) — only in `main()` or test setup
- [ ] Error handling style is consistent within each package

### SMELL-2: Goroutine Safety

- [ ] Every goroutine respects context cancellation
- [ ] No goroutines leak (started without a way to stop)
- [ ] Shared state is protected by mutex or channels
- [ ] No `time.Sleep()` in production code (use tickers, timers, or channels)

### SMELL-3: Complexity

- [ ] No function longer than ~80 lines (including comments)
- [ ] No conditional nesting deeper than 3 levels
- [ ] No function with more than 5 parameters (use options struct)
- [ ] No file larger than ~600 lines (split responsibilities)

### SMELL-4: Hardcoded Values

- [ ] No magic numbers without named constants
- [ ] No hardcoded file paths (use config or constants)
- [ ] No hardcoded ports, timeouts, or retry counts (use config with defaults)
- [ ] Defaults are defined in one place, not scattered across files

### SMELL-5: Naming

- [ ] No type stutter (`agent.AgentConfig` → should be `agent.Config`)
- [ ] Consistent naming for the same concept across packages (e.g., always `nodeName` or always `agentName`, not both)
- [ ] YAML field names use consistent case (all snake_case or all camelCase)
- [ ] CLI flags use consistent format (all kebab-case)

### SMELL-6: Duplication

- [ ] No copy-pasted code blocks across packages (extract to shared utility)
- [ ] No duplicated validation logic (CLI validates AND engine validates differently)
- [ ] No duplicated error message strings
- [ ] gRPC handler boilerplate follows a consistent pattern (not hand-rolled each time)

---

## TEST — Test Quality

### TEST-1: Coverage

- [ ] Every gRPC handler has at least one test
- [ ] Every public function in `pkg/` has at least one test
- [ ] Error paths are tested (not just happy paths)
- [ ] Edge cases in manifest parsing are tested (empty services, missing fields, invalid values)

### TEST-2: Meaningfulness

- [ ] Tests assert on behavior, not implementation details
- [ ] Tests check return values AND side effects, not just `err == nil`
- [ ] Table-driven tests have at least 3 cases (happy, error, edge)
- [ ] No test exists solely to assert `!= nil` on a returned object

### TEST-3: Correctness

- [ ] No tests pass for the wrong reason (asserting zero-value defaults)
- [ ] No `time.Sleep()` in tests (use channels, conditions, or polling)
- [ ] Tests don't depend on execution order
- [ ] Tests clean up resources (close clients, stop servers)

### TEST-4: Test Files Match Source Files

- [ ] Every `*.go` file in `pkg/` has a corresponding `*_test.go` (or tests in the package test file)
- [ ] No test files exist for source files that have been deleted

---

## CONS — Consistency

### CONS-1: Error Handling Pattern

Check that the same error handling style is used across all packages:

- [ ] Same wrapping pattern (`%w` everywhere, not mixed with `%v` or `errors.New`)
- [ ] Same approach to error types (custom types vs sentinel errors)
- [ ] Same log-vs-return policy within each layer (handlers log, libraries return)

### CONS-2: Logging Pattern

- [ ] Same logger used across all packages
- [ ] Same log level for similar events (auth failure = warn everywhere, not error in one place and warn in another)
- [ ] Structured fields consistent (always `"node"` not sometimes `"nodeName"` and sometimes `"agent"`)

### CONS-3: Config Pattern

- [ ] Config loaded the same way in engine, agent, and CLI
- [ ] CLI flags mapped to config struct fields consistently
- [ ] Default values defined in one place per config field

### CONS-4: gRPC Handler Pattern

- [ ] All handlers follow: validate input → execute logic → build response → return
- [ ] Same error code for the same kind of error across handlers
- [ ] Same response structure conventions

---

## BUILD — Build & Module Health

### BUILD-1: go.mod

- [ ] `go mod tidy` produces no changes
- [ ] No unused dependencies listed
- [ ] No missing dependencies
- [ ] No `replace` directives (unless for local development, documented)

### BUILD-2: Compilation

- [ ] `go build ./...` succeeds with no warnings
- [ ] `go vet ./...` reports no issues
- [ ] No build tags used that aren't documented

### BUILD-3: Linting

- [ ] Run project linter if configured (check `.golangci.yml` or `Makefile`)
- [ ] No linter suppression comments (`//nolint`) without explanation

---

## Quick Reference: Priority Order

During a time-constrained audit, check in this order:

1. **DOC-6** — Roadmap accuracy (fast check, high impact)
2. **DOC-1** — Landing page claims (user's first impression)
3. **INT-1** — Package import chain (catches VPC-style issues)
4. **INT-2/3** — Startup completeness (are new components wired in?)
5. **DEAD-1** — Unused packages (quick win)
6. **BUILD-1** — go.mod health (30-second check)
7. **TEST-1** — Coverage gaps (identifies risk)
8. Everything else

This order maximizes issue-finding per minute of audit time.
