---
name: banyan-code-quality-audit
description: Code quality audit for the Banyan container orchestration platform. Finds doc-vs-code lies, dead code, unintegrated packages, code smells, test gaps, and consistency issues. Use when reviewing code quality, preparing for release, cleaning up tech debt, or after a fast development sprint. Triggers on "code quality", "code audit", "dead code", "code smells", "tech debt", or "quality review".
---

# Banyan Code Quality Audit

Systematic code quality review for Banyan. This skill finds the problems that accumulate during fast development: documentation that lies about what's implemented, packages that exist but aren't wired in, dead code, code smells, and inconsistencies.

**This skill produces reports only. It never modifies source code.**

## Why This Exists

Banyan moves fast. When you move fast, things fall out of sync:
- A package gets built but never integrated into the startup path
- Docs claim a feature works, but the code was never wired in
- A function gets replaced but the old one stays around
- Error handling is careful in one package and sloppy in another
- Tests exist but don't actually test the right thing

The VPC incident is the defining example: `pkg/vpc/` was fully implemented, tests passed, user docs said "built-in VPC networking" — but it was never integrated into engine/agent startup. Containers on different hosts couldn't talk to each other.

**This skill exists to make sure that never happens again.**

## Core Principle

> **If the docs say it works, the code must prove it. If the code exists, something must use it. If nothing uses it, it shouldn't exist.**

## Two Modes

### Mode 1: Full Audit

Use when: "code quality audit", "full quality review", no specific scope, preparing for release, or after a development sprint.

Comprehensive review of the entire codebase. Produces a complete report.

### Mode 2: Change Audit

Use when: reviewing a PR, specific files, a feature branch, or a recent change.

Targeted review checking whether changes introduce quality regressions, doc-code drift, or dead code.

**Auto-detect**: If the user mentions specific files, a PR, or a branch — use Change Audit. Otherwise, use Full Audit.

---

## Full Audit Workflow

### Phase 1: Map the Codebase

Before auditing, understand what exists and how it connects.

1. **Read the component map**: Start with [component-map.md](component-map.md) — it defines Banyan's expected wiring between packages.
2. **Read the quality checklist**: Review [quality-checklist.md](quality-checklist.md) for specific checks.
3. **Scan the current structure**: Use Serena tools and code exploration to verify the component map is still accurate. If new packages or files exist that aren't in the map, flag them — the map needs updating.
4. **Run Go tooling**:
   - `go vet ./...` — static analysis
   - `go build ./...` — verify everything compiles
   - Check `go.mod` for unused or missing dependencies

### Phase 2: Doc-Code Integrity ("The Lies")

**This is the highest-priority category.** A doc that lies is worse than a missing doc — it actively misleads users.

For every user-facing claim in the documentation:

1. **Read user docs** in `website/src/content/docs/`:
   - `index.mdx` — landing page feature claims
   - `getting-started/quickstart.md` — setup and first-deploy claims
   - `getting-started/installation.md` — install process claims
   - `guides/*.md` — feature-specific claims
   - `reference/manifest.md` — supported YAML fields
   - `reference/cli.md` — supported commands and flags
   - `roadmap.md` — what's done vs planned

2. **Read README.md** — feature claims, install instructions, "What you get" section.

3. **For each claim**, verify:
   - Does the code that implements it exist?
   - Is that code actually wired into the startup path? (Check [component-map.md](component-map.md))
   - Is it reachable by the user through CLI or manifest?
   - Does it work end-to-end, or just partially?

4. **For roadmap items marked "Done"**, verify the feature actually works. "Done" in the roadmap must mean "implemented, integrated, and reachable by users."

5. **Check CLI help text** (`--help` output for each command) — does it match actual behavior?

6. **Check error messages** — do they reference features, commands, or flags that exist?

### Phase 3: Integration Integrity

Find packages, functions, or features that exist in the code but aren't wired into the system.

1. **Package-level check**: For every directory under `pkg/`:
   - Is it imported by at least one file in `cmd/` or another `pkg/` that IS imported?
   - If it's only imported by test files, it's dead code.
   - Trace the import chain: `cmd/` → `pkg/engine` → `pkg/vpc` means VPC is integrated. `pkg/shared` imported by nothing means it's orphaned.

2. **Startup path check**: Walk the engine, agent, and CLI startup paths (see [component-map.md](component-map.md)):
   - Is every expected component initialized?
   - Are there init calls that are commented out or behind flags that are never set?
   - Are there TODO comments about integration that were never done?

3. **gRPC completeness**:
   - Every RPC defined in `.proto` files must have a handler in the server implementation
   - Every handler must be reachable through the CLI (for CLI RPCs) or agent (for agent RPCs)
   - No phantom RPCs (defined in proto, handler exists, but nothing calls it)

4. **Config completeness**:
   - Every config field in types must be read somewhere
   - Every CLI flag must be passed to the component that uses it
   - No config fields that are parsed but ignored

### Phase 4: Dead Code

Find code that serves no purpose.

1. **Unused packages**: Directories under `pkg/` that nothing imports
2. **Unused exports**: Public functions/types that are never called outside their package (use `grep` for the function name across the codebase)
3. **Unused imports**: `go vet` catches these, but also check for blank imports (`import _ "pkg"`) that may be stale
4. **Commented-out code**: Blocks of commented code that should either be restored or deleted
5. **Stale TODO/FIXME/HACK**: Comments indicating temporary solutions that became permanent
6. **Unused proto fields**: Fields defined in `.proto` that are never read or written in Go code
7. **Unused CLI flags**: Flags registered on commands but never accessed in the handler
8. **Orphaned test helpers**: Test utilities or mocks that no test uses

### Phase 5: Code Smells

Find patterns that indicate deeper problems.

1. **Go-specific smells**:
   - Error returns ignored (especially from `Close()`, network calls, file operations)
   - `panic()` in library code (should return errors instead)
   - `init()` functions with side effects
   - Goroutine leaks (started but never stopped, no context cancellation)
   - Race conditions (shared state without synchronization)
   - Hardcoded values that should be constants or config

2. **Complexity**:
   - Functions longer than ~50 lines — likely doing too much
   - Deeply nested conditionals (>3 levels)
   - Long parameter lists (>5 params — use options struct)
   - God files (>500 lines, multiple unrelated responsibilities)

3. **Duplication**:
   - Similar code blocks across packages (especially error handling, config loading, gRPC patterns)
   - Copy-pasted proto handling logic
   - Duplicated validation logic (CLI vs engine)

4. **Error handling**:
   - Swallowed errors (`_ = someFunction()` without good reason)
   - Errors without context (`return err` vs `return fmt.Errorf("doing X: %w", err)`)
   - Inconsistent error wrapping (some use `%w`, some use `%v`, some use string concat)
   - Errors logged AND returned (double-reporting)

5. **Naming**:
   - Inconsistent naming across packages (e.g., `NodeName` vs `AgentName` for the same concept)
   - Stutter (`agent.AgentConfig` instead of `agent.Config`)
   - Abbreviations without context (`cfg`, `mgr`, `svc` — OK in small scope, not in public API)

### Phase 6: Test Quality

Tests that exist but don't test the right things are worse than no tests — they give false confidence.

1. **Coverage gaps**: Functions and methods without test coverage, especially:
   - gRPC handlers (the most critical code paths)
   - Error paths (not just happy paths)
   - Edge cases in manifest parsing
   - Auth validation logic

2. **Test meaningfulness**:
   - Tests that only check `err == nil` without verifying the result
   - Tests that assert on implementation details instead of behavior
   - Tests with no assertions at all (just "it didn't panic")
   - Table-driven tests with only one test case

3. **Test correctness**:
   - Tests that pass for the wrong reason (checking a zero value that happens to be the default)
   - Tests with `time.Sleep` instead of proper synchronization
   - Tests that depend on execution order
   - Flaky tests (check CI history if available)

4. **Test maintenance**:
   - Test helpers more complex than the code they test
   - Tests for code that no longer exists
   - Skipped tests (`t.Skip()`) without explanation

### Phase 7: Consistency

Inconsistency in a codebase increases cognitive load and hides bugs.

1. **Error handling patterns**: Does every package handle errors the same way? Check:
   - Error wrapping style
   - Where errors are logged vs returned
   - Custom error types vs standard errors

2. **Logging patterns**: Is logging consistent?
   - Same logger across packages?
   - Consistent log levels for similar events?
   - Structured logging vs printf-style?

3. **Config patterns**: Is config loaded the same way everywhere?
   - CLI flags → config struct → component
   - Environment variables handled consistently?

4. **gRPC patterns**: Do all RPC handlers follow the same structure?
   - Input validation → business logic → response
   - Same error code usage across handlers?

5. **Naming conventions**:
   - YAML field names (snake_case? camelCase?)
   - Go struct tags consistent?
   - CLI flag names (kebab-case? consistent prefixes?)

### Phase 8: Build & Module Health

1. **go.mod cleanliness**:
   - Run `go mod tidy` — are there differences?
   - Unused dependencies listed?
   - Missing dependencies?
   - Replace directives that shouldn't be there?

2. **Build warnings**: Does `go build ./...` produce any warnings?

3. **Linter output**: If there's a linter config (`.golangci.yml`), run it. Otherwise check with standard `go vet`.

---

## Finding Classification

### Severity

| Level | Definition | Example |
|-------|-----------|---------|
| **Critical** | Doc claims feature works but it doesn't. User will follow docs, hit a wall, and lose trust. | Docs say "VPC networking" but containers can't communicate across hosts |
| **High** | Unintegrated component, significant dead code hiding real bugs, major code smell that could cause runtime issues | Package exists with tests but is never imported by cmd/ |
| **Medium** | Code duplication, inconsistent patterns, missing tests on important paths, stale TODOs | gRPC handlers follow different error patterns; critical function untested |
| **Low** | Minor naming issues, small duplication, informational TODOs, style inconsistencies | Stutter in type names; commented-out code block |
| **Informational** | Observations, suggestions, patterns worth considering | "This function could be simplified but works correctly" |

### Category

| Category | Tag | Description |
|----------|-----|-------------|
| Doc-Code Integrity | `DOC` | Documentation claims something the code doesn't deliver |
| Integration Integrity | `INT` | Code exists but isn't wired into the system |
| Dead Code | `DEAD` | Code that serves no purpose |
| Code Smell | `SMELL` | Patterns indicating deeper problems |
| Test Quality | `TEST` | Missing, weak, or incorrect tests |
| Consistency | `CONS` | Inconsistent patterns across the codebase |
| Build Health | `BUILD` | Module, dependency, or compilation issues |

---

## Report Format

### Full Audit

Create the report in `docs/reports/code-quality-audit/YYYY-MM-DD/`:

```
docs/reports/code-quality-audit/YYYY-MM-DD/
├── SUMMARY.md
└── findings/
    ├── CRIT-DOC-001-<short-name>.md
    ├── HIGH-INT-001-<short-name>.md
    ├── MED-SMELL-001-<short-name>.md
    └── ...
```

### SUMMARY.md Format

```markdown
# Code Quality Audit Report — YYYY-MM-DD

## Scope

[Full audit / Change audit of <scope>]

## Methodology

Systematic review against Banyan component map and quality checklist.
Go tooling: go vet, go build, go mod tidy results.

## Findings Summary

| Severity | DOC | INT | DEAD | SMELL | TEST | CONS | BUILD | Total |
|----------|-----|-----|------|-------|------|------|-------|-------|
| Critical |     |     |      |       |      |      |       |       |
| High     |     |     |      |       |      |      |       |       |
| Medium   |     |     |      |       |      |      |       |       |
| Low      |     |     |      |       |      |      |       |       |
| Info     |     |     |      |       |      |      |       |       |

## Critical Findings

[One-line descriptions with links to finding files]

## High Findings

[One-line descriptions with links to finding files]

## Top Recommendations

[3-5 prioritized actions]

## Packages Reviewed

[List each pkg/ directory with status]

## Doc Pages Verified

[List each doc page with pass/fail]
```

### Finding File Format

```markdown
# [SEV-CAT-NNN] Short descriptive title

**Severity**: Critical / High / Medium / Low / Informational
**Category**: DOC / INT / DEAD / SMELL / TEST / CONS / BUILD
**Component**: e.g., pkg/vpc, website/docs, cmd/banyan-cli
**File(s)**: `path/to/file.go:line`

## Description

What the issue is. Reference exact code.

## Evidence

What proves this is an issue:
- For DOC issues: Quote the doc claim, then show the code (or lack of code) that contradicts it
- For INT issues: Show the package exists, then show it's never imported in the startup path
- For DEAD issues: Show the code exists, then show nothing references it
- For SMELL issues: Show the code pattern and explain why it's problematic

## Impact

What happens if this isn't fixed:
- User impact (confusion, broken feature, lost trust)
- Developer impact (wasted time, hidden bugs, harder maintenance)
- System impact (runtime errors, performance, reliability)

## Recommendation

How to fix it. Be specific.
```

---

## Change Audit Workflow

### Step 1: Scope the Change

- What files changed?
- Which components are affected? (Check [component-map.md](component-map.md))
- Were any docs updated? Were any docs NOT updated that should have been?

### Step 2: Quality Impact Assessment

For each changed file, check:

- **Doc drift**: Does this change add/remove/modify a feature? If yes, do the docs still match?
- **Integration**: Does this add new code? Is it wired into the startup path?
- **Dead code**: Does this replace old code? Is the old code removed?
- **Tests**: Does this change have corresponding test updates?
- **Consistency**: Does this follow existing patterns in the codebase?

### Step 3: Report

Write a single file:
```
docs/reports/code-quality-audit/YYYY-MM-DD/CHANGE-AUDIT-<description>.md
```

---

## Rules

1. **Never modify source code.** Reports only.
2. **Doc-Code Integrity is always Phase 2.** It's the highest-priority category. A lie in the docs is worse than dead code.
3. **Trace the full path.** Don't just check if a package exists — trace from `cmd/main.go` through the startup path to verify it's actually initialized.
4. **Use Go tooling.** Run `go vet`, `go build`, `go mod tidy` — they catch real issues that code reading misses.
5. **Be specific.** Every finding needs file paths and line numbers.
6. **Don't flag style preferences.** If the code works and is readable, an unconventional style is not a finding. Focus on issues that cause real problems.
7. **Check both directions.** Doc → Code (does the claimed feature work?) AND Code → Doc (is the implemented feature documented?).
8. **Compare to prior audits.** If previous reports exist in `docs/reports/code-quality-audit/`, note what's been fixed and what's regressed.
