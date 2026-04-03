---
phase: 11-audit-remediation-correctness-security-and-api-hardening
plan: 04
subsystem: transport, api, test
tags: [iscsi, digest, context, mock-target, byte-order]

requires:
  - phase: 11-audit-remediation-correctness-security-and-api-hardening
    provides: Plan 11-01 (MaxRecvDSL in framer)
provides:
  - Configurable digest byte order via WithDigestByteOrder
  - Context-aware async and PDU hook callbacks
  - Instrumented mock target with unhandled opcode logging
affects: [transport, session, options, test]

tech-stack:
  added: []
  patterns: [variadic parameters for backward-compatible API extension]

key-files:
  created: []
  modified:
    - internal/transport/framer.go
    - internal/transport/conn.go
    - internal/session/types.go
    - internal/session/metrics.go
    - options.go
    - test/target.go

key-decisions:
  - "Used variadic digestByteOrder parameter to avoid breaking all existing ReadRawPDU/WriteRawPDU callers"
  - "Breaking change for WithAsyncHandler/WithPDUHook is acceptable pre-v1.0"
  - "MockTarget strict mode is opt-in to avoid breaking existing tests"

patterns-established:
  - "Variadic parameters for backward-compatible extension of low-level functions"

requirements-completed: [AUDIT-4, AUDIT-14, AUDIT-15]

duration: 10min
completed: 2026-04-03
---

# Plan 11-04: API Hardening Summary

**Configurable digest byte order, context-aware callbacks, and instrumented mock target**

## Performance

- **Duration:** 10 min
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments
- Digest byte order configurable via WithDigestByteOrder (default LittleEndian)
- WithAsyncHandler and WithPDUHook callbacks receive context.Context
- MockTarget logs unhandled opcodes via slog and supports strict mode

## Task Commits

1. **Task 1+2: Digest byte order, context callbacks, mock target** - `7d10207` (fix)

## Decisions Made
- Used variadic parameter for digestByteOrder in framer to maintain backward compatibility

## Deviations from Plan
None.

## Issues Encountered
None.

---
*Phase: 11-audit-remediation-correctness-security-and-api-hardening*
*Completed: 2026-04-03*
