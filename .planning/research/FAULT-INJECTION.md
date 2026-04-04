# Fault Injection and Protocol Testing Infrastructure - Research

**Researched:** 2026-04-04
**Domain:** iSCSI protocol-level test infrastructure (MockTarget extensions, TCP proxy, connection fault injection)
**Confidence:** HIGH

## Summary

The v1.1 test suite requires a "test station" that can inject protocol-level faults at the iSCSI PDU layer: specific SCSI error status codes, DataSN gaps, Async Messages, command window control, mid-transfer connection drops, zero-length Data-In PDUs, A-bit Data-In, and delayed responses. This research evaluates three architectural approaches and recommends a strategy that maps each test category to the right tool.

**Primary recommendation:** Extend MockTarget as the primary test vehicle for all protocol-level fault injection. MockTarget already runs over real TCP (not net.Pipe), so it provides realistic framing. Use faultConn for transport-level byte/connection faults. Do NOT introduce toxiproxy or an external TCP proxy -- the overhead is unjustified when MockTarget gives full PDU-level control.

## Project Constraints (from CLAUDE.md)

- **Language:** Go 1.25
- **Dependencies:** Minimal external (Bronx Method) -- no new runtime deps for test infrastructure
- **Testing:** Must be fully testable without manual infrastructure setup
- **Standard:** RFC 7143 compliance drives everything

## Architecture Analysis

### Current Infrastructure Inventory

| Component | Location | Capability | Limitations |
|-----------|----------|-----------|-------------|
| **MockTarget** | `test/target.go` | TCP listener, handler-based PDU dispatch, login negotiation, SCSI read/write/error, NOP-Out, TMF, discovery, logout | Single handler per opcode (no per-command routing), no state tracking (CmdSN window not enforced), no async message sending, no multi-PDU DataIn sequences |
| **FaultConn** | `internal/transport/faultconn.go` | Wraps net.Conn with injectable read/write faults keyed on cumulative byte count, runtime-settable | Byte-level only -- not PDU-aware, cannot selectively corrupt/drop specific PDUs |
| **LIO E2E target** | `test/lio/` | Real kernel iSCSI target, production-grade | Cannot inject protocol errors, no programmable misbehavior, requires root |
| **ss -K** | `test/e2e/recovery_test.go` | TCP socket kill for connection drop tests | Requires root, Linux-only, coarse-grained (kills entire socket, no mid-PDU precision) |

### Three Approaches Evaluated

#### Approach 1: Extend MockTarget (RECOMMENDED)

MockTarget already listens on real TCP and dispatches by opcode. The handler signature `func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error` gives full access to:
- Raw BHS bytes (can forge any field)
- Decoded PDU struct (can inspect ITT, CmdSN, LUN, CDB opcode)
- TargetConn (can send any PDU at any time, close connection, track StatSN)

**What needs to be added to MockTarget:**

| Capability | Current State | Extension Required |
|-----------|--------------|-------------------|
| SCSI error status (BUSY, RESERVATION CONFLICT) | HandleSCSIError exists (status + sense data) | Already works -- just pass status=0x08 or 0x18 |
| CHECK CONDITION with specific sense | HandleSCSIError exists | Already works -- pass sense bytes |
| DataSN gaps in Data-In | HandleSCSIRead sends single DataIn | New: multi-PDU DataIn handler that can skip DataSN values |
| Async Messages (codes 1-4) | No sending support | New: method to send AsyncMsg on TargetConn at any time |
| Command window control | MaxCmdSN hardcoded to CmdSN+10 | New: configurable MaxCmdSN policy (zero window, window-of-1, large) |
| Mid-transfer connection drop | Can close tc.nc any time | New: handler that sends partial data then closes |
| Zero-length Data-In | Not implemented | New: DataIn with DataSegmentLen=0 |
| A-bit Data-In (trigger DataACK) | DataIn.Acknowledge field exists | New: DataIn handler that sets A-bit |
| Reject PDU (SNACK reject) | Reject PDU type exists | New: handler or helper to send Reject |
| Delayed responses | No timing control | New: handler wrapper that adds time.Sleep or synctest delay |
| Per-command routing | One handler per opcode | New: routing by CDB opcode, ITT, or call count |

**Key insight:** MockTarget handlers are Go functions -- they can contain arbitrary logic. A "handler that skips DataSN=1" is just:

```go
mt.Handle(pdu.OpSCSICommand, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
    cmd := decoded.(*pdu.SCSICommand)
    if !cmd.Read { return nil }
    
    chunk := 512
    total := int(cmd.ExpectedDataTransferLength)
    dataSN := uint32(0)
    offset := uint32(0)
    
    for offset < uint32(total) {
        if dataSN == 1 {
            dataSN++ // Skip DataSN=1 -- creates gap
            continue
        }
        end := min(int(offset)+chunk, total)
        isFinal := end == total
        din := &pdu.DataIn{
            Header:    pdu.Header{Final: isFinal, InitiatorTaskTag: cmd.InitiatorTaskTag},
            DataSN:    dataSN,
            BufferOffset: offset,
            Data:      data[offset:end],
            // ... set other fields
        }
        if isFinal {
            din.HasStatus = true
            din.Status = 0x00
            din.StatSN = tc.NextStatSN()
        }
        tc.SendPDU(din)
        dataSN++
        offset = uint32(end)
    }
    return nil
})
```

**Advantages:**
- Full PDU-level control -- can craft any valid or invalid PDU
- No external dependencies
- Runs in-process (fast, deterministic)
- Real TCP transport (exercises framing, read pump, write pump)
- Already used by 22 conformance tests
- Can use testing/synctest for timing control

**Limitations:**
- Cannot test against LIO-specific behavior (but LIO E2E already covers that)
- MockTarget login is simplified (always AuthMethod=None)

#### Approach 2: TCP Proxy (REJECTED for this use case)

A protocol-aware TCP proxy sits between the initiator and a real target, intercepting and modifying PDUs in flight.

**Why this does not fit:**
1. **Complexity without benefit:** A proxy needs full PDU parsing (already have it), connection management, and bidirectional interception. MockTarget does all this more simply because it IS the target.
2. **External dependency risk:** toxiproxy is not PDU-aware -- it operates at TCP byte level. It can delay/drop bytes but cannot selectively skip a DataSN or inject an AsyncMsg.
3. **A custom Go proxy would duplicate MockTarget.** If you write a Go proxy that parses iSCSI PDUs and modifies them, you have essentially written a programmable target -- which is what MockTarget already is.
4. **LIO cannot be programmed to misbehave.** A proxy in front of LIO would need to suppress LIO's correct responses and inject wrong ones. At that point, you do not need LIO at all.

**When a proxy IS justified:** Testing interoperability with third-party targets where you cannot control the target behavior. Not the case here.

#### Approach 3: iptables/nftables Network Faults (SUPPLEMENT ONLY)

Network-level tools (iptables, tc/netem) operate below TCP:
- `tc netem delay 100ms` -- add latency
- `iptables -A OUTPUT -p tcp --dport 3260 -m statistic --mode random --probability 0.1 -j DROP` -- random packet loss

**When useful:** Testing TCP-level resilience (retransmission, keepalive timeout, RST vs FIN). The existing `ss -K` approach for E2E connection drop tests is sufficient and simpler.

**Not useful for:** Protocol-level tests. iptables cannot selectively drop "the third Data-In PDU." It drops TCP segments, not iSCSI PDUs.

## Recommended Test Strategy

### Test Category to Tool Mapping

| Test Category | Requirements | Tool | Why |
|--------------|-------------|------|-----|
| SCSI error status codes | ERR-05, ERR-06 | MockTarget.HandleSCSIError (exists) | Direct status injection |
| CHECK CONDITION + sense data | ERR-01 | MockTarget.HandleSCSIError (exists) | Direct sense data injection |
| SNACK reject + retry | ERR-02, CMDSEQ-07 | MockTarget + Reject PDU sender | Send Reject, verify retry PDU |
| Unexpected/insufficient unsolicited data | ERR-03, ERR-04 | MockTarget custom handler | Return error status for wrong data amount |
| DataSN gap (trigger SNACK) | SNACK-01 | MockTarget multi-PDU DataIn handler | Skip DataSN values in sequence |
| A-bit DataACK | SNACK-02, DATA-07 | MockTarget DataIn with Acknowledge=true | Set A-bit, verify SNACK DataACK response |
| Async Messages (codes 1-4) | ASYNC-01 through ASYNC-04 | MockTarget + TargetConn.SendPDU(AsyncMsg) | Direct async message injection |
| Command window (zero/1/large) | CMDSEQ-04 through CMDSEQ-06, CMDSEQ-09 | MockTarget with configurable MaxCmdSN | Control ExpCmdSN/MaxCmdSN in responses |
| Connection drop mid-transfer | SESS-07, SESS-08 | MockTarget handler that closes tc.nc | Precise timing: close after N PDUs |
| Zero-length Data-In | DATA-09 | MockTarget DataIn with empty data | Send DataIn{DataSegmentLen: 0} |
| ExpStatSN gap | CMDSEQ-08 | MockTarget that skips StatSN values | Increment StatSN by 2+ |
| NOP-Out variants | SESS-03, SESS-04, SESS-05 | MockTarget HandleNOPOut (exists) + solicited NOP-In sender | Send NOP-In with TTT to trigger NOP-Out response |
| ERL 2 connection replace + task reassign | SESS-07, SESS-08 | MockTarget with connection tracking | Accept second connection after first drops |
| Wire capture + field assertion | Phase 13 (all phases) | PDU capture middleware on MockTarget | Record all PDUs exchanged, query by opcode/field |

### MockTarget Extension Design

The extensions needed fall into these categories:

**1. Stateful Session Tracking (NEW)**

MockTarget currently does not track session-level state (CmdSN window, ISID, TSIH). For command window tests, it needs:

```go
type MockSession struct {
    ExpCmdSN  uint32
    MaxCmdSN  uint32
    ISID      [6]byte
    TSIH      uint16
}
```

This does NOT need to be a full iSCSI state machine. It just needs enough state to:
- Set MaxCmdSN in responses to control the initiator's command window
- Track ExpCmdSN to verify the initiator advances CmdSN correctly
- Optionally skip StatSN values to create ExpStatSN gaps

**2. Multi-PDU Data-In Handler (NEW)**

Replace the single-PDU HandleSCSIRead with a configurable handler:

```go
type DataInConfig struct {
    Data          []byte
    ChunkSize     int       // Bytes per Data-In PDU
    SkipDataSN    []uint32  // DataSN values to skip (create gaps)
    SetABit       bool      // Set Acknowledge bit (trigger DataACK)
    ZeroLenFinal  bool      // Send zero-length final Data-In
}

func (mt *MockTarget) HandleSCSIReadMulti(lun uint64, cfg DataInConfig)
```

**3. Async Message Injection (NEW)**

Add a method to send unsolicited AsyncMsg on any active connection:

```go
func (mt *MockTarget) SendAsyncMsg(eventCode uint8, params ...uint16) error {
    // Send to first active connection (or all connections)
}
```

This is straightforward -- TargetConn.SendPDU already exists. The new method just constructs the AsyncMsg and calls SendPDU.

**4. Per-Command Handler Routing (NEW)**

Current limitation: one handler per opcode. For tests that need different behavior per command (e.g., first READ succeeds, second READ gets BUSY), add:

```go
// HandleFunc registers a handler that is called for every SCSI command.
// The handler can inspect CDB opcode, command count, etc. to decide behavior.
// Replaces single-behavior HandleSCSIRead/HandleSCSIWrite.
func (mt *MockTarget) HandleSCSIFunc(h func(tc *TargetConn, cmd *pdu.SCSICommand, callCount int) error)
```

Or simpler: use an atomic counter in the handler closure. Since handlers are plain Go functions, test authors can embed any routing logic.

**5. PDU Capture Middleware (Phase 13)**

This is the Phase 13 deliverable but relevant here. A capture layer that records all PDUs sent and received:

```go
type CapturedPDU struct {
    Direction string    // "sent" or "received"
    Opcode    pdu.OpCode
    Raw       *transport.RawPDU
    Decoded   pdu.PDU
    Time      time.Time
}

type PDUCapture struct {
    pdus []CapturedPDU
    mu   sync.Mutex
}

func (c *PDUCapture) Filter(opcode pdu.OpCode) []CapturedPDU
func (c *PDUCapture) AssertField(t *testing.T, idx int, field string, expected any)
```

This can be implemented as a wrapper around TargetConn that intercepts SendPDU/ReadPDU, or as instrumentation on the initiator side via a wrapped net.Conn.

### FaultConn Role

FaultConn remains useful for transport-level faults that MockTarget cannot simulate:
- TCP read error after N bytes (simulates network failure mid-PDU)
- TCP write error (simulates send failure)
- Combined with MockTarget: inject transport fault AFTER a specific PDU exchange

FaultConn could be enhanced to be PDU-aware:

```go
// WithReadFaultAfterPDUs returns a fault function that triggers after
// N complete PDUs have been read (counting BHS + data segment boundaries).
func WithReadFaultAfterPDUs(n int, err error) func(int64) error
```

But this is optional -- most protocol-level tests use MockTarget handlers, not FaultConn.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| TCP-level delay/jitter | Custom net.Conn wrapper with timers | `time.Sleep` in MockTarget handler (or `testing/synctest` for determinism) | Simple, deterministic, no abstraction needed |
| External TCP proxy | toxiproxy, custom proxy daemon | MockTarget (is the target AND the proxy) | Proxy adds complexity without PDU-level control |
| Kernel network manipulation | iptables/nftables rules in tests | MockTarget close/FaultConn | Requires root, non-portable, coarse-grained |
| Full iSCSI state machine in MockTarget | session.Session on the target side | Minimal state struct (CmdSN/MaxCmdSN only) | Only need enough state for the specific test scenario |

## Common Pitfalls

### Pitfall 1: Over-engineering MockTarget into a Full Target Implementation
**What goes wrong:** Adding comprehensive session state, negotiation, and error handling to MockTarget until it becomes as complex as a real target.
**Why it happens:** Each test needs "just one more field tracked."
**How to avoid:** MockTarget handlers are per-test functions. State belongs in the handler closure, not in MockTarget itself. Each test creates exactly the behavior it needs.
**Warning signs:** MockTarget grows beyond 1000 lines, has its own state machine, or needs its own tests.

### Pitfall 2: Handler Registration Order Conflicts
**What goes wrong:** Two convenience methods both call `mt.Handle(pdu.OpSCSICommand, ...)`, and the second silently overwrites the first.
**Why it happens:** HandleSCSIRead, HandleSCSIWrite, and HandleSCSIError all register on OpSCSICommand.
**How to avoid:** Use a single multiplexing handler that routes by CDB opcode and read/write flags. Or use HandleSCSIFunc which provides call-count-based routing.
**Warning signs:** Test works alone but fails when combined with other handlers.

### Pitfall 3: Race Conditions in Async Message Injection
**What goes wrong:** Test calls `mt.SendAsyncMsg()` but the initiator has not yet reached full feature phase, or the message arrives between PDU reads and gets dropped.
**Why it happens:** Timing dependency between test goroutine and initiator's read pump.
**How to avoid:** Use testing/synctest bubbles for deterministic async event delivery. Or use a sync barrier: wait for the initiator to issue a command (proving it is in FFP) before injecting the async message.
**Warning signs:** Test passes 95% of the time, flaky in CI.

### Pitfall 4: StatSN Tracking Drift
**What goes wrong:** MockTarget's NextStatSN() is called by all handlers independently, so StatSN values in responses drift from what the initiator expects.
**Why it happens:** TargetConn.statSN is shared across all handlers via atomic increment. If a handler sends multiple PDUs (multi-PDU DataIn), each call to NextStatSN advances the counter.
**How to avoid:** Only call NextStatSN() for PDUs that carry StatSN (status-bearing PDUs). Data-In without S-bit does NOT carry StatSN. Document this clearly in handler examples.
**Warning signs:** Initiator logs "unexpected StatSN" warnings.

## Code Examples

### Sending Async Message from MockTarget

```go
// SendAsyncMsg sends an Async Message PDU to the first active connection.
func (mt *MockTarget) SendAsyncMsg(eventCode uint8, statSN, expCmdSN, maxCmdSN uint32) error {
    mt.mu.Lock()
    if len(mt.conns) == 0 {
        mt.mu.Unlock()
        return fmt.Errorf("no active connections")
    }
    tc := mt.conns[len(mt.conns)-1]
    mt.mu.Unlock()

    async := &pdu.AsyncMsg{
        Header: pdu.Header{
            Final: true,
            // LUN field set to 0 for session-level events
        },
        AsyncEvent: eventCode,
        StatSN:     statSN,
        ExpCmdSN:   expCmdSN,
        MaxCmdSN:   maxCmdSN,
    }

    // For code 1 (target requests logout): Parameter1/2/3 carry Time2Wait/Time2Retain
    if eventCode == 1 {
        async.Parameter1 = 2 // Time2Wait
        async.Parameter2 = 20 // Time2Retain
    }

    return tc.SendPDU(async)
}
```

### Command Window Control via Configurable MaxCmdSN

```go
// HandleSCSIReadWithWindow registers a read handler with configurable command window.
func (mt *MockTarget) HandleSCSIReadWithWindow(lun uint64, data []byte, maxCmdSNDelta int32) {
    mt.Handle(pdu.OpSCSICommand, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
        cmd := decoded.(*pdu.SCSICommand)
        statSN := tc.NextStatSN()
        
        // maxCmdSNDelta = -1 means zero window (MaxCmdSN = ExpCmdSN - 1)
        // maxCmdSNDelta = 0 means window of 1
        // maxCmdSNDelta = 10 means window of 11
        maxCmdSN := uint32(int32(cmd.CmdSN+1) + maxCmdSNDelta)
        
        din := &pdu.DataIn{
            Header:    pdu.Header{Final: true, InitiatorTaskTag: cmd.InitiatorTaskTag},
            HasStatus: true,
            Status:    0x00,
            StatSN:    statSN,
            ExpCmdSN:  cmd.CmdSN + 1,
            MaxCmdSN:  maxCmdSN,
            DataSN:    0,
            Data:      data,
        }
        return tc.SendPDU(din)
    })
}
```

### Multi-PDU Data-In with DataSN Gap

```go
func handleReadWithDataSNGap(tc *TargetConn, cmd *pdu.SCSICommand, data []byte, skipSN uint32) error {
    chunkSize := 512
    total := len(data)
    dataSN := uint32(0)
    offset := 0

    for offset < total {
        if dataSN == skipSN {
            dataSN++ // Skip this DataSN -- initiator should send SNACK
            continue
        }
        
        end := offset + chunkSize
        if end > total {
            end = total
        }
        isFinal := end == total

        din := &pdu.DataIn{
            Header: pdu.Header{
                Final:            isFinal,
                InitiatorTaskTag: cmd.InitiatorTaskTag,
                DataSegmentLen:   uint32(end - offset),
            },
            DataSN:       dataSN,
            BufferOffset: uint32(offset),
            Data:         data[offset:end],
        }

        if isFinal {
            din.HasStatus = true
            din.Status = 0x00
            din.StatSN = tc.NextStatSN()
            din.ExpCmdSN = cmd.CmdSN + 1
            din.MaxCmdSN = cmd.CmdSN + 10
        }

        if err := tc.SendPDU(din); err != nil {
            return err
        }

        dataSN++
        offset = end
    }
    return nil
}
```

### Reject PDU for SNACK Reject Tests

```go
func sendReject(tc *TargetConn, reason uint8, rejectedBHS [48]byte, cmdSN uint32) error {
    reject := &pdu.Reject{
        Header: pdu.Header{
            Final:          true,
            DataSegmentLen: 48, // BHS of rejected PDU
        },
        Reason:   reason,
        StatSN:   tc.NextStatSN(),
        ExpCmdSN: cmdSN,
        MaxCmdSN: cmdSN + 10,
        Data:     rejectedBHS[:],
    }
    return tc.SendPDU(reject)
}
```

## Open Questions

1. **Per-connection vs. per-session handler state**
   - What we know: TargetConn tracks StatSN per connection. MockTarget registers handlers globally.
   - What is unclear: For ERL 2 tests, the second connection needs different login behavior (TSIH echoed, task reassign). Should handlers be per-connection?
   - Recommendation: Add optional per-connection handler override. The ERL 2 test registers a second-connection-specific login handler after the first connection drops.

2. **PDU capture placement: initiator-side or target-side?**
   - What we know: Capturing on the target side (in TargetConn) shows what the initiator sent and what the target replied. Capturing on the initiator side (via wrapped net.Conn) shows the same but from the initiator's perspective.
   - What is unclear: Which gives more useful assertions?
   - Recommendation: Both. Target-side capture is simpler (instrument TargetConn.SendRaw and serveConn read loop). Initiator-side capture requires a net.Conn wrapper but tests exactly what the initiator sees. Phase 13 should decide.

3. **synctest integration with MockTarget**
   - What we know: testing/synctest virtualizes time in "bubbles." MockTarget runs goroutines (acceptLoop, serveConn).
   - What is unclear: Whether synctest bubbles correctly capture MockTarget goroutines spawned inside the bubble.
   - Recommendation: Test this early in Phase 13. If synctest works with MockTarget, use it for all timing-sensitive tests (delayed responses, timeout verification, async message timing). If not, fall back to real-time with generous timeouts.

## Sources

### Primary (HIGH confidence)
- `test/target.go` -- MockTarget implementation, 710 lines, handler-based architecture
- `internal/transport/faultconn.go` -- FaultConn implementation, byte-level fault injection
- `internal/pdu/target.go` -- All target PDU types including AsyncMsg, Reject, DataIn with A-bit
- `internal/session/async.go` -- Initiator's async message handling (EventCode 0-4)
- `test/e2e/recovery_test.go` -- Existing ss -K approach for connection drop E2E

### Secondary (MEDIUM confidence)
- [Shopify/toxiproxy](https://github.com/Shopify/toxiproxy) -- Go TCP proxy for chaos testing; evaluated and rejected for this use case (not PDU-aware)
- RFC 7143 Sections 11.7 (DataIn), 11.9 (AsyncMsg), 11.17 (Reject), 3.2.2 (CmdSN windowing) -- protocol requirements driving test design

## Metadata

**Confidence breakdown:**
- MockTarget extension approach: HIGH -- based on direct code inspection, architecture is clearly extensible
- FaultConn role: HIGH -- well-understood byte-level tool, clear boundary with MockTarget
- TCP proxy rejection: HIGH -- clear analysis of why PDU-level control trumps byte-level proxy
- synctest integration: MEDIUM -- untested with MockTarget's goroutine model, flagged as open question

**Research date:** 2026-04-04
**Valid until:** 2026-05-04 (stable -- no external dependency changes expected)
