# Phase 14: Data Transfer and R2T Wire Validation - Research

**Researched:** 2026-04-05
**Domain:** iSCSI Data-Out / Data-In / R2T wire-level conformance testing
**Confidence:** HIGH

## Summary

Phase 14 is the largest single phase in v1.1, covering 18 requirements (DATA-01 through DATA-14 and R2T-01 through R2T-04) that validate every wire-level field in Data-Out, Data-In, and R2T PDUs. The implementation strategy extends the MockTarget from Phase 13 with (a) configurable login negotiation parameters on the target side, (b) multi-PDU Data-In helpers, (c) R2T sequence generation helpers, and (d) a Data-Out receive handler so the target can participate in solicited write flows.

The existing codebase already has the data transfer implementation (dataout.go, datain.go, snack.go) and comprehensive unit tests at the internal/session level. Phase 14 elevates these to E2E wire conformance tests using pducapture.Recorder to assert PDU field values on the wire. The key infrastructure gap is that MockTarget cannot currently (1) receive Data-Out PDUs from the initiator during write sequences, or (2) control what negotiation parameter values the target offers during login. Both must be added before conformance tests can exercise write paths.

**Primary recommendation:** Build MockTarget extensions in the first plan (negotiation parameter config, Data-Out handler, multi-PDU Data-In helpers, R2T sequence helpers), then layer conformance tests across three files in subsequent plans.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Hybrid approach -- dedicated helpers for common multi-PDU Data-In responses and R2T sequences (e.g., `HandleSCSIReadMultiPDU`, R2T sequence generation), plus `HandleSCSIFunc` for fault injection and edge cases. Helpers handle correct DataSN/offset/F-bit construction; `HandleSCSIFunc` gives full manual control when tests need wrong values.
- **D-02:** This implements the deferred Phase 13 D-06 item 2 (multi-PDU Data-In with configurable DataSN gaps).
- **D-03:** Three test files in `test/conformance/`: `dataout_test.go` (DATA-01 through DATA-05, DATA-08, DATA-10, DATA-11, DATA-12, DATA-13), `datain_test.go` (DATA-06, DATA-07, DATA-09, DATA-14), `r2t_test.go` (R2T-01 through R2T-04). Mirrors the `internal/session/dataout.go` / `datain.go` split.
- **D-04:** MockTarget-side configuration -- add a method to control what parameter values the target offers during login negotiation (ImmediateData, InitialR2T, FirstBurstLength, MaxBurstLength, MaxRecvDataSegmentLength). The initiator negotiates normally against the target's offers. Tests control outcomes by controlling the target side.
- **D-05:** DATA-07 (A-bit SNACK DataACK at ERL>=1) stays in Phase 14 in `datain_test.go`. Whatever MockTarget support is needed for A-bit injection and SNACK DataACK verification is built in this phase, not deferred to Phase 16.

### Claude's Discretion
- Exact API shape of multi-PDU Data-In and R2T helpers
- Exact method name/signature for MockTarget negotiation parameter config
- How DATA requirements map to individual test functions vs subtests within the three files
- Plan count and task breakdown across the 18 requirements

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DATA-01 | Data-Out DataSN starts at 0 and increments per R2T sequence (FFP #6.1) | MockTarget R2T generation + pducapture Data-Out filter; verify DataSN field per burst |
| DATA-02 | Unsolicited data respects FirstBurstLength with solicited R2T follow-up (FFP #8.1) | Target negotiation param config (D-04) to set FirstBurstLength; capture total unsolicited bytes |
| DATA-03 | No unsolicited data in R2T-only mode (FFP #8.2) | Target sets InitialR2T=Yes, ImmediateData=No; verify no Data-Out before R2T |
| DATA-04 | Unsolicited Data-Out PDUs respect FirstBurstLength (FFP #8.3) | Target sets small FirstBurstLength; sum Data-Out segment lengths |
| DATA-05 | Data-Out echoes Target Transfer Tag from R2T (FFP #9.1) | Target sends R2T with known TTT; capture Data-Out TTT field |
| DATA-06 | Status accepted in final Data-In PDU with S+F bits (FFP #10.1) | Multi-PDU Data-In helper with HasStatus+Final on last PDU |
| DATA-07 | Data-In A-bit triggers SNACK DataACK at ERL>=1 (FFP #10.2) | Target sends Data-In with A-bit; capture SNACK Request with Type=DataACK |
| DATA-08 | Data-Out respects target MaxRecvDataSegmentLength (FFP #11.1.1) | Target declares small MaxRecvDSL during login; verify each Data-Out DSL |
| DATA-09 | Initiator accepts Data-In with DataSegmentLength=0 (FFP #11.1.2) | Target sends zero-length Data-In PDU; verify no error |
| DATA-10 | F bit set on last unsolicited Data-Out PDU (FFP #11.2.1) | Capture unsolicited Data-Out sequence; verify Final only on last |
| DATA-11 | F bit set on last solicited Data-Out PDU (FFP #11.2.2) | Capture solicited Data-Out burst; verify Final only on last |
| DATA-12 | DataSN per R2T sequence in Data-Out (FFP #11.3) | Multiple R2T responses; verify DataSN resets per-burst |
| DATA-13 | Buffer Offset increases correctly in Data-Out (FFP #11.4) | Multi-PDU Data-Out burst; verify offset = previous_offset + previous_DSL |
| DATA-14 | Expected Data Transfer Length matches actual transfer (FFP #16.6) | Multi-PDU Data-In; sum all segment lengths and compare to EDTL |
| R2T-01 | Single Data-Out response to R2T with correct TTT, offset, length (FFP #12.1) | R2T with DesiredDTL fitting one PDU; verify single Data-Out fields |
| R2T-02 | Multi-PDU response to R2T with F bit and continuous offsets (FFP #12.2) | R2T with large DesiredDTL; verify PDU chain |
| R2T-03 | R2T fulfillment order when DataSequenceInOrder=No (FFP #12.3) | Multiple R2Ts for same command; verify Data-Out ordering |
| R2T-04 | Parallel command R2T fulfillment ordering (FFP #12.4) | Two concurrent commands with R2Ts; verify per-command correctness |
</phase_requirements>

## Architecture Patterns

### MockTarget Extensions Required

Four categories of MockTarget extensions are needed, all in `test/target.go`:

#### 1. Negotiation Parameter Configuration (D-04)

The current `HandleLogin` echoes back whatever the initiator proposes (operational phase handler at line 389-428). For conformance tests, the target needs to control the resolved values.

**Pattern:** Add a `NegotiationConfig` struct and setter method on MockTarget. The login handler reads from this config instead of echoing.

```go
// Source: codebase analysis of test/target.go HandleLogin
// NegotiationConfig controls what the target offers during login.
type NegotiationConfig struct {
    ImmediateData            *bool   // nil = echo initiator
    InitialR2T               *bool
    FirstBurstLength         *uint32
    MaxBurstLength           *uint32
    MaxRecvDataSegmentLength *uint32
    ErrorRecoveryLevel       *uint32
}

func (mt *MockTarget) SetNegotiationConfig(cfg NegotiationConfig)
```

The login handler's operational phase (case 1) must be modified to check `NegotiationConfig` fields and override echoed values with target-configured values when non-nil. [VERIFIED: codebase analysis of test/target.go lines 389-428]

#### 2. Data-Out Receive Handler

MockTarget currently has **no handler for `OpDataOut`** -- the initiator sends Data-Out PDUs but the target ignores them (unhandled opcode path). For write conformance tests, the target needs to:
- Receive Data-Out PDUs after sending R2T
- Track received data for verification
- Send SCSIResponse after all data received

**Pattern:** Register an `OpDataOut` handler that accumulates received data and completes when F-bit is seen on the expected final burst. [VERIFIED: grep confirms no OpDataOut handler in test/]

#### 3. Multi-PDU Data-In Helpers (D-01)

```go
// Source: codebase analysis of test/target.go HandleSCSIRead pattern
// HandleSCSIReadMultiPDU sends read data in multiple Data-In PDUs.
func (mt *MockTarget) HandleSCSIReadMultiPDU(lun uint64, data []byte, pduSize int)
```

Must correctly set: DataSN (incrementing from 0), BufferOffset (cumulative), Final+HasStatus only on last PDU, StatSN only on status-bearing PDU. [VERIFIED: pdu.DataIn struct in internal/pdu/target.go lines 231-292]

#### 4. R2T Sequence Helpers

```go
// Source: codebase analysis of pdu.R2T struct in internal/pdu/target.go
// SendR2TSequence sends one or more R2Ts for a write command.
// Each R2T carries a unique TTT, incrementing R2TSN, and correct BufferOffset.
func SendR2TSequence(tc *TargetConn, itt uint32, startOffset uint32,
    totalLen uint32, burstLen uint32, session *SessionState) []uint32
```

Returns TTT values so the caller can verify Data-Out echoes them. [VERIFIED: pdu.R2T fields in target.go lines 330-367]

### Recommended Test Structure

```
test/conformance/
    dataout_test.go    # DATA-01,02,03,04,05,08,10,11,12,13 (write-path tests)
    datain_test.go     # DATA-06,07,09,14 (read-path tests)
    r2t_test.go        # R2T-01,02,03,04 (R2T fulfillment tests)
```

Each test follows the Phase 13 established pattern:
1. Create `pducapture.Recorder`
2. Create MockTarget with appropriate handlers and negotiation config
3. `uiscsi.Dial` with `WithPDUHook(rec.Hook())` and optionally `WithOperationalOverrides`
4. Execute operation (read or write)
5. Filter captured PDUs by opcode + direction
6. Assert field values with `t.Errorf` (not `t.Fatal` for field assertions)

[VERIFIED: test/conformance/cmdseq_test.go pattern]

### Initiator-Side Parameter Control

For write-path tests that need specific ImmediateData/InitialR2T combinations, the test must control BOTH sides:
- **Target side (D-04):** `SetNegotiationConfig` to offer the desired values
- **Initiator side:** `WithOperationalOverrides` to propose matching values

Both are needed because iSCSI login negotiation is bilateral -- the resolved value depends on both sides. For boolean params like ImmediateData and InitialR2T, the result is AND of both sides (per RFC 7143 Section 13.3). [ASSUMED -- RFC 7143 AND-semantics for boolean keys]

### DataACK / A-bit Pattern (DATA-07)

For DATA-07, the target sends a Data-In with `Acknowledge=true` (A-bit). The initiator at ERL>=1 must respond with a SNACK of Type=DataACK (type 2). The test needs:
1. Target configured with ERL=1 via negotiation
2. Multi-PDU Data-In with A-bit set on intermediate PDU
3. Capture SNACK Request with `Type == SNACKTypeDataACK` (value 2)
4. Verify SNACK carries correct ExpDataSN/BegRun

The existing `snack.go` implementation handles gap-based SNACK (Type 0) and Status SNACK (Type 1). The DataACK path (Type 2) responds to A-bit, which is handled in `datain.go` handleDataIn -- but only if ERL>=1. Need to verify this code path exists. [VERIFIED: SNACKTypeDataACK = 2 in session/types.go:99]

### Parallel Command Testing (R2T-04)

R2T-04 requires two concurrent write commands, each receiving R2Ts, and verifying that Data-Out for each command carries the correct ITT and TTT. Pattern:
1. Submit two write commands concurrently (goroutines or sequential Submit)
2. Target sends R2Ts for both with distinct TTTs
3. Capture all Data-Out PDUs, group by ITT, verify per-command field correctness

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PDU field extraction | Manual byte parsing | `pdu.DecodeBHS` + typed PDU structs | Already available, type-safe, tested |
| Wire capture | Custom net.Conn wrapper | `pducapture.Recorder` + `WithPDUHook` | Phase 13 infrastructure, proven pattern |
| Login parameter negotiation | Custom login mock | Extend existing `HandleLogin` with `NegotiationConfig` | Reuse existing login flow, just parameterize it |
| R2T burst calculation | Manual offset math | Dedicated helper function | Offset/DataSN/TTT calculation is error-prone, centralize it |

## Common Pitfalls

### Pitfall 1: DataSN Scope Confusion
**What goes wrong:** Assuming DataSN is global per-session when it is per-R2T-sequence (per burst).
**Why it happens:** RFC 7143 Section 11.7.4 states DataSN starts at 0 for each R2T sequence, not for the command or session.
**How to avoid:** Each burst (sendDataOutBurst) already resets dataSN=0. Tests must verify this explicitly for multi-R2T scenarios (DATA-12).
**Warning signs:** DataSN incrementing across bursts instead of resetting.

### Pitfall 2: Bilateral Negotiation Parameters
**What goes wrong:** Configuring only the target side for boolean params, but the initiator proposes a different value, so the AND-resolution produces the wrong result.
**Why it happens:** Boolean keys like ImmediateData and InitialR2T resolve to the AND of both proposals (RFC 7143 Section 13.3). If the target offers ImmediateData=No but the initiator proposes ImmediateData=Yes, the result is No (correct). But if both default to Yes and you only change one, you need to change both.
**How to avoid:** Tests that need ImmediateData=No must set it on BOTH target (NegotiationConfig) AND initiator (WithOperationalOverrides). Tests that need ImmediateData=Yes can rely on both defaults.
**Warning signs:** Test works for "No" values but fails for certain "Yes" combinations.

### Pitfall 3: MockTarget Data-Out Race
**What goes wrong:** Target sends R2T, then immediately reads for Data-Out, but the initiator hasn't processed the R2T yet.
**Why it happens:** The target-side handler runs in the serveConn goroutine, which must yield to allow the initiator's R2T dispatch loop to process and send Data-Out.
**How to avoid:** Use `HandleSCSIFunc` where the handler sends R2T then reads Data-Out from the same connection in sequence. Or use a channel-based synchronization pattern.
**Warning signs:** Intermittent test timeouts or wrong PDU reads.

### Pitfall 4: pducapture Records Initiator-Side Only
**What goes wrong:** Trying to capture target-sent R2T PDUs with pducapture, but the hook only sees PDUs from the initiator's perspective (Sent = initiator originated, Received = initiator received).
**Why it happens:** WithPDUHook is on the initiator session, not the target.
**How to avoid:** To verify Data-Out fields, use `rec.Sent(pdu.OpDataOut)`. To verify the initiator received R2T correctly, use `rec.Received(pdu.OpR2T)`. You cannot capture what the target sends from the target side via pducapture.
**Warning signs:** Empty capture results when filtering for target opcodes with Sent direction.

### Pitfall 5: Immediate Data Not Captured by pducapture
**What goes wrong:** SCSI Command PDUs carry immediate data in the DataSegment, which is part of the same PDU. The DataSegmentLength of the command includes the immediate data.
**Why it happens:** Immediate data is encoded in the SCSI Command PDU, not a separate Data-Out PDU. Tests checking "total data sent" must include immediate data from the command PDU.
**How to avoid:** When verifying FirstBurstLength compliance (DATA-02, DATA-04), sum: (1) immediate data from SCSI Command DataSegmentLength, plus (2) all unsolicited Data-Out DataSegmentLength values.
**Warning signs:** Total sent bytes off by the immediate data amount.

## Code Examples

### Pattern 1: Write Conformance Test with R2T
```go
// Source: established pattern from test/conformance/cmdseq_test.go + test/target.go
func TestDataOut_R2TFulfillment(t *testing.T) {
    rec := &pducapture.Recorder{}

    tgt, err := testutil.NewMockTarget()
    if err != nil { t.Fatalf("NewMockTarget: %v", err) }
    t.Cleanup(func() { tgt.Close() })

    tgt.SetNegotiationConfig(testutil.NegotiationConfig{
        ImmediateData: ptr(false),
        InitialR2T:    ptr(true),
    })
    tgt.HandleLogin()
    tgt.HandleLogout()
    tgt.HandleNOPOut()

    // HandleSCSIFunc sends R2T and collects Data-Out
    tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, n int) error {
        expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
        // Send R2T for full data
        r2t := &pdu.R2T{
            Header:                   pdu.Header{InitiatorTaskTag: cmd.InitiatorTaskTag, Final: true},
            TargetTransferTag:        0x1234,
            StatSN:                   tc.NextStatSN(),
            ExpCmdSN:                 expCmdSN,
            MaxCmdSN:                 maxCmdSN,
            R2TSN:                    0,
            BufferOffset:             0,
            DesiredDataTransferLength: cmd.ExpectedDataTransferLength,
        }
        if err := tc.SendPDU(r2t); err != nil { return err }

        // Read Data-Out PDUs until Final bit
        for {
            raw, err := transport.ReadRawPDU(tc.nc, false, false, 0)
            if err != nil { return err }
            // ... accumulate and check F-bit
        }

        // Send SCSIResponse
        resp := &pdu.SCSIResponse{...}
        return tc.SendPDU(resp)
    })

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    sess, err := uiscsi.Dial(ctx, tgt.Addr(),
        uiscsi.WithPDUHook(rec.Hook()),
        uiscsi.WithOperationalOverrides(map[string]string{
            "ImmediateData": "No",
            "InitialR2T":    "Yes",
        }),
    )
    // ... execute write, then assert captured Data-Out fields
}
```

### Pattern 2: Multi-PDU Data-In Read Test
```go
// Source: established pattern from test/target.go HandleSCSIRead
// Target handler that sends data in multiple Data-In PDUs
tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, n int) error {
    expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
    edtl := cmd.ExpectedDataTransferLength
    pduSize := uint32(512) // each PDU carries 512 bytes
    var offset, dataSN uint32

    for offset < edtl {
        chunk := min(pduSize, edtl-offset)
        isFinal := offset+chunk >= edtl

        din := &pdu.DataIn{
            Header: pdu.Header{
                Final:            isFinal,
                InitiatorTaskTag: cmd.InitiatorTaskTag,
                DataSegmentLen:   chunk,
            },
            DataSN:       dataSN,
            BufferOffset: offset,
            ExpCmdSN:     expCmdSN,
            MaxCmdSN:     maxCmdSN,
            Data:         data[offset : offset+chunk],
        }
        if isFinal {
            din.HasStatus = true
            din.Status = 0x00
            din.StatSN = tc.NextStatSN()
        }
        if err := tc.SendPDU(din); err != nil { return err }
        offset += chunk
        dataSN++
    }
    return nil
})
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib), Go 1.25 |
| Config file | None needed -- `go test` with package path |
| Quick run command | `go test ./test/conformance/ -run TestDataOut -count=1 -timeout 30s` |
| Full suite command | `go test ./test/conformance/ -count=1 -timeout 120s -race` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DATA-01 | DataSN starts 0, increments per R2T | E2E conformance | `go test ./test/conformance/ -run TestDataOut_DataSN -count=1` | Wave 0 |
| DATA-02 | Unsolicited respects FirstBurstLength + R2T followup | E2E conformance | `go test ./test/conformance/ -run TestDataOut_UnsolicitedFirstBurst -count=1` | Wave 0 |
| DATA-03 | No unsolicited in R2T-only mode | E2E conformance | `go test ./test/conformance/ -run TestDataOut_NoUnsolicited -count=1` | Wave 0 |
| DATA-04 | Unsolicited respects FirstBurstLength | E2E conformance | `go test ./test/conformance/ -run TestDataOut_FirstBurstLimit -count=1` | Wave 0 |
| DATA-05 | TTT echoed from R2T | E2E conformance | `go test ./test/conformance/ -run TestDataOut_TTTEcho -count=1` | Wave 0 |
| DATA-06 | S+F status in final Data-In | E2E conformance | `go test ./test/conformance/ -run TestDataIn_StatusInFinal -count=1` | Wave 0 |
| DATA-07 | A-bit triggers DataACK SNACK | E2E conformance | `go test ./test/conformance/ -run TestDataIn_ABitDataACK -count=1` | Wave 0 |
| DATA-08 | Data-Out respects MaxRecvDSL | E2E conformance | `go test ./test/conformance/ -run TestDataOut_MaxRecvDSL -count=1` | Wave 0 |
| DATA-09 | Accept zero-length Data-In | E2E conformance | `go test ./test/conformance/ -run TestDataIn_ZeroLength -count=1` | Wave 0 |
| DATA-10 | F-bit on last unsolicited | E2E conformance | `go test ./test/conformance/ -run TestDataOut_FBitUnsolicited -count=1` | Wave 0 |
| DATA-11 | F-bit on last solicited | E2E conformance | `go test ./test/conformance/ -run TestDataOut_FBitSolicited -count=1` | Wave 0 |
| DATA-12 | DataSN per R2T sequence | E2E conformance | `go test ./test/conformance/ -run TestDataOut_DataSNPerR2T -count=1` | Wave 0 |
| DATA-13 | BufferOffset increases | E2E conformance | `go test ./test/conformance/ -run TestDataOut_BufferOffset -count=1` | Wave 0 |
| DATA-14 | EDTL matches actual transfer | E2E conformance | `go test ./test/conformance/ -run TestDataIn_EDTL -count=1` | Wave 0 |
| R2T-01 | Single PDU R2T response | E2E conformance | `go test ./test/conformance/ -run TestR2T_SinglePDU -count=1` | Wave 0 |
| R2T-02 | Multi-PDU R2T response | E2E conformance | `go test ./test/conformance/ -run TestR2T_MultiPDU -count=1` | Wave 0 |
| R2T-03 | Out-of-order R2T fulfillment | E2E conformance | `go test ./test/conformance/ -run TestR2T_MultipleR2T -count=1` | Wave 0 |
| R2T-04 | Parallel command R2T | E2E conformance | `go test ./test/conformance/ -run TestR2T_ParallelCommand -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./test/conformance/ -run "TestDataOut|TestDataIn|TestR2T" -count=1 -timeout 30s`
- **Per wave merge:** `go test ./test/conformance/ -count=1 -timeout 120s -race`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `test/conformance/dataout_test.go` -- covers DATA-01 through DATA-05, DATA-08, DATA-10 through DATA-13
- [ ] `test/conformance/datain_test.go` -- covers DATA-06, DATA-07, DATA-09, DATA-14
- [ ] `test/conformance/r2t_test.go` -- covers R2T-01 through R2T-04
- [ ] MockTarget NegotiationConfig + Data-Out handler in `test/target.go`

## Standard Stack

No new dependencies. All work uses existing stdlib + project codebase:

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `testing` | stdlib (Go 1.25) | Test framework | Project constraint -- no testify |
| `pducapture` | project internal | PDU capture + filter | Phase 13 infrastructure |
| `test` (MockTarget) | project internal | In-process iSCSI target | Phase 13 infrastructure |
| `pdu` | project internal | PDU types + marshal | Core PDU library |

[VERIFIED: CLAUDE.md constraints -- stdlib only, no testify]

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Boolean iSCSI keys resolve via AND of both proposals (RFC 7143 Section 13.3) | Pitfall 2 | Tests with ImmediateData/InitialR2T combinations may produce unexpected values; could be mitigated by verifying resolved params in the login response |
| A2 | MockTarget serveConn can read Data-Out PDUs inline within HandleSCSIFunc callback | Architecture Pattern 2, Pitfall 3 | If serveConn dispatches asynchronously, inline reads will get wrong PDUs; need to verify dispatch is synchronous per-connection |

**A2 risk assessment:** The MockTarget `serveConn` loop (target.go line 238) reads PDUs sequentially and dispatches to the handler. Since Data-Out PDUs arrive on the same connection after the SCSI Command, the handler can perform additional reads from `tc.nc` to consume Data-Out PDUs inline. This is confirmed by the existing unit test pattern in `internal/session/dataout_test.go` which uses `readDataOutPDU(t, targetConn)` to read Data-Out from the net.Pipe. However, in MockTarget the serveConn loop owns the read side, so the handler must NOT return until all Data-Out PDUs are consumed -- otherwise serveConn will try to decode Data-Out as a new PDU dispatch. This means write-path handlers in HandleSCSIFunc must be **blocking** until the write sequence completes. [VERIFIED: test/target.go serveConn lines 238-276]

## Open Questions (RESOLVED)

1. **A-bit DataACK code path in initiator**
   - What we know: `snack.go` has SNACKTypeDataACK=2, `datain.go` handleDataIn resets SNACK timer on each Data-In at ERL>=1, gap detection sends SNACKTypeDataR2T
   - What's unclear: Whether the initiator currently sends DataACK SNACK when it sees A-bit=true on a received Data-In. The handleDataIn code checks for DataSN gaps but does not appear to check `din.Acknowledge` (A-bit). This may need implementation, not just testing.
   - Recommendation: During plan execution, verify if `handleDataIn` responds to A-bit. If not, add DataACK SNACK send when A-bit is seen. This is implementation work within the test phase, but it is small and clearly scoped to DATA-07.
   - RESOLVED: Plan 14-03, Task 1 addresses this. The task action includes a pre-check of datain.go for the A-bit code path and adds it if missing (5-10 lines in handleDataIn). internal/session/datain.go is listed in 14-03 files_modified.

2. **TargetConn.nc access for inline Data-Out reads**
   - What we know: `TargetConn.nc` is the private `net.Conn` field
   - What's unclear: Whether HandleSCSIFunc handlers can access `tc.nc` directly to read Data-Out PDUs inline, or if a helper method is needed
   - Recommendation: Add `TargetConn.ReadPDU()` method that wraps `transport.ReadRawPDU` with the connection, or expose a public accessor. Simpler than exposing nc directly.
   - RESOLVED: Plan 14-01, Task 1 adds `TargetConn.ReadPDU()` method wrapping `transport.ReadRawPDU`. Plan 14-01, Task 2 adds `ReadDataOutPDUs()` helper that uses ReadPDU to collect Data-Out PDUs until F-bit.

## Sources

### Primary (HIGH confidence)
- Codebase analysis: `test/target.go` -- MockTarget structure, HandleLogin, handler dispatch
- Codebase analysis: `internal/session/dataout.go` -- sendDataOutBurst, handleR2T, sendUnsolicitedDataOut
- Codebase analysis: `internal/session/datain.go` -- handleDataIn with DataSN/offset validation, SNACK recovery
- Codebase analysis: `internal/pdu/initiator.go` -- DataOut PDU struct and wire format
- Codebase analysis: `internal/pdu/target.go` -- DataIn, R2T PDU structs and wire formats
- Codebase analysis: `test/conformance/cmdseq_test.go` -- Phase 13 established test pattern
- Codebase analysis: `internal/session/snack.go` -- SNACK types, DataACK constant
- Codebase analysis: `internal/login/params.go` -- NegotiatedParams with all operational parameters

### Secondary (MEDIUM confidence)
- Phase 14 CONTEXT.md -- locked decisions D-01 through D-05

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all stdlib + existing project code
- Architecture: HIGH -- clear codebase analysis, established patterns from Phase 13
- Pitfalls: HIGH -- verified against actual code paths and wire formats
- MockTarget extensions: MEDIUM -- A-bit/DataACK path may need initiator implementation, not just testing

**Research date:** 2026-04-05
**Valid until:** 2026-05-05 (stable -- iSCSI protocol does not change)
