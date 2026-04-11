package session

import (
	"fmt"
	"io"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// sendDataOutBurst reads up to burstLen bytes from t.reader and sends them
// as Data-Out PDUs via writeCh. Each PDU's data segment is bounded by
// maxRecvDSL. Returns the number of bytes sent. DataSN starts at 0 for
// each burst per RFC 7143 Section 11.7.
func (t *task) sendDataOutBurst(writeCh chan<- *transport.RawPDU,
	ttt uint32, offset uint32, burstLen uint32, maxRecvDSL uint32,
	expStatSN func() uint32, stampDigests func(*transport.RawPDU)) (uint32, error) {

	var dataSN uint32 // per-burst DataSN starts at 0 (Pitfall 2)
	var sent uint32

	for sent < burstLen {
		chunkSize := min(maxRecvDSL, burstLen-sent)
		bufBp := transport.GetBuffer(int(chunkSize))
		buf := (*bufBp)[:chunkSize]

		n, err := io.ReadFull(t.reader, buf[:chunkSize])
		if err != nil && err != io.ErrUnexpectedEOF {
			transport.PutBuffer(bufBp)
			// Partial reads are discarded on non-EOF errors; the caller
			// will see the error and can retry the entire burst.
			return sent, fmt.Errorf("session: read write data: %w", err)
		}
		if n == 0 {
			transport.PutBuffer(bufBp)
			break
		}

		isFinal := sent+uint32(n) >= burstLen || err == io.ErrUnexpectedEOF

		dout := &pdu.DataOut{
			Header: pdu.Header{
				Final:            isFinal,
				InitiatorTaskTag: t.itt,
				DataSegmentLen:   uint32(n),
			},
			TargetTransferTag: ttt,
			ExpStatSN:         expStatSN(),
			DataSN:            dataSN,
			BufferOffset:      offset + sent,
			Data:              buf[:n],
		}

		bhs, encErr := dout.MarshalBHS()
		if encErr != nil {
			transport.PutBuffer(bufBp)
			return sent, fmt.Errorf("session: encode DataOut: %w", encErr)
		}

		// D-09: Defense-in-depth check — ensure the data segment does not exceed
		// the target's MaxRecvDataSegmentLength before putting it on the wire.
		// chunkSize is already bounded by min(maxRecvDSL, ...) above, so this
		// should never trigger in practice, but guards against programmer error.
		if valErr := transport.ValidateOutgoingSegmentLength(uint32(n), maxRecvDSL); valErr != nil {
			transport.PutBuffer(bufBp)
			return sent, fmt.Errorf("session: outgoing Data-Out validation: %w", valErr)
		}

		// Copy data out of the pool buffer before crossing the goroutine boundary.
		// WriteRawPDU (called by WritePump) will copy DataSegment into its own
		// pool buffer, so dsData only needs to live until that copy completes.
		// The pool buffer is freed here — in the originating goroutine — so
		// WritePump never touches the pool-owned memory (D-01, T-02-02).
		dsData := make([]byte, n)
		copy(dsData, buf[:n])
		transport.PutBuffer(bufBp)

		raw := &transport.RawPDU{
			BHS:         bhs,
			DataSegment: dsData,
		}
		if stampDigests != nil {
			stampDigests(raw)
		}

		writeCh <- raw

		dataSN++
		sent += uint32(n)

		if err == io.ErrUnexpectedEOF {
			break // reader exhausted
		}
	}

	return sent, nil
}

// handleR2T processes an R2T PDU by reading data from the task's io.Reader
// and sending Data-Out PDUs. Enforces MaxBurstLength per WRITE-05.
func (t *task) handleR2T(r2t *pdu.R2T, writeCh chan<- *transport.RawPDU,
	expStatSN func() uint32, params login.NegotiatedParams,
	stampDigests func(*transport.RawPDU)) error {

	// Cap at MaxBurstLength per WRITE-05.
	desired := min(r2t.DesiredDataTransferLength, params.MaxBurstLength)

	_, err := t.sendDataOutBurst(writeCh, r2t.TargetTransferTag,
		r2t.BufferOffset, desired, params.MaxRecvDataSegmentLength, expStatSN, stampDigests)
	return err
}

// sendUnsolicitedDataOut sends unsolicited Data-Out PDUs (TTT=0xFFFFFFFF)
// when InitialR2T=No. The burst is bounded by FirstBurstLength minus any
// immediate data already sent (tracked in t.bytesSent).
func (t *task) sendUnsolicitedDataOut(writeCh chan<- *transport.RawPDU,
	expStatSN func() uint32, params login.NegotiatedParams,
	stampDigests func(*transport.RawPDU)) error {

	remaining := params.FirstBurstLength - t.bytesSent
	if remaining <= 0 {
		return nil // immediate data consumed entire first burst
	}

	const unsolicitedTTT = 0xFFFFFFFF
	offset := t.bytesSent

	sent, err := t.sendDataOutBurst(writeCh, unsolicitedTTT, offset,
		remaining, params.MaxRecvDataSegmentLength, expStatSN, stampDigests)
	t.bytesSent += sent
	return err
}
