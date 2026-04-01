package session

import (
	"fmt"
	"io"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// sendDataOutBurst reads up to burstLen bytes from t.reader and sends them
// as Data-Out PDUs via writeCh. Each PDU's data segment is bounded by
// maxRecvDSL. Returns the number of bytes sent. DataSN starts at 0 for
// each burst per RFC 7143 Section 11.7.
func (t *task) sendDataOutBurst(writeCh chan<- *transport.RawPDU,
	ttt uint32, offset uint32, burstLen uint32, maxRecvDSL uint32,
	expStatSN func() uint32) (uint32, error) {

	var dataSN uint32 // per-burst DataSN starts at 0 (Pitfall 2)
	var sent uint32

	for sent < burstLen {
		chunkSize := min(maxRecvDSL, burstLen-sent)
		buf := transport.GetBuffer(int(chunkSize))

		n, err := io.ReadFull(t.reader, buf[:chunkSize])
		if err != nil && err != io.ErrUnexpectedEOF {
			transport.PutBuffer(buf)
			if n == 0 {
				return sent, fmt.Errorf("session: read write data: %w", err)
			}
			// Partial read with unexpected error -- still send what we have,
			// but return the error after.
			return sent, fmt.Errorf("session: read write data: %w", err)
		}
		if n == 0 {
			transport.PutBuffer(buf)
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
			transport.PutBuffer(buf)
			return sent, fmt.Errorf("session: encode DataOut: %w", encErr)
		}

		raw := &transport.RawPDU{
			BHS:         bhs,
			DataSegment: buf[:n],
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
	expStatSN func() uint32, params login.NegotiatedParams) error {

	desired := r2t.DesiredDataTransferLength
	// Cap at MaxBurstLength per WRITE-05.
	if desired > params.MaxBurstLength {
		desired = params.MaxBurstLength
	}

	_, err := t.sendDataOutBurst(writeCh, r2t.TargetTransferTag,
		r2t.BufferOffset, desired, params.MaxRecvDataSegmentLength, expStatSN)
	return err
}

// sendUnsolicitedDataOut sends unsolicited Data-Out PDUs (TTT=0xFFFFFFFF)
// when InitialR2T=No. The burst is bounded by FirstBurstLength minus any
// immediate data already sent (tracked in t.bytesSent).
func (t *task) sendUnsolicitedDataOut(writeCh chan<- *transport.RawPDU,
	expStatSN func() uint32, params login.NegotiatedParams) error {

	remaining := params.FirstBurstLength - t.bytesSent
	if remaining <= 0 {
		return nil // immediate data consumed entire first burst
	}

	const unsolicitedTTT = 0xFFFFFFFF
	offset := t.bytesSent

	sent, err := t.sendDataOutBurst(writeCh, unsolicitedTTT, offset,
		remaining, params.MaxRecvDataSegmentLength, expStatSN)
	t.bytesSent += sent
	return err
}
