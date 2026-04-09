package uiscsi

import (
	"bytes"
	"context"

	"github.com/uiscsi/uiscsi/internal/scsi"
	"github.com/uiscsi/uiscsi/internal/session"
)

// SCSIOps provides typed SCSI command methods. Obtain via [Session.SCSI].
type SCSIOps struct {
	s *session.Session
}

// ReadBlocks reads blocks from the target using READ(16).
func (o *SCSIOps) ReadBlocks(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32) ([]byte, error) {
	cmd := scsi.Read16(lun, lba, blocks, blockSize)
	return submitAndCheck(o.s, ctx, cmd)
}

// WriteBlocks writes blocks to the target using WRITE(16).
func (o *SCSIOps) WriteBlocks(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32, data []byte) error {
	cmd := scsi.Write16(lun, lba, blocks, blockSize, bytes.NewReader(data))
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// Inquiry sends a standard INQUIRY command and returns the parsed response.
func (o *SCSIOps) Inquiry(ctx context.Context, lun uint64) (*InquiryData, error) {
	cmd := scsi.Inquiry(lun, 255)
	result, err := submitAndWait(o.s, ctx, cmd)
	if err != nil {
		return nil, err
	}
	resp, err := scsi.ParseInquiry(result)
	if err != nil {
		return nil, wrapSCSIError(err)
	}
	return convertInquiry(resp), nil
}

// ReadCapacity returns the capacity of the specified LUN using READ CAPACITY(16)
// with fallback to READ CAPACITY(10).
func (o *SCSIOps) ReadCapacity(ctx context.Context, lun uint64) (*Capacity, error) {
	cmd := scsi.ReadCapacity16(lun, 32)
	result, err := submitAndWait(o.s, ctx, cmd)
	if err != nil {
		return nil, err
	}
	resp16, err := scsi.ParseReadCapacity16(result)
	if err == nil {
		return convertCapacity16(resp16), nil
	}
	cmd10 := scsi.ReadCapacity10(lun)
	result10, err := submitAndWait(o.s, ctx, cmd10)
	if err != nil {
		return nil, err
	}
	resp10, err := scsi.ParseReadCapacity10(result10)
	if err != nil {
		return nil, wrapSCSIError(err)
	}
	return convertCapacity10(resp10), nil
}

// TestUnitReady checks whether the specified LUN is ready.
func (o *SCSIOps) TestUnitReady(ctx context.Context, lun uint64) error {
	cmd := scsi.TestUnitReady(lun)
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// RequestSense sends a REQUEST SENSE command and returns the parsed sense data.
func (o *SCSIOps) RequestSense(ctx context.Context, lun uint64) (*SenseInfo, error) {
	cmd := scsi.RequestSense(lun, 252)
	data, err := submitAndCheck(o.s, ctx, cmd)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	sd, err := scsi.ParseSense(data)
	if err != nil {
		return nil, err
	}
	return convertSense(sd), nil
}

// ReportLuns returns all LUNs reported by the target.
func (o *SCSIOps) ReportLuns(ctx context.Context) ([]uint64, error) {
	cmd := scsi.ReportLuns(1024)
	result, err := submitAndWait(o.s, ctx, cmd)
	if err != nil {
		return nil, err
	}
	luns, err := scsi.ParseReportLuns(result)
	if err != nil {
		return nil, wrapSCSIError(err)
	}
	return luns, nil
}

// ModeSense6 sends a MODE SENSE(6) command and returns the raw mode page bytes.
func (o *SCSIOps) ModeSense6(ctx context.Context, lun uint64, pageCode, subPageCode uint8) ([]byte, error) {
	cmd := scsi.ModeSense6(lun, pageCode, subPageCode, 255)
	return submitAndCheck(o.s, ctx, cmd)
}

// ModeSense10 sends a MODE SENSE(10) command and returns the raw mode page bytes.
func (o *SCSIOps) ModeSense10(ctx context.Context, lun uint64, pageCode, subPageCode uint8) ([]byte, error) {
	cmd := scsi.ModeSense10(lun, pageCode, subPageCode, 1024)
	return submitAndCheck(o.s, ctx, cmd)
}

// ModeSelect6 sends a MODE SELECT(6) command with the given parameter data.
// The data must include the mode parameter header, block descriptor(s), and
// any mode pages. PF (Page Format) bit is set automatically.
func (o *SCSIOps) ModeSelect6(ctx context.Context, lun uint64, data []byte) error {
	cmd := scsi.ModeSelect6(lun, data)
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// ModeSelect10 sends a MODE SELECT(10) command with the given parameter data.
func (o *SCSIOps) ModeSelect10(ctx context.Context, lun uint64, data []byte) error {
	cmd := scsi.ModeSelect10(lun, data)
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// SynchronizeCache flushes the target's volatile cache for the entire LUN.
func (o *SCSIOps) SynchronizeCache(ctx context.Context, lun uint64) error {
	cmd := scsi.SynchronizeCache16(lun, 0, 0)
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// Verify requests the target to verify the specified LBA range.
func (o *SCSIOps) Verify(ctx context.Context, lun uint64, lba uint64, blocks uint32) error {
	cmd := scsi.Verify16(lun, lba, blocks)
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// WriteSame writes the same block of data to the specified LBA range.
func (o *SCSIOps) WriteSame(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32, data []byte) error {
	cmd := scsi.WriteSame16(lun, lba, blocks, blockSize, bytes.NewReader(data))
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// Unmap deallocates the specified LBA ranges (thin provisioning).
func (o *SCSIOps) Unmap(ctx context.Context, lun uint64, descriptors []UnmapBlockDescriptor) error {
	internal := make([]scsi.UnmapBlockDescriptor, len(descriptors))
	for i, d := range descriptors {
		internal[i] = scsi.UnmapBlockDescriptor{LBA: d.LBA, BlockCount: d.Blocks}
	}
	cmd := scsi.Unmap(lun, internal)
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// CompareAndWrite performs an atomic read-compare-write operation.
func (o *SCSIOps) CompareAndWrite(ctx context.Context, lun uint64, lba uint64, blocks uint8, blockSize uint32, data []byte) error {
	cmd := scsi.CompareAndWrite(lun, lba, blocks, blockSize, bytes.NewReader(data))
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// StartStopUnit sends a START STOP UNIT command.
func (o *SCSIOps) StartStopUnit(ctx context.Context, lun uint64, powerCondition uint8, start, loadEject bool) error {
	cmd := scsi.StartStopUnit(lun, powerCondition, start, loadEject)
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}

// PersistReserveIn sends a PERSISTENT RESERVE IN command and returns the raw response.
func (o *SCSIOps) PersistReserveIn(ctx context.Context, lun uint64, serviceAction uint8) ([]byte, error) {
	cmd := scsi.PersistReserveIn(lun, serviceAction, 1024)
	return submitAndCheck(o.s, ctx, cmd)
}

// PersistReserveOut sends a PERSISTENT RESERVE OUT command.
func (o *SCSIOps) PersistReserveOut(ctx context.Context, lun uint64, serviceAction, scopeType uint8, key, saKey uint64) error {
	cmd := scsi.PersistReserveOut(lun, serviceAction, scopeType, key, saKey)
	_, err := submitAndCheck(o.s, ctx, cmd)
	return err
}
