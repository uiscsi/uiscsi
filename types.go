package uiscsi

import (
	"io"
	"time"

	"github.com/uiscsi/uiscsi/internal/scsi"
	"github.com/uiscsi/uiscsi/internal/session"
)

// DecodeLUN extracts the flat LUN number from an 8-byte SAM LUN encoding
// as returned by [Session.ReportLuns]. SAM-5 Section 4.6.1 defines the
// encoding: the first two bytes carry the address method and LUN value.
//
// For peripheral device addressing (method 00b), the LUN is in the low
// 8 bits of the first two bytes. For flat space addressing (method 01b),
// the LUN is a 14-bit value. Other methods return the raw upper 16 bits.
func DecodeLUN(raw uint64) uint64 {
	method := (raw >> 62) & 0x03
	switch method {
	case 0x00: // Peripheral device addressing
		return (raw >> 48) & 0xFF
	case 0x01: // Flat space addressing
		return (raw >> 48) & 0x3FFF
	default:
		return (raw >> 48) & 0xFFFF
	}
}

// deviceTypeNames maps SCSI peripheral device type codes (SPC-4 Table 49)
// to short human-readable names. Indexed by code (0x00-0x1F).
var deviceTypeNames = [32]string{
	0x00: "disk",
	0x01: "tape",
	0x02: "printer",
	0x03: "processor",
	0x04: "worm",
	0x05: "cd/dvd",
	0x06: "scanner",
	0x07: "optical",
	0x08: "media changer",
	0x09: "communications",
	0x0C: "storage",
	0x0D: "enclosure",
	0x0E: "disk",
	0x0F: "osd",
	0x11: "osd",
	0x1E: "wlun",
	0x1F: "unknown",
}

// DeviceTypeName returns the short human-readable name for a SCSI peripheral
// device type code as found in [InquiryData.DeviceType]. The names follow the
// SPC-4 Table 49 convention used by lsscsi. Unknown or unmapped codes return
// "unknown".
func DeviceTypeName(code uint8) string {
	if int(code) >= len(deviceTypeNames) {
		return "unknown"
	}
	name := deviceTypeNames[code]
	if name == "" {
		return "unknown"
	}
	return name
}

// Target represents a discovered iSCSI target.
type Target struct {
	Name    string
	Portals []Portal
}

// Portal represents a target portal address.
type Portal struct {
	Address  string
	Port     int
	GroupTag int
}

// RawResult carries the raw SCSI command outcome for Execute().
type RawResult struct {
	Status    uint8
	Data      []byte
	SenseData []byte
}

// StreamResult carries the outcome of a streaming raw SCSI command via
// [Session.StreamExecute]. Unlike [RawResult], Data is an [io.Reader] that
// streams the response as PDUs arrive, without buffering the entire response
// into []byte. This is critical for high-throughput sequential devices (tape
// drives, etc.) where large blocks (256KB–4MB) at sustained rates (400+ MB/s)
// make double-buffering expensive.
//
// Memory usage is bounded to a small number of PDU-sized chunks regardless
// of the total transfer size (typically ~64KB with default negotiated
// parameters).
//
// The caller must fully consume Data (or drain to [io.Discard]), then call
// [StreamResult.Wait] to retrieve the final SCSI status and sense data.
//
// Usage:
//
//	sr, err := session.StreamExecute(ctx, lun, cdb, uiscsi.WithDataIn(blockSize))
//	if err != nil { ... }
//	_, err = io.Copy(dst, sr.Data)     // stream data
//	status, sense, err := sr.Wait()    // get final status
type StreamResult struct {
	// Data streams the SCSI read response as PDUs arrive from the target.
	// Nil for non-read commands. Must be fully consumed before calling Wait.
	Data io.Reader

	// resultCh delivers the final status/sense when the command completes.
	resultCh <-chan session.Result
}

// Wait blocks until the SCSI command completes and returns the final status
// and sense data. The caller should fully consume (or discard) Data before
// calling Wait; otherwise Wait may block indefinitely due to flow-control
// backpressure. If the transport fails mid-transfer, Data.Read returns the
// error, and Wait returns it as well.
func (sr *StreamResult) Wait() (status uint8, senseData []byte, err error) {
	r := <-sr.resultCh
	if r.Err != nil {
		return 0, nil, r.Err
	}
	return r.Status, r.SenseData, nil
}

// InquiryData holds a parsed INQUIRY response.
type InquiryData struct {
	DeviceType uint8
	VendorID   string
	ProductID  string
	Revision   string
}

// Capacity holds a parsed READ CAPACITY response (unified for 10/16).
type Capacity struct {
	LBA           uint64
	BlockSize     uint32
	LogicalBlocks uint64
}

// SenseInfo holds parsed sense data.
type SenseInfo struct {
	Key         uint8
	ASC         uint8
	ASCQ        uint8
	Description string
	Filemark    bool
	EOM         bool
	ILI         bool
}

// TMFResult carries a task management function outcome.
type TMFResult struct {
	Response uint8
}

// AsyncEvent carries an asynchronous event from the target.
type AsyncEvent struct {
	EventCode  uint8
	VendorCode uint8
	Parameter1 uint16
	Parameter2 uint16
	Parameter3 uint16
	Data       []byte
}

// MetricEventType discriminates metric event kinds.
type MetricEventType uint8

// Metric event type constants, mirroring internal session values.
const (
	MetricPDUSent         MetricEventType = MetricEventType(session.MetricPDUSent)
	MetricPDUReceived     MetricEventType = MetricEventType(session.MetricPDUReceived)
	MetricCommandComplete MetricEventType = MetricEventType(session.MetricCommandComplete)
	MetricBytesIn         MetricEventType = MetricEventType(session.MetricBytesIn)
	MetricBytesOut        MetricEventType = MetricEventType(session.MetricBytesOut)
)

// MetricEvent carries a single metric observation.
type MetricEvent struct {
	Type    MetricEventType
	OpCode  uint8
	Bytes   uint64
	Latency time.Duration
}

// PDUDirection indicates whether a PDU was sent or received.
type PDUDirection uint8

// PDU direction constants, mirroring internal session values.
const (
	PDUSend    PDUDirection = PDUDirection(session.PDUSend)
	PDUReceive PDUDirection = PDUDirection(session.PDUReceive)
)

// UnmapBlockDescriptor describes a single LBA range to deallocate.
type UnmapBlockDescriptor struct {
	LBA    uint64
	Blocks uint32
}

// convertTarget converts an internal DiscoveryTarget to a public Target.
func convertTarget(dt session.DiscoveryTarget) Target {
	t := Target{Name: dt.Name}
	for _, p := range dt.Portals {
		t.Portals = append(t.Portals, Portal{
			Address:  p.Address,
			Port:     p.Port,
			GroupTag: p.GroupTag,
		})
	}
	return t
}

// convertInquiry converts an internal InquiryResponse to a public InquiryData.
func convertInquiry(r *scsi.InquiryResponse) *InquiryData {
	return &InquiryData{
		DeviceType: r.PeripheralDeviceType,
		VendorID:   r.Vendor,
		ProductID:  r.Product,
		Revision:   r.Revision,
	}
}

// convertCapacity16 converts a ReadCapacity16Response to a public Capacity.
func convertCapacity16(r *scsi.ReadCapacity16Response) *Capacity {
	return &Capacity{
		LBA:           r.LastLBA,
		BlockSize:     r.BlockSize,
		LogicalBlocks: r.LastLBA + 1,
	}
}

// convertCapacity10 converts a ReadCapacity10Response to a public Capacity.
func convertCapacity10(r *scsi.ReadCapacity10Response) *Capacity {
	return &Capacity{
		LBA:           uint64(r.LastLBA),
		BlockSize:     r.BlockSize,
		LogicalBlocks: uint64(r.LastLBA) + 1,
	}
}

// convertSense converts internal SenseData to a public SenseInfo.
func convertSense(sd *scsi.SenseData) *SenseInfo {
	return &SenseInfo{
		Key:         uint8(sd.Key),
		ASC:         sd.ASC,
		ASCQ:        sd.ASCQ,
		Description: sd.String(),
		Filemark:    sd.Filemark,
		EOM:         sd.EOM,
		ILI:         sd.ILI,
	}
}

// convertTMFResult converts an internal TMFResult to a public TMFResult.
func convertTMFResult(r *session.TMFResult) *TMFResult {
	return &TMFResult{Response: r.Response}
}

// convertAsyncEvent converts an internal AsyncEvent to a public AsyncEvent.
func convertAsyncEvent(ae session.AsyncEvent) AsyncEvent {
	return AsyncEvent{
		EventCode:  ae.EventCode,
		VendorCode: ae.VendorCode,
		Parameter1: ae.Parameter1,
		Parameter2: ae.Parameter2,
		Parameter3: ae.Parameter3,
		Data:       ae.Data,
	}
}

// convertMetricEvent converts an internal MetricEvent to a public MetricEvent.
func convertMetricEvent(me session.MetricEvent) MetricEvent {
	return MetricEvent{
		Type:    MetricEventType(me.Type),
		OpCode:  uint8(me.OpCode),
		Bytes:   me.Bytes,
		Latency: me.Latency,
	}
}
