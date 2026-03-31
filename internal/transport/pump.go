package transport

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
)

// WritePump owns all writes to the underlying connection. It receives RawPDUs
// from writeCh and serializes them to w one at a time, preventing TCP byte
// interleaving (Pitfall 7). Returns when ctx is cancelled or writeCh is closed.
func WritePump(ctx context.Context, w io.Writer, writeCh <-chan *RawPDU) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case pdu, ok := <-writeCh:
			if !ok {
				return nil // channel closed
			}
			if err := WriteRawPDU(w, pdu); err != nil {
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
func ReadPump(ctx context.Context, r io.Reader, router *Router, unsolicitedCh chan<- *RawPDU, digestHeader, digestData bool) error {
	for {
		// Check cancellation before each read.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		raw, err := ReadRawPDU(r, digestHeader, digestData)
		if err != nil {
			// Check if context was cancelled during the read.
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return err
		}

		// Extract ITT from BHS bytes 16-19.
		itt := binary.BigEndian.Uint32(raw.BHS[16:20])

		if itt == reservedITT {
			// Unsolicited target PDU (NOP-In ping, async message).
			select {
			case unsolicitedCh <- raw:
			default:
				slog.Warn("transport: unsolicited PDU channel full, dropping PDU",
					"opcode", raw.BHS[0]&0x3f)
			}
			continue
		}

		if !router.Dispatch(itt, raw) {
			slog.Warn("transport: no pending entry for ITT, dropping PDU",
				"itt", itt,
				"opcode", raw.BHS[0]&0x3f)
		}
	}
}
