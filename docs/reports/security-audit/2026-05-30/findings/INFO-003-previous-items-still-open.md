# [INFO-003] Previous audit items still open

**Severity**: Informational
**Component**: Multiple
**File(s)**: Various

## Description

Several issues from prior audits remain unresolved:

| ID | Issue | First Reported | Status |
|----|-------|----------------|--------|
| LOW-001 | CLI --value flag exposes secrets in `ps` | 2026-03-27 | Still open |
| LOW-002 | Streaming RPCs bypass rate limiting | 2026-03-27 | Still open |
| LOW-003 | AES-GCM uses no additional authenticated data | 2026-03-27 | Still open |
| MED-001 | secrets.key permissions not verified | 2026-03-27 | Still open |
| MED-006 | Error messages leak internals (partial fix) | 2026-03-07 | Partially fixed — login errors now uniform but some RPCs still embed raw errors |

## Impact

These are lower severity items that haven't been addressed. They represent accumulated security tech debt.

## Recommendation

1. Address `--value` flag issue — this is a straightforward fix (remove flag or add warning)
2. Consider addressing the other LOW/MED items in the next sprint
3. Track security debt explicitly in the project issue tracker

## Secure Default Consideration

These are not new issues — they were identified in prior audits and remain unfixed. Tracking them here to ensure they're not lost.