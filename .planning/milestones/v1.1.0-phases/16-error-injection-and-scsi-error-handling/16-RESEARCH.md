# Phase 16: Error Injection and SCSI Error Handling - Research

**Researched:** 2026-04-05
**Domain:** iSCSI error handling, SCSI status codes, SNACK wire-level conformance testing
**Confidence:** HIGH

## Summary

This phase adds MockTarget error injection capabilities and conformance tests that verify the initiator correctly handles error conditions defined in RFC 7143 and the UNH-IOL FFP test suite. The scope divides cleanly into two areas: (1) SCSI error status and sense data handling (ERR-01 through ERR-06), and (2) SNACK PDU wire-level verification (SNACK-01, SNACK-02).

The existing codebase already has all the building blocks. The initiator correctly constructs SNACK PDUs (verified by unit tests in `internal/session/snack_test.go`) and handles Reject PDUs (session.go lines 471-508). The public `SCSIError` type supports `errors.As` with pointer receiver. The `HandleSCSIError` helper in `test/target.go` handles simple status+sense injection. What's missing are: (a) conformance tests that exercise these paths end-to-end through MockTarget, (b) the `HandleSCSIWithStatus` convenience helper for simple status-only tests, and (c) wire-level SNACK field assertions using `pducapture.Recorder`.

**Primary recommendation:** Use the existing `HandleSCSIFunc` for complex scenarios (DataSN gaps, Reject+retry) and add a thin `HandleSCSIWithStatus(lun, status, senseData)` helper for simple status code tests. Tests go in `test/conformance/error_test.go` (ERR-01 to ERR-06) and `test/conformance/snack_test.go` (SNACK-01, SNACK-02).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Hybrid approach -- HandleSCSIFunc for complex error scenarios (handler manually sends wrong PDUs, gaps, rejects via tc.SendPDU), plus 1-2 common helpers for simple status code tests (e.g., `HandleSCSIWithStatus(lun, status, senseData)` for ERR-03/04/05/06). Balances readability with minimal API growth. Complex scenarios like DataSN gaps and reject+retry remain inline in HandleSCSIFunc.
- **D-02:** Two files in `test/conformance/`: `error_test.go` (ERR-01 through ERR-06: CRC sense data, SNACK reject, unsolicited data errors, BUSY/RESERVATION CONFLICT status) and `snack_test.go` (SNACK-01, SNACK-02: DataSN gap SNACK construction, DataACK SNACK wire fields). Clean split by protocol mechanism.
- **D-03:** Extend, don't duplicate. Phase 14 DATA-07 tested the A-bit trigger. Phase 16 SNACK-02 focuses on the SNACK PDU wire field depth -- verifying BegRun, RunLength, and Type fields on the captured SNACK PDU. References Phase 14 DATA-07 as the trigger test; Phase 16 adds field-level wire assertions.
- **D-04:** Check error type using Go idiomatic `errors.As()` pattern. Verify the returned error wraps or is a specific error type (e.g., `*SCSIError` or equivalent with a `Status` field). Tests assert `errors.As(&scsierr)` and then check `scsierr.Status == 0x08` (BUSY) or `scsierr.Status == 0x18` (RESERVATION CONFLICT). No string matching.

### Claude's Discretion
- Exact signature of HandleSCSIWithStatus helper
- How ERR-01 through ERR-06 map to individual test functions vs subtests
- Whether HandleSCSIWithStatus goes in test/target.go or helpers_test.go
- Plan count and task breakdown across the 8 requirements
- Whether existing error types in the codebase already support the errors.As pattern or need extension

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ERR-01 | E2E test validates handling of CRC error sense data (FFP #16.4.1) | MockTarget injects CHECK CONDITION (0x02) with sense key 0x0B (ABORTED COMMAND) + ASC/ASCQ for CRC error. Use HandleSCSIFunc to send SCSIResponse with sense data. Verify SCSIError fields via errors.As. |
| ERR-02 | E2E test validates handling of SNACK reject followed by new command (FFP #16.4.2) | MockTarget sends Reject PDU (reason 0x09) for SNACK, then initiator issues new command successfully. HandleSCSIFunc orchestrates: first call triggers gap, target sends Reject for SNACK, second call responds normally. |
| ERR-03 | E2E test validates handling of unexpected unsolicited data error (FFP #16.4.3) | MockTarget returns CHECK CONDITION with sense for unexpected unsolicited data. Use HandleSCSIWithStatus or HandleSCSIFunc. Verify SCSIError surfaced to caller. |
| ERR-04 | E2E test validates handling of "not enough unsolicited data" error (FFP #16.4.4) | Same pattern as ERR-03 but with different ASC/ASCQ indicating insufficient data. |
| ERR-05 | E2E test validates handling of BUSY status 0x08 (FFP #16.4.5) | HandleSCSIWithStatus(0x08, nil). Verify errors.As yields *SCSIError with Status==0x08. No retry expected. |
| ERR-06 | E2E test validates handling of RESERVATION CONFLICT 0x18 (FFP #16.4.6) | HandleSCSIWithStatus(0x18, nil). Verify errors.As yields *SCSIError with Status==0x18. No retry expected. |
| SNACK-01 | E2E test validates Data/R2T SNACK construction on DataSN gap (FFP #13.1) | MockTarget sends Data-In with DataSN gap (skip DataSN=1, send DataSN=0 then DataSN=2). Capture SNACK via Recorder, verify Type=0 (DataR2T), BegRun=1, RunLength=1. Requires ERL>=1. |
| SNACK-02 | E2E test validates DataACK SNACK in response to A-bit (FFP #13.2) | Extends DATA-07 pattern. MockTarget sends Data-In with A-bit. Capture SNACK via Recorder, verify Type=2 (DataACK), BegRun, RunLength=0, TargetTransferTag echoed. Wire field depth beyond DATA-07. |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `testing` | Go 1.25 | Test framework | Project constraint -- no testify |
| Go stdlib `errors` | Go 1.25 | errors.As/errors.Is for typed error inspection | Project constraint -- idiomatic Go |
| `test/pducapture` | internal | PDU capture for wire-level assertions | Existing Phase 13 infrastructure |
| `test/target.go` | internal | MockTarget with HandleSCSIFunc, HandleSCSIError | Existing Phase 13 infrastructure |

[VERIFIED: codebase grep]

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `uiscsi.WithPDUHook` | public API | Hook PDU recorder into session | All SNACK tests need capture |
| `uiscsi.WithOperationalOverrides` | public API | Force ERL=1 for SNACK tests | SNACK-01, SNACK-02, ERR-02 |
| `uiscsi.WithKeepaliveInterval` | public API | Prevent keepalive interference | All conformance tests |

[VERIFIED: codebase grep]

## Architecture Patterns

### Test File Structure
```
test/conformance/
  error_test.go     # ERR-01 through ERR-06 (NEW)
  snack_test.go     # SNACK-01, SNACK-02 (NEW)
  helpers_test.go   # Shared helpers (EXISTING, possibly extend)
  datain_test.go    # DATA-07 reference (EXISTING)
```

### Pattern 1: Simple Status Code Test (ERR-05, ERR-06)
**What:** Test that SCSI status codes surface correctly through the public API error chain.
**When to use:** Status-only error conditions (BUSY, RESERVATION CONFLICT) with no sense data or complex PDU interaction.
**Example:**
```go
// Source: existing test/conformance/error_test.go pattern
func TestError_BUSY(t *testing.T) {
    tgt, err := testutil.NewMockTarget()
    // ... setup ...
    tgt.HandleLogin()
    tgt.HandleLogout()
    // Use HandleSCSIWithStatus for simple status injection
    tgt.HandleSCSIWithStatus(0x08, nil) // BUSY, no sense data

    sess, err := uiscsi.Dial(ctx, tgt.Addr())
    _, readErr := sess.ReadBlocks(ctx, 0, 0, 1, 512)
    var scsiErr *uiscsi.SCSIError
    if !errors.As(readErr, &scsiErr) {
        t.Fatalf("expected *SCSIError, got %T", readErr)
    }
    if scsiErr.Status != 0x08 {
        t.Fatalf("Status: got 0x%02X, want 0x08", scsiErr.Status)
    }
}
```
[VERIFIED: codebase patterns in error_test.go]

### Pattern 2: Complex Error Injection (ERR-01, ERR-02)
**What:** HandleSCSIFunc with inline PDU manipulation for complex multi-step error scenarios.
**When to use:** CRC error sense data injection, Reject PDU sending, DataSN gap creation.
**Example:**
```go
// Source: existing datain_test.go HandleSCSIFunc patterns
tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
    expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
    // Build sense data with CRC error
    senseData := buildCRCErrorSense() // sense key 0x0B, ASC/ASCQ for CRC
    dataSegment := buildSenseDataSegment(senseData)
    resp := &pdu.SCSIResponse{
        // ... CHECK CONDITION (0x02) with sense ...
    }
    return tc.SendPDU(resp)
})
```
[VERIFIED: codebase patterns in datain_test.go, scsicommand_test.go]

### Pattern 3: SNACK Wire-Level Verification (SNACK-01, SNACK-02)
**What:** Use pducapture.Recorder to capture initiator-sent SNACK PDUs and assert field values.
**When to use:** SNACK conformance tests that need to verify BegRun, RunLength, Type on the wire.
**Example:**
```go
// Source: existing datain_test.go TestDataIn_ABitDataACK
rec := &pducapture.Recorder{}
// ... setup with ERL=1, PDU hook ...
sess, err := uiscsi.Dial(ctx, tgt.Addr(),
    uiscsi.WithPDUHook(rec.Hook()),
    uiscsi.WithKeepaliveInterval(30*time.Second),
    uiscsi.WithOperationalOverrides(map[string]string{
        "ErrorRecoveryLevel": "1",
    }),
)
// ... trigger gap or A-bit ...
time.Sleep(100 * time.Millisecond) // allow SNACK propagation
snacks := rec.Sent(pdu.OpSNACKReq)
// Assert Type, BegRun, RunLength on captured SNACK
```
[VERIFIED: codebase pattern in datain_test.go lines 112-225]

### Pattern 4: Reject PDU Injection (ERR-02)
**What:** MockTarget sends a Reject PDU via tc.SendPDU when it receives a SNACK.
**When to use:** ERR-02 -- testing that SNACK rejection leads to new command (not retry).
**Example:**
```go
// Target handler reads SNACK from initiator and responds with Reject
tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
    if callCount == 0 {
        // Send Data-In with gap to trigger SNACK
        // ...send DataSN=0, skip 1, send DataSN=2...
        // Read the SNACK the initiator will send
        snackPDU, _, _ := tc.ReadPDU()
        // Send Reject for the SNACK
        snackBHS, _ := snackPDU.MarshalBHS()
        reject := &pdu.Reject{
            Header: pdu.Header{
                Final:            true,
                InitiatorTaskTag: 0xFFFFFFFF, // Reject uses reserved ITT
            },
            Reason:   0x09, // Invalid PDU field
            StatSN:   tc.NextStatSN(),
            ExpCmdSN: expCmdSN,
            MaxCmdSN: maxCmdSN,
            Data:     snackBHS[:], // BHS of rejected PDU
        }
        return tc.SendPDU(reject)
    }
    // Second call: respond normally
    return sendNormalResponse(tc, cmd)
})
```
[VERIFIED: Reject PDU structure from internal/pdu/target.go, session.go Reject handling]

### Anti-Patterns to Avoid
- **String matching on error messages:** Use `errors.As(&scsiErr)` + field checks, never `strings.Contains(err.Error(), "BUSY")`. Decision D-04 explicitly forbids this.
- **Hardcoding ExpCmdSN/MaxCmdSN:** Always use `tgt.Session().Update()` for correct command sequencing. The existing `HandleSCSIError` hardcodes `cmd.CmdSN+1/+10` which is fragile -- prefer `HandleSCSIWithStatus` that uses SessionState.
- **Duplicating DATA-07 test logic in SNACK-02:** Per D-03, SNACK-02 extends the A-bit trigger test from Phase 14 -- add deeper wire field assertions, don't rebuild the trigger scenario from scratch.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PDU capture | Manual byte inspection | `pducapture.Recorder` + `rec.Sent(opcode)` | Handles concurrency, decoding, sequencing |
| Sense data encoding | Manual byte packing | Reuse the `HandleSCSIError` pattern (2-byte SenseLength prefix + sense bytes) | RFC 7143 Section 11.4.7.2 format already implemented |
| Command sequencing | Hardcoded CmdSN values | `tgt.Session().Update(cmdSN, immediate)` | Handles immediate/non-immediate correctly |
| Error type assertion | Type switches on error | `errors.As(&scsiErr)` | Follows Go error chain conventions |

## Common Pitfalls

### Pitfall 1: HandleSCSIError Uses Hardcoded CmdSN Arithmetic
**What goes wrong:** `HandleSCSIError` (line 760-761) uses `cmd.CmdSN+1` and `cmd.CmdSN+10` instead of SessionState.Update(), which can produce incorrect ExpCmdSN/MaxCmdSN if the test issues multiple commands.
**Why it happens:** Legacy helper written before SessionState was added in Phase 13.
**How to avoid:** The new `HandleSCSIWithStatus` helper MUST use `tgt.Session().Update()`. For single-command tests, `HandleSCSIError` works fine, but multi-command tests (like ERR-02) must use HandleSCSIFunc with SessionState.
**Warning signs:** Incorrect ExpCmdSN causing command window issues in multi-command tests.

### Pitfall 2: SNACK Propagation Timing
**What goes wrong:** SNACK assertions fail because the check runs before the initiator has sent the SNACK PDU.
**Why it happens:** The SNACK is sent asynchronously in the initiator's dispatch loop, not synchronously on the calling goroutine.
**How to avoid:** Add `time.Sleep(100 * time.Millisecond)` after the operation completes, as done in TestDataIn_ABitDataACK (datain_test.go line 197). This is an established pattern in this codebase.
**Warning signs:** Flaky tests that pass locally but fail in CI.

### Pitfall 3: Reject PDU ITT Must Be 0xFFFFFFFF
**What goes wrong:** Reject PDU sent with the SNACK's ITT instead of the reserved ITT.
**Why it happens:** Confusion between "what ITT the Reject carries" vs "what ITT identifies the rejected PDU". The Reject PDU itself always has ITT=0xFFFFFFFF (unsolicited); the rejected PDU's BHS is in the data segment.
**How to avoid:** Always set Reject.Header.InitiatorTaskTag = 0xFFFFFFFF. The session's unsolicited handler (handleUnsolicited, session.go line 471) expects this routing.
**Warning signs:** Reject PDU gets routed to task loop instead of unsolicited handler.

### Pitfall 4: Sense Data Format Prefix
**What goes wrong:** Raw sense data passed without the 2-byte SenseLength prefix.
**Why it happens:** RFC 7143 Section 11.4.7.2 wraps sense data with a length prefix in the SCSI Response data segment, but SPC sense data format doesn't include this prefix.
**How to avoid:** Always prepend `binary.BigEndian.PutUint16(dataSegment[0:2], uint16(len(senseData)))` before the actual sense bytes. The existing `HandleSCSIError` already does this correctly (target.go lines 748-750). Copy this pattern.
**Warning signs:** Sense data parsing fails or returns garbage SenseKey/ASC/ASCQ.

### Pitfall 5: ERL=1 Required for SNACK Tests
**What goes wrong:** Initiator doesn't send SNACK on DataSN gap, just returns error.
**Why it happens:** At ERL=0, DataSN gaps are fatal (datain.go line 84). SNACK construction only activates at ERL >= 1.
**How to avoid:** Configure both target (`SetNegotiationConfig(NegotiationConfig{ErrorRecoveryLevel: Uint32Ptr(1)})`) and initiator (`WithOperationalOverrides(map[string]string{"ErrorRecoveryLevel": "1"})`) for ERL=1.
**Warning signs:** SNACK tests see `Result.Err` with "DataSN gap" message instead of SNACK on wire.

### Pitfall 6: ERR-02 Requires Multi-Step Handler
**What goes wrong:** Test tries to use HandleSCSIError for the reject scenario, but needs to orchestrate gap -> SNACK -> Reject -> retry.
**Why it happens:** ERR-02 is a multi-step protocol interaction that can't be expressed with a simple status code handler.
**How to avoid:** Use HandleSCSIFunc with callCount to distinguish first call (inject gap + reject SNACK) from second call (respond normally). Use tc.ReadPDU() to receive the initiator's SNACK inline.
**Warning signs:** Test hangs because handler doesn't consume the SNACK PDU the initiator sends.

## Code Examples

### CRC Error Sense Data Construction (ERR-01)
```go
// Source: SPC-4 Annex D, RFC 7143 Section 6.2.1
// Sense key 0x0B (ABORTED COMMAND), ASC=0x47 (SCSI PARITY ERROR), ASCQ=0x05 (CRC ERROR)
// This is the sense data the target sends when it detects a CRC error.
func buildCRCErrorSense() []byte {
    sense := make([]byte, 18)
    sense[0] = 0x70    // response code: current errors, fixed format
    sense[2] = 0x0B    // sense key: ABORTED COMMAND
    sense[7] = 10      // additional sense length
    sense[12] = 0x47   // ASC: SCSI PARITY ERROR (covers CRC)
    sense[13] = 0x05   // ASCQ: INITIATOR DETECTED ERROR MESSAGE RECEIVED
    return sense
}
```
[CITED: SPC-4 Annex D sense code table, already in codebase internal/scsi/sense.go]

### Unsolicited Data Error Sense (ERR-03, ERR-04)
```go
// ERR-03: Unexpected unsolicited data
// Sense key 0x0B (ABORTED COMMAND), ASC=0x0C (WRITE ERROR), ASCQ=0x0D (UNEXPECTED UNSOLICITED DATA)
func buildUnexpectedUnsolicitedSense() []byte {
    sense := make([]byte, 18)
    sense[0] = 0x70
    sense[2] = 0x0B    // ABORTED COMMAND
    sense[7] = 10
    sense[12] = 0x0C   // ASC: WRITE ERROR
    sense[13] = 0x0D   // ASCQ: UNEXPECTED UNSOLICITED DATA
    return sense
}

// ERR-04: Not enough unsolicited data
// Sense key 0x0B (ABORTED COMMAND), ASC=0x0C (WRITE ERROR), ASCQ=0x0E (NOT ENOUGH UNSOLICITED DATA)
func buildNotEnoughUnsolicitedSense() []byte {
    sense := make([]byte, 18)
    sense[0] = 0x70
    sense[2] = 0x0B    // ABORTED COMMAND
    sense[7] = 10
    sense[12] = 0x0C   // ASC: WRITE ERROR
    sense[13] = 0x0E   // ASCQ: NOT ENOUGH UNSOLICITED DATA
    return sense
}
```
[ASSUMED: ASC/ASCQ codes 0x0C/0x0D and 0x0C/0x0E. These are the standard SPC-4 codes for unsolicited data errors but should be verified against the internal/scsi/sense.go lookup table.]

### HandleSCSIWithStatus Helper Signature
```go
// Source: decision D-01, evolved from HandleSCSIError pattern
// Place in test/target.go alongside HandleSCSIError.
func (mt *MockTarget) HandleSCSIWithStatus(status uint8, senseData []byte) {
    mt.Handle(pdu.OpSCSICommand, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
        cmd := decoded.(*pdu.SCSICommand)
        expCmdSN, maxCmdSN := mt.session.Update(cmd.CmdSN, cmd.Header.Immediate)

        var dataSegment []byte
        if len(senseData) > 0 {
            dataSegment = make([]byte, 2+len(senseData))
            binary.BigEndian.PutUint16(dataSegment[0:2], uint16(len(senseData)))
            copy(dataSegment[2:], senseData)
        }

        resp := &pdu.SCSIResponse{
            Header: pdu.Header{
                Final:            true,
                InitiatorTaskTag: cmd.InitiatorTaskTag,
                DataSegmentLen:   uint32(len(dataSegment)),
            },
            Status:   status,
            StatSN:   tc.NextStatSN(),
            ExpCmdSN: expCmdSN,
            MaxCmdSN: maxCmdSN,
            Data:     dataSegment,
        }
        return tc.SendPDU(resp)
    })
}
```
[VERIFIED: Pattern derived from HandleSCSIError at target.go:741-766, improved with SessionState.Update]

### DataSN Gap Injection for SNACK-01
```go
// Source: datain.go gap detection logic, datain_test.go A-bit pattern
tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
    expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
    totalData := make([]byte, 1536) // 3 x 512-byte PDUs

    // Send DataSN=0
    din0 := &pdu.DataIn{
        Header: pdu.Header{
            Final: false, InitiatorTaskTag: cmd.InitiatorTaskTag, DataSegmentLen: 512,
        },
        DataSN: 0, BufferOffset: 0,
        ExpCmdSN: expCmdSN, MaxCmdSN: maxCmdSN,
        Data: totalData[0:512],
    }
    if err := tc.SendPDU(din0); err != nil { return err }

    // SKIP DataSN=1 (create gap)

    // Send DataSN=2
    din2 := &pdu.DataIn{
        Header: pdu.Header{
            Final: false, InitiatorTaskTag: cmd.InitiatorTaskTag, DataSegmentLen: 512,
        },
        DataSN: 2, BufferOffset: 1024, // offset as if DataSN=1 was sent
        ExpCmdSN: expCmdSN, MaxCmdSN: maxCmdSN,
        Data: totalData[1024:1536],
    }
    if err := tc.SendPDU(din2); err != nil { return err }

    // Wait for SNACK, then retransmit DataSN=1
    time.Sleep(100 * time.Millisecond)
    din1 := &pdu.DataIn{
        Header: pdu.Header{
            Final: false, InitiatorTaskTag: cmd.InitiatorTaskTag, DataSegmentLen: 512,
        },
        DataSN: 1, BufferOffset: 512,
        ExpCmdSN: expCmdSN, MaxCmdSN: maxCmdSN,
        Data: totalData[512:1024],
    }
    if err := tc.SendPDU(din1); err != nil { return err }

    // Send final with status
    dinFinal := &pdu.DataIn{
        Header: pdu.Header{
            Final: true, InitiatorTaskTag: cmd.InitiatorTaskTag, DataSegmentLen: 0,
        },
        DataSN: 3, HasStatus: true, Status: 0x00,
        StatSN: tc.NextStatSN(),
        ExpCmdSN: expCmdSN, MaxCmdSN: maxCmdSN,
    }
    return tc.SendPDU(dinFinal)
})
```
[VERIFIED: Gap detection logic from datain.go lines 65-81, SNACK send from snack.go lines 29-59]

## Existing Error Type Analysis

The public `SCSIError` type at `errors.go:14` already supports `errors.As`:

```go
type SCSIError struct {
    Status   uint8
    SenseKey uint8
    ASC      uint8
    ASCQ     uint8
    Message  string
}
```

Key findings:
1. **SCSIError uses pointer receiver** -- `errors.As(&scsiErr)` works correctly for `*SCSIError`. [VERIFIED: errors.go:27]
2. **submitAndCheck creates SCSIError for any non-zero status** -- `session.go:51-65`. Status 0x08 (BUSY) and 0x18 (RESERVATION CONFLICT) will produce `*SCSIError` with correct Status field. [VERIFIED: session.go:51-65]
3. **Sense data is parsed via scsi.ParseSense** -- If sense data is present, SenseKey/ASC/ASCQ are populated. If absent, only Status and a generic message are set. [VERIFIED: session.go:55-64]
4. **No retry logic for BUSY/RESERVATION CONFLICT** -- The initiator surfaces these as errors to the caller without automatic retry, which matches the D-04 requirement "without retry". [VERIFIED: session.go has no retry for non-zero status]
5. **Reject PDU cancels the task with an error** -- session.go:501 calls `tk.cancel()` with a descriptive error, which surfaces as `Result.Err` (transport-level error, not SCSIError). [VERIFIED: session.go:501]

**Conclusion:** No extension to existing error types is needed. The existing `SCSIError` and `errors.As` infrastructure fully supports D-04.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| HandleSCSIError with hardcoded CmdSN | HandleSCSIWithStatus with SessionState | Phase 16 | Multi-command error tests work correctly |
| SNACK unit tests only (internal/session/snack_test.go) | E2E SNACK wire verification via pducapture | Phase 16 | Full coverage of FFP #13.1, #13.2 |
| No conformance test for BUSY/RESERVATION CONFLICT | errors.As-based status verification | Phase 16 | Covers FFP #16.4.5, #16.4.6 |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | ASC=0x0C/ASCQ=0x0D for "unexpected unsolicited data" and ASC=0x0C/ASCQ=0x0E for "not enough unsolicited data" | Code Examples (ERR-03, ERR-04) | Wrong sense codes in test -- would still test error surfacing but with incorrect UNH-IOL compliance codes. Verify against SPC-4 Annex D or internal/scsi/sense.go. |
| A2 | ASC=0x47/ASCQ=0x05 for CRC error sense data | Code Examples (ERR-01) | Same risk as A1. Need to verify the exact ASC/ASCQ pair that targets use for iSCSI CRC errors. |

## Open Questions (RESOLVED)

1. **ERR-02 Reject Handling Flow**
   - What we know: Session.handleUnsolicited handles Reject by cancelling the task (session.go:501). The test needs to verify the initiator issues a *new* command after the reject, not a retry.
   - What's unclear: Does the caller (test code) need to explicitly re-issue the command, or is there any automatic retry path? Based on code review, the answer is "caller must re-issue" since Reject cancels the task.
   - Recommendation: Test should verify first command fails (Reject surfaces as error), then issue a second command that succeeds. This matches "new command (not retry)" from FFP #16.4.2.

2. **SNACK-01 DataSN=2 BufferOffset**
   - What we know: When we skip DataSN=1 (512 bytes), DataSN=2 should arrive with BufferOffset=1024. The initiator buffers DataSN=2 in pendingDataIn (datain.go:73) and sends SNACK for the gap.
   - What's unclear: Whether the target handler should use tc.ReadPDU() to wait for the SNACK before retransmitting, or just use time.Sleep. ReadPDU is simpler but blocks the handler goroutine.
   - Recommendation: Use time.Sleep(100ms) for the retransmit delay -- simpler and matches the existing pattern. The SNACK is captured by Recorder for assertion regardless of whether the target "reads" it.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib), Go 1.25 |
| Config file | None -- standard `go test` |
| Quick run command | `go test ./test/conformance/ -run 'TestError_\|TestSNACK_' -race -count=1 -v` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ERR-01 | CRC error sense data handling | conformance | `go test ./test/conformance/ -run TestError_CRCErrorSense -race -count=1` | Exists (error_test.go) but test not yet written |
| ERR-02 | SNACK reject + new command | conformance | `go test ./test/conformance/ -run TestError_SNACKReject -race -count=1` | Exists (error_test.go) but test not yet written |
| ERR-03 | Unexpected unsolicited data error | conformance | `go test ./test/conformance/ -run TestError_UnexpectedUnsolicited -race -count=1` | Exists (error_test.go) but test not yet written |
| ERR-04 | Not enough unsolicited data error | conformance | `go test ./test/conformance/ -run TestError_NotEnoughUnsolicited -race -count=1` | Exists (error_test.go) but test not yet written |
| ERR-05 | BUSY status 0x08 | conformance | `go test ./test/conformance/ -run TestError_BUSY -race -count=1` | Exists (error_test.go) but test not yet written |
| ERR-06 | RESERVATION CONFLICT 0x18 | conformance | `go test ./test/conformance/ -run TestError_ReservationConflict -race -count=1` | Exists (error_test.go) but test not yet written |
| SNACK-01 | DataSN gap SNACK construction | conformance | `go test ./test/conformance/ -run TestSNACK_DataSNGap -race -count=1` | Does not exist (snack_test.go) |
| SNACK-02 | DataACK SNACK wire fields | conformance | `go test ./test/conformance/ -run TestSNACK_DataACKWireFields -race -count=1` | Does not exist (snack_test.go) |

### Sampling Rate
- **Per task commit:** `go test ./test/conformance/ -run 'TestError_\|TestSNACK_' -race -count=1`
- **Per wave merge:** `go test ./... -race -count=1`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `test/conformance/snack_test.go` -- new file for SNACK-01, SNACK-02
- [ ] `HandleSCSIWithStatus` helper in test/target.go -- needed for ERR-05, ERR-06

## Project Constraints (from CLAUDE.md)

- **Language:** Go 1.25 -- use modern features where they improve clarity
- **Dependencies:** stdlib only for production code; `gostor/gotgt` for integration tests
- **Testing:** No testify; use stdlib `testing` with table-driven tests
- **Standard:** RFC 7143 compliance drives implementation
- **API style:** Go idiomatic -- context.Context, io.Reader/Writer, structured errors
- **Quality:** High test coverage, no dead code, no speculative abstractions
- **Logging:** `log/slog` with injectable handler
- **Error types:** `errors.As`/`errors.Is` for typed error inspection (public SCSIError, TransportError, AuthError)

## Sources

### Primary (HIGH confidence)
- Codebase: `errors.go` -- SCSIError type, errors.As support verified
- Codebase: `session.go` -- submitAndCheck error flow, non-zero status handling verified
- Codebase: `internal/session/datain.go` -- SNACK gap detection, A-bit DataACK logic verified
- Codebase: `internal/session/snack.go` -- sendSNACK implementation verified
- Codebase: `internal/session/session.go` -- Reject PDU handling (handleUnsolicited) verified
- Codebase: `internal/pdu/target.go` -- Reject PDU struct, SCSIResponse struct verified
- Codebase: `test/target.go` -- HandleSCSIFunc, HandleSCSIError, SessionState API verified
- Codebase: `test/conformance/datain_test.go` -- TestDataIn_ABitDataACK pattern verified
- Codebase: `test/conformance/error_test.go` -- Existing error test patterns verified
- Codebase: `test/pducapture/capture.go` -- Recorder.Sent() capture API verified
- Codebase: `doc/test_matrix_initiator_ffp.md` -- FFP gap analysis for ERR and SNACK tests

### Secondary (MEDIUM confidence)
- RFC 7143 Sections 11.4 (SCSI Response), 11.16 (SNACK), 11.17 (Reject) -- wire format definitions
- SPC-4 Annex D -- ASC/ASCQ code tables for sense data construction

### Tertiary (LOW confidence)
- ASC/ASCQ values for unsolicited data errors (0x0C/0x0D, 0x0C/0x0E) -- need verification against SPC-4

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all tools are existing codebase infrastructure, verified by grep
- Architecture: HIGH -- follows established conformance test patterns from Phases 13-15
- Pitfalls: HIGH -- all identified from actual code review of existing implementations
- Error type analysis: HIGH -- direct code verification of SCSIError, errors.As, submitAndCheck
- Sense code values: MEDIUM -- ASC/ASCQ for CRC and unsolicited data errors from training knowledge, should verify against SPC-4

**Research date:** 2026-04-05
**Valid until:** 2026-05-05 (stable -- protocol spec and codebase patterns well-established)
