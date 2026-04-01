# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Product Context

**Banyan is a container orchestration platform that bridges the gap between `docker compose up` and production.** It lets teams deploy containers across multiple servers using the same Docker Compose syntax they already know — no Helm charts, no CRDs, no new YAML dialect to learn.

### Target users

Small to medium teams (2-30 engineers) who:
- Already know Docker Compose and have outgrown a single server
- Don't have a dedicated DevOps/platform team — the same engineers who write code also deploy it
- Tried or evaluated Kubernetes and found it too complex for their needs
- Want to ship features, not operate infrastructure

### Design principles

Every code change should be evaluated against these principles:

- **Three concepts only: engine, agent, manifest.** Resist adding new abstractions, resource types, or configuration layers. If a feature can't be expressed through these three concepts, reconsider the approach.
- **Docker Compose compatibility.** If Compose has a syntax for something, use the same syntax. Users shouldn't need to learn new YAML fields.
- **Sensible defaults over configuration.** Every flag should have a default that works for most users. If a user must provide a value that has an obvious default, that's a bug.
- **CLI output is the UI.** Every command should show what happened, what's running, and what to do next. Error messages should tell the user how to fix the problem.
- **Honest about limitations.** Don't half-implement features. Either do it well or put it on the roadmap. A missing feature is better than a broken one.

### Anti-patterns to avoid

These patterns creep in from Kubernetes and enterprise software. They are wrong for Banyan:

- Don't add CRD-like abstractions or custom resource types
- Don't add YAML templating, variable substitution, or chart-like packaging
- Don't require external dependencies (no Consul, no Vault, no separate Prometheus setup)
- Don't add features that need a docs page to explain — if it needs a long explanation, simplify the feature first
- Don't optimize for the 1% edge case at the cost of the 99% common case

### Documentation

User-facing docs live in `website/src/content/docs/`. When writing or updating documentation, use the banyan-document-writer skill (`.claude/skills/banyan-document-writer/SKILL.md`) which defines the voice, tone, audience, and quality standards.

## **IMPORTANT: Development Rules**

**When working on this repository, you MUST follow these critical guidelines:**

- **Follow standard Go coding conventions.** All changes must be linted to ensure no broken code.

- **Always choose the simplest approach.** If there are 3 design patterns that can solve the problem, choose the simple but extensible pattern. Avoid over-engineering.

- **NEVER duplicate files, functions, or documentation.** Always update the original file directly. Don't create duplicates as backups - we don't need backups.

- **Always think deeply when working in this repository.** Tokens are not a problem - don't perform quick fixes based on small pieces of knowledge.

- **Always think like a senior software engineer** who knows what to do and what constitutes over-engineering.

- **All functions in this repository need unit tests.** When adding new functions or updating existing ones, you MUST review and update the corresponding unit tests.

- **When writing unit tests, make them simple and easy to understand.** Don't over-engineer unit tests.

- **Unit test code coverage must be > 90%** When adding new functions or updating existing ones, you MUST add unit tests to ensure that the code coverage remains above 90%. In case the function already has low coverage before, you MUST add unit tests to increase the coverage to above 90%.

- **ALWAYS use Serena MCP server for codebase understanding.** Before starting any task, use the Serena tools to understand the current codebase structure and context. This ensures you have complete, up-to-date knowledge of the project.

## Codebase Context

**MANDATORY**: Before working on any task, you MUST use the Serena MCP server to understand the current codebase:

- Use Serena tools to get file summaries and project structure
- Check related files and dependencies before making changes
- Understand the complete context before implementing solutions
- Use Serena to validate your changes don't break existing functionality

## Project Information

For detailed project overview, architecture, development commands, and implementation status, see [DEVELOPMENT.md](./DEVELOPMENT.md).

## Design System
Always read DESIGN.md before making any visual or UI decisions.
All font choices, colors, spacing, icons, and aesthetic direction are defined there.
Do not deviate without explicit user approval.
In QA mode, flag any code that doesn't match DESIGN.md.

## Skill routing

When the user's request matches an available skill, ALWAYS invoke it using the Skill
tool as your FIRST action. Do NOT answer directly, do NOT use other tools first.
The skill has specialized workflows that produce better results than ad-hoc answers.

Key routing rules:
- Product ideas, "is this worth building", brainstorming → invoke office-hours
- Bugs, errors, "why is this broken", 500 errors → invoke investigate
- Ship, deploy, push, create PR → invoke ship
- QA, test the site, find bugs → invoke qa
- Code review, check my diff → invoke review
- Update docs after shipping → invoke document-release
- Weekly retro → invoke retro
- Design system, brand → invoke design-consultation
- Visual audit, design polish → invoke design-review
- Architecture review → invoke plan-eng-review