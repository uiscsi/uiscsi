package transport

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"

	"github.com/uiscsi/uiscsi/internal/pdu"
)

// PDU hook direction constants. These live in the transport package to
// avoid a circular dependency between transport and session.
const (
	HookSend    uint8 = 0
	HookReceive uint8 = 1
)

// WritePump owns all writes to the underlying connection. It receives RawPDUs
// from writeCh and serializes them to w one at a time, preventing TCP byte
// interleaving (Pitfall 7). Returns when ctx is cancelled or writeCh is closed.
func WritePump(ctx context.Context, w io.Writer, writeCh <-chan *RawPDU,
	logger *slog.Logger, pduHook func(uint8, *RawPDU), digestByteOrder binary.ByteOrder) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-writeCh:
			if !ok {
				return nil // channel closed
			}
			if pduHook != nil {
				pduHook(HookSend, p)
			}
			if logger.Enabled(ctx, slog.LevelDebug) {
				opcode := pdu.OpCode(p.BHS[0] & 0x3f)
				itt := binary.BigEndian.Uint32(p.BHS[16:20])
				cmdSN := binary.BigEndian.Uint32(p.BHS[24:28])
				dsLen := uint32(p.BHS[5])<<16 | uint32(p.BHS[6])<<8 | uint32(p.BHS[7])
				logger.DebugContext(ctx, "pdu sent",
					"opcode", opcode.String(),
					"itt", fmt.Sprintf("0x%08x", itt),
					"cmd_sn", cmdSN,
					"ds_len", dsLen)
			}
			if err := WriteRawPDU(w, p, digestByteOrder); err != nil {
				return err
			}
		}
	}
}

// ReadPump continuously reads PDUs from r and dispatches them by ITT.
// PDUs with the reserved ITT 0xFFFFFFFF (unsolicited target PDUs such as
// NOP-In pings and async messages) are sent to unsolicitedCh. All other
// PDUs are delivered via router.Dispatch. Returns when the read fails
// (connection closed) or ctx is cancelled.
func ReadPump(ctx context.Context, r io.Reader, router *Router,
	unsolicitedCh chan<- *RawPDU, digestHeader, digestData bool,
	logger *slog.Logger, pduHook func(uint8, *RawPDU), maxRecvDSL uint32, digestByteOrder binary.ByteOrder) error {
	for {
		// Check cancellation before each read.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		raw, err := ReadRawPDU(r, digestHeader, digestData, maxRecvDSL, digestByteOrder)
		if err != nil {
			// Check if context was cancelled during the read.
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return err
		}

		if pduHook != nil {
			pduHook(HookReceive, raw)
		}
		if logger.Enabled(ctx, slog.LevelDebug) {
			opcode := pdu.OpCode(raw.BHS[0] & 0x3f)
			itt := binary.BigEndian.Uint32(raw.BHS[16:20])
			statSN := binary.BigEndian.Uint32(raw.BHS[24:28])
			dsLen := uint32(raw.BHS[5])<<16 | uint32(raw.BHS[6])<<8 | uint32(raw.BHS[7])
			logger.DebugContext(ctx, "pdu received",
				"opcode", opcode.String(),
				"itt", fmt.Sprintf("0x%08x", itt),
				"stat_sn", statSN,
				"ds_len", dsLen)
		}

		// Extract ITT from BHS bytes 16-19.
		itt := binary.BigEndian.Uint32(raw.BHS[16:20])

		if itt == reservedITT {
			// Unsolicited target PDU (NOP-In ping, async message).
			select {
			case unsolicitedCh <- raw:
			default:
				logger.Warn("transport: unsolicited PDU channel full, dropping PDU",
					"opcode", raw.BHS[0]&0x3f)
			}
			continue
		}

		if !router.Dispatch(itt, raw) {
			logger.Warn("transport: no pending entry for ITT, dropping PDU",
				"itt", itt,
				"opcode", raw.BHS[0]&0x3f)
		}
	}
}
