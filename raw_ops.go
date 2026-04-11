package uiscsi

import (
	"context"
	"fmt"
	"io"

	"github.com/uiscsi/uiscsi/internal/session"
)

// RawOps provides raw CDB pass-through methods. Obtain via [Session.Raw].
//
// Unlike the typed methods on [SCSIOps], raw methods do NOT interpret the
// SCSI status. CHECK CONDITION (0x02) is returned as-is in [RawResult.Status]
// or via [StreamResult.Wait]. Use [CheckStatus] or [ParseSenseData] for
// convenience.
//
// # Security
//
// Raw CDB methods pass command descriptor blocks to the target without
// interpretation or validation. Any SCSI command can be issued, including
// destructive operations such as FORMAT UNIT, WRITE SAME with UNMAP, or
// PERSISTENT RESERVE OUT with preempt-and-abort. Callers are responsible
// for validating CDB content before submission. Prefer the typed [SCSIOps]
// methods for commands with safe, validated parameter encoding.
type RawOps struct {
	s *Session
}

// ExecuteOption configures raw CDB execution.
type ExecuteOption = executeOption

// Execute sends a raw CDB to the target and returns the buffered result.
// The entire response is read into [RawResult.Data] as []byte.
// For high-throughput streaming, use [RawOps.StreamExecute] instead.
//
// # Security
//
// Execute passes the CDB to the target without interpretation. Any SCSI
// command can be issued, including destructive operations (FORMAT UNIT,
// WRITE SAME, etc.). See [RawOps] for details.
func (o *RawOps) Execute(ctx context.Context, lun uint64, cdb []byte, opts ...ExecuteOption) (*RawResult, error) {
	return o.s.execute(ctx, lun, cdb, opts...)
}

// StreamExecute sends a raw CDB and returns a streaming result.
// Response data is delivered via [StreamResult.Data] as an [io.Reader]
// with bounded memory (~64KB). Call [StreamResult.Wait] after consuming
// Data to retrieve the final SCSI status and sense data.
//
// # Security
//
// StreamExecute passes the CDB to the target without interpretation. Any
// SCSI command can be issued, including destructive operations. See
// [RawOps] for details.
func (o *RawOps) StreamExecute(ctx context.Context, lun uint64, cdb []byte, opts ...ExecuteOption) (*StreamResult, error) {
	return o.s.streamExecute(ctx, lun, cdb, opts...)
}

// WithDataIn configures a read response of up to allocLen bytes.
func WithDataIn(allocLen uint32) ExecuteOption {
	return func(c *executeConfig) {
		c.dataInLen = allocLen
	}
}

// WithDataOut configures data to send with the command.
func WithDataOut(r io.Reader, length uint32) ExecuteOption {
	return func(c *executeConfig) {
		c.dataOut = r
		c.dataOutLen = length
	}
}

// executeOption is the internal type for ExecuteOption.
type executeOption func(*executeConfig)

type executeConfig struct {
	dataInLen  uint32
	dataOut    io.Reader
	dataOutLen uint32
}

// execute is the internal implementation of Execute.
func (s *Session) execute(ctx context.Context, lun uint64, cdb []byte, opts ...executeOption) (*RawResult, error) {
	cfg := &executeConfig{}
	for _, o := range opts {
		o(cfg)
	}

	if len(cdb) == 0 {
		return nil, fmt.Errorf("iscsi execute: empty CDB")
	}
	if len(cdb) > 16 {
		return nil, fmt.Errorf("iscsi execute: CDB length %d exceeds maximum 16 bytes (AHS Extended CDB not yet supported)", len(cdb))
	}

	var cmd session.Command
	copy(cmd.CDB[:len(cdb)], cdb)
	cmd.LUN = lun

	if cfg.dataInLen > 0 {
		cmd.Read = true
		cmd.ExpectedDataTransferLen = cfg.dataInLen
	}
	if cfg.dataOut != nil {
		cmd.Write = true
		cmd.Data = cfg.dataOut
		cmd.ExpectedDataTransferLen = cfg.dataOutLen
	}

	result, err := submitAndWait(s.s, ctx, cmd)
	if err != nil {
		return nil, err
	}
	if result.Err != nil {
		return nil, wrapTransportError("execute", result.Err)
	}

	rr := &RawResult{
		Status:    result.Status,
		SenseData: result.SenseData,
	}
	if result.Data != nil {
		rr.Data, err = io.ReadAll(result.Data)
		if err != nil {
			return nil, wrapTransportError("read", err)
		}
	}
	return rr, nil
}

// streamExecute is the internal implementation of StreamExecute.
func (s *Session) streamExecute(ctx context.Context, lun uint64, cdb []byte, opts ...executeOption) (*StreamResult, error) {
	cfg := &executeConfig{}
	for _, o := range opts {
		o(cfg)
	}

	if len(cdb) == 0 {
		return nil, fmt.Errorf("iscsi execute: empty CDB")
	}
	if len(cdb) > 16 {
		return nil, fmt.Errorf("iscsi execute: CDB length %d exceeds maximum 16 bytes (AHS Extended CDB not yet supported)", len(cdb))
	}

	var cmd session.Command
	copy(cmd.CDB[:len(cdb)], cdb)
	cmd.LUN = lun

	if cfg.dataInLen > 0 {
		cmd.Read = true
		cmd.ExpectedDataTransferLen = cfg.dataInLen
	}
	if cfg.dataOut != nil {
		cmd.Write = true
		cmd.Data = cfg.dataOut
		cmd.ExpectedDataTransferLen = cfg.dataOutLen
	}

	resultCh, dataReader, err := s.s.SubmitStreaming(ctx, cmd)
	if err != nil {
		return nil, wrapTransportError("submit", err)
	}

	return &StreamResult{
		Data:     dataReader,
		resultCh: resultCh,
	}, nil
}
