# [LOW-003] curl|bash Install Pattern

**Severity**: Low
**Responsibility**: Default Issue
**Component**: Install Script
**File(s)**:
- `install.sh:8-9` (usage instructions)

## Description

The install script documents usage as `curl -sSL ... | sudo bash`, which pipes remote content directly to a root shell. While this is industry-standard for developer tools (Docker, Homebrew, Rust, etc.), it is inherently risky:

- Partial downloads execute truncated code
- No opportunity for user review before execution
- Requires root trust in the download source

Mitigating factor: HTTPS is used, versions are pinned.

## Recommendation

Add a note in the documentation: "Review the script before running, or install manually." Provide manual installation instructions as an alternative.
