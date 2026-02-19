# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

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