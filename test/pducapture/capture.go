// Package pducapture provides a PDU capture framework for iSCSI test assertions.
// It records every PDU sent and received during a session via WithPDUHook,
// decoding raw bytes into typed pdu.PDU structs for field-level inspection.
package pducapture

import (
	"context"
	"sync"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/internal/pdu"
)

// CapturedPDU represents a single captured PDU with direction, decoded type,
// raw bytes, and a monotonic sequence number.
type CapturedPDU struct {
	Direction uiscsi.PDUDirection
	Decoded   pdu.PDU
	Raw       []byte // defensive copy of BHS + DataSegment
	Seq       int    // monotonically increasing sequence number
}

// Recorder captures PDUs via the WithPDUHook callback.
// It is safe for concurrent use.
type Recorder struct {
	mu   sync.Mutex
	pdus []CapturedPDU
	seq  int
}

// Hook returns a closure compatible with uiscsi.WithPDUHook. The closure
// decodes each PDU's BHS into a typed pdu.PDU and appends it to the recorder.
// PDUs shorter than BHSLength (48 bytes) or with unrecognizable opcodes are
// silently skipped.
func (r *Recorder) Hook() func(context.Context, uiscsi.PDUDirection, []byte) {
	return func(_ context.Context, dir uiscsi.PDUDirection, data []byte) {
		if len(data) < pdu.BHSLength {
			return
		}

		var bhs [pdu.BHSLength]byte
		copy(bhs[:], data[:pdu.BHSLength])

		decoded, err := pdu.DecodeBHS(bhs)
		if err != nil {
			return
		}

		raw := make([]byte, len(data))
		copy(raw, data)

		r.mu.Lock()
		r.pdus = append(r.pdus, CapturedPDU{
			Direction: dir,
			Decoded:   decoded,
			Raw:       raw,
			Seq:       r.seq,
		})
		r.seq++
		r.mu.Unlock()
	}
}

// All returns a snapshot copy of all captured PDUs.
func (r *Recorder) All() []CapturedPDU {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]CapturedPDU, len(r.pdus))
	copy(result, r.pdus)
	return result
}

// Filter returns captured PDUs matching both the given opcode and direction.
func (r *Recorder) Filter(opcode pdu.OpCode, dir uiscsi.PDUDirection) []CapturedPDU {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []CapturedPDU
	for _, p := range r.pdus {
		if p.Decoded.Opcode() == opcode && p.Direction == dir {
			result = append(result, p)
		}
	}
	return result
}

// Sent returns captured PDUs matching the given opcode that were sent by the initiator.
func (r *Recorder) Sent(opcode pdu.OpCode) []CapturedPDU {
	return r.Filter(opcode, uiscsi.PDUSend)
}

// Received returns captured PDUs matching the given opcode that were received from the target.
func (r *Recorder) Received(opcode pdu.OpCode) []CapturedPDU {
	return r.Filter(opcode, uiscsi.PDUReceive)
}
