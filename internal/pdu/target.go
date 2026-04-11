package pdu

import "encoding/binary"

// NOPIn represents an iSCSI NOP-In PDU (opcode 0x20).
// RFC 7143 Section 11.19.
type NOPIn struct {
	Header
	TargetTransferTag uint32
	StatSN            uint32
	ExpCmdSN          uint32
	MaxCmdSN          uint32
	Data              []byte // Ping data
}

func (*NOPIn) Opcode() OpCode       { return OpNOPIn }
func (p *NOPIn) DataSegment() []byte   { return p.Data }
func (p *NOPIn) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpNOPIn
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	binary.BigEndian.PutUint32(bhs[20:24], p.TargetTransferTag)
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	return bhs, nil
}
func (p *NOPIn) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.TargetTransferTag = binary.BigEndian.Uint32(bhs[20:24])
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
}

// SCSIResponse represents an iSCSI SCSI Response PDU (opcode 0x21).
// RFC 7143 Section 11.4.
type SCSIResponse struct {
	Header
	BidiOverflow       bool  // o bit (byte 1 bit 4)
	BidiUnderflow      bool  // u bit (byte 1 bit 3)
	Overflow           bool  // O bit (byte 1 bit 2)
	Underflow          bool  // U bit (byte 1 bit 1)
	Response           uint8 // byte 2
	Status             uint8 // byte 3
	SNACKTag           uint32
	StatSN             uint32
	ExpCmdSN           uint32
	MaxCmdSN           uint32
	ExpDataSN          uint32
	BidiResidualCount  uint32
	ResidualCount      uint32
	Data               []byte // Sense data
}

func (*SCSIResponse) Opcode() OpCode       { return OpSCSIResponse }
func (p *SCSIResponse) DataSegment() []byte   { return p.Data }
func (p *SCSIResponse) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpSCSIResponse
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	if p.BidiOverflow {
		bhs[1] |= 0x10
	}
	if p.BidiUnderflow {
		bhs[1] |= 0x08
	}
	if p.Overflow {
		bhs[1] |= 0x04
	}
	if p.Underflow {
		bhs[1] |= 0x02
	}
	bhs[2] = p.Response
	bhs[3] = p.Status
	binary.BigEndian.PutUint32(bhs[20:24], p.SNACKTag)
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	binary.BigEndian.PutUint32(bhs[36:40], p.ExpDataSN)
	binary.BigEndian.PutUint32(bhs[40:44], p.BidiResidualCount)
	binary.BigEndian.PutUint32(bhs[44:48], p.ResidualCount)
	return bhs, nil
}
func (p *SCSIResponse) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.BidiOverflow = bhs[1]&0x10 != 0
	p.BidiUnderflow = bhs[1]&0x08 != 0
	p.Overflow = bhs[1]&0x04 != 0
	p.Underflow = bhs[1]&0x02 != 0
	p.Response = bhs[2]
	p.Status = bhs[3]
	p.SNACKTag = binary.BigEndian.Uint32(bhs[20:24])
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
	p.ExpDataSN = binary.BigEndian.Uint32(bhs[36:40])
	p.BidiResidualCount = binary.BigEndian.Uint32(bhs[40:44])
	p.ResidualCount = binary.BigEndian.Uint32(bhs[44:48])
}

// TaskMgmtResp represents an iSCSI Task Management Function Response PDU (opcode 0x22).
// RFC 7143 Section 11.6.
type TaskMgmtResp struct {
	Header
	Response uint8 // byte 2
	StatSN   uint32
	ExpCmdSN uint32
	MaxCmdSN uint32
}

func (*TaskMgmtResp) Opcode() OpCode       { return OpTaskMgmtResp }
func (*TaskMgmtResp) DataSegment() []byte   { return nil }
func (p *TaskMgmtResp) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpTaskMgmtResp
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	bhs[2] = p.Response
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	return bhs, nil
}
func (p *TaskMgmtResp) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Response = bhs[2]
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
}

// LoginResp represents an iSCSI Login Response PDU (opcode 0x23).
// RFC 7143 Section 11.13.
type LoginResp struct {
	Header
	Transit       bool  // T bit
	Continue      bool  // C bit
	CSG           uint8 // Current Stage
	NSG           uint8 // Next Stage
	VersionMax    uint8 // byte 2
	VersionActive uint8 // byte 3
	ISID          [6]byte
	TSIH          uint16
	StatSN        uint32
	ExpCmdSN      uint32
	MaxCmdSN      uint32
	StatusClass   uint8  // byte 36
	StatusDetail  uint8  // byte 37
	Data          []byte // key-value pairs
}

func (*LoginResp) Opcode() OpCode       { return OpLoginResp }
func (p *LoginResp) DataSegment() []byte   { return p.Data }
func (p *LoginResp) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpLoginResp
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	bhs[1] = 0
	if p.Transit {
		bhs[1] |= 0x80
	}
	if p.Continue {
		bhs[1] |= 0x40
	}
	bhs[1] |= (p.CSG & 0x03) << 2
	bhs[1] |= p.NSG & 0x03
	bhs[2] = p.VersionMax
	bhs[3] = p.VersionActive
	copy(bhs[8:14], p.ISID[:])
	binary.BigEndian.PutUint16(bhs[14:16], p.TSIH)
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	bhs[36] = p.StatusClass
	bhs[37] = p.StatusDetail
	return bhs, nil
}
func (p *LoginResp) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Transit = bhs[1]&0x80 != 0
	p.Continue = bhs[1]&0x40 != 0
	p.CSG = (bhs[1] >> 2) & 0x03
	p.NSG = bhs[1] & 0x03
	p.VersionMax = bhs[2]
	p.VersionActive = bhs[3]
	copy(p.ISID[:], bhs[8:14])
	p.TSIH = binary.BigEndian.Uint16(bhs[14:16])
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
	p.StatusClass = bhs[36]
	p.StatusDetail = bhs[37]
}

// TextResp represents an iSCSI Text Response PDU (opcode 0x24).
// RFC 7143 Section 11.11.
type TextResp struct {
	Header
	Continue          bool // C bit
	TargetTransferTag uint32
	StatSN            uint32
	ExpCmdSN          uint32
	MaxCmdSN          uint32
	Data              []byte // key-value pairs
}

func (*TextResp) Opcode() OpCode       { return OpTextResp }
func (p *TextResp) DataSegment() []byte   { return p.Data }
func (p *TextResp) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpTextResp
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	if p.Continue {
		bhs[1] |= 0x40
	}
	binary.BigEndian.PutUint32(bhs[20:24], p.TargetTransferTag)
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	return bhs, nil
}
func (p *TextResp) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Continue = bhs[1]&0x40 != 0
	p.TargetTransferTag = binary.BigEndian.Uint32(bhs[20:24])
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
}

// DataIn represents an iSCSI Data-In PDU (opcode 0x25).
// RFC 7143 Section 11.7.
type DataIn struct {
	Header
	Acknowledge       bool  // A bit (byte 1 bit 6)
	ResidualOverflow  bool  // O bit (byte 1 bit 2)
	ResidualUnderflow bool  // U bit (byte 1 bit 3)
	HasStatus         bool  // S bit (byte 1 bit 0)
	Status            uint8 // byte 3 (only valid if S=1)
	TargetTransferTag uint32
	StatSN            uint32 // only valid if S=1
	ExpCmdSN          uint32
	MaxCmdSN          uint32
	DataSN            uint32
	BufferOffset      uint32
	ResidualCount     uint32
	Data              []byte // Read data
}

func (*DataIn) Opcode() OpCode       { return OpDataIn }
func (p *DataIn) DataSegment() []byte   { return p.Data }
func (p *DataIn) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpDataIn
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	if p.Acknowledge {
		bhs[1] |= 0x40
	}
	if p.ResidualOverflow {
		bhs[1] |= 0x04
	}
	if p.ResidualUnderflow {
		bhs[1] |= 0x08
	}
	if p.HasStatus {
		bhs[1] |= 0x01
	}
	bhs[3] = p.Status
	binary.BigEndian.PutUint32(bhs[20:24], p.TargetTransferTag)
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	binary.BigEndian.PutUint32(bhs[36:40], p.DataSN)
	binary.BigEndian.PutUint32(bhs[40:44], p.BufferOffset)
	binary.BigEndian.PutUint32(bhs[44:48], p.ResidualCount)
	return bhs, nil
}
func (p *DataIn) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Acknowledge = bhs[1]&0x40 != 0
	p.ResidualOverflow = bhs[1]&0x04 != 0
	p.ResidualUnderflow = bhs[1]&0x08 != 0
	p.HasStatus = bhs[1]&0x01 != 0
	p.Status = bhs[3]
	p.TargetTransferTag = binary.BigEndian.Uint32(bhs[20:24])
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
	p.DataSN = binary.BigEndian.Uint32(bhs[36:40])
	p.BufferOffset = binary.BigEndian.Uint32(bhs[40:44])
	p.ResidualCount = binary.BigEndian.Uint32(bhs[44:48])
}

// LogoutResp represents an iSCSI Logout Response PDU (opcode 0x26).
// RFC 7143 Section 11.15.
type LogoutResp struct {
	Header
	Response    uint8 // byte 2
	StatSN      uint32
	ExpCmdSN    uint32
	MaxCmdSN    uint32
	Time2Wait   uint16 // bytes 40-41
	Time2Retain uint16 // bytes 42-43
}

func (*LogoutResp) Opcode() OpCode       { return OpLogoutResp }
func (*LogoutResp) DataSegment() []byte   { return nil }
func (p *LogoutResp) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpLogoutResp
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	bhs[2] = p.Response
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	binary.BigEndian.PutUint16(bhs[40:42], p.Time2Wait)
	binary.BigEndian.PutUint16(bhs[42:44], p.Time2Retain)
	return bhs, nil
}
func (p *LogoutResp) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Response = bhs[2]
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
	p.Time2Wait = binary.BigEndian.Uint16(bhs[40:42])
	p.Time2Retain = binary.BigEndian.Uint16(bhs[42:44])
}

// R2T represents an iSCSI Ready To Transfer PDU (opcode 0x31).
// RFC 7143 Section 11.8.
type R2T struct {
	Header
	TargetTransferTag          uint32
	StatSN                     uint32
	ExpCmdSN                   uint32
	MaxCmdSN                   uint32
	R2TSN                      uint32
	BufferOffset               uint32
	DesiredDataTransferLength  uint32
}

func (*R2T) Opcode() OpCode       { return OpR2T }
func (*R2T) DataSegment() []byte   { return nil }
func (p *R2T) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpR2T
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	binary.BigEndian.PutUint32(bhs[20:24], p.TargetTransferTag)
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	binary.BigEndian.PutUint32(bhs[36:40], p.R2TSN)
	binary.BigEndian.PutUint32(bhs[40:44], p.BufferOffset)
	binary.BigEndian.PutUint32(bhs[44:48], p.DesiredDataTransferLength)
	return bhs, nil
}
func (p *R2T) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.TargetTransferTag = binary.BigEndian.Uint32(bhs[20:24])
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
	p.R2TSN = binary.BigEndian.Uint32(bhs[36:40])
	p.BufferOffset = binary.BigEndian.Uint32(bhs[40:44])
	p.DesiredDataTransferLength = binary.BigEndian.Uint32(bhs[44:48])
}

// AsyncMsg represents an iSCSI Asynchronous Message PDU (opcode 0x32).
// RFC 7143 Section 11.9.
type AsyncMsg struct {
	Header
	StatSN     uint32
	ExpCmdSN   uint32
	MaxCmdSN   uint32
	AsyncEvent uint8  // byte 36
	AsyncVCode uint8  // byte 37
	Parameter1 uint16 // bytes 38-39
	Parameter2 uint16 // bytes 40-41
	Parameter3 uint16 // bytes 42-43
	Data       []byte // Async event data
}

func (*AsyncMsg) Opcode() OpCode       { return OpAsyncMsg }
func (p *AsyncMsg) DataSegment() []byte   { return p.Data }
func (p *AsyncMsg) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpAsyncMsg
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	bhs[36] = p.AsyncEvent
	bhs[37] = p.AsyncVCode
	binary.BigEndian.PutUint16(bhs[38:40], p.Parameter1)
	binary.BigEndian.PutUint16(bhs[40:42], p.Parameter2)
	binary.BigEndian.PutUint16(bhs[42:44], p.Parameter3)
	return bhs, nil
}
func (p *AsyncMsg) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
	p.AsyncEvent = bhs[36]
	p.AsyncVCode = bhs[37]
	p.Parameter1 = binary.BigEndian.Uint16(bhs[38:40])
	p.Parameter2 = binary.BigEndian.Uint16(bhs[40:42])
	p.Parameter3 = binary.BigEndian.Uint16(bhs[42:44])
}

// Reject represents an iSCSI Reject PDU (opcode 0x3f).
// RFC 7143 Section 11.17.
type Reject struct {
	Header
	Reason   uint8 // byte 2
	StatSN   uint32
	ExpCmdSN uint32
	MaxCmdSN uint32
	DataSN   uint32 // DataSN/R2TSN (bytes 36-39)
	Data     []byte // Complete BHS of the rejected PDU
}

func (*Reject) Opcode() OpCode       { return OpReject }
func (p *Reject) DataSegment() []byte   { return p.Data }
func (p *Reject) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpReject
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	bhs[2] = p.Reason
	binary.BigEndian.PutUint32(bhs[24:28], p.StatSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpCmdSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.MaxCmdSN)
	binary.BigEndian.PutUint32(bhs[36:40], p.DataSN)
	return bhs, nil
}
func (p *Reject) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Reason = bhs[2]
	p.StatSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpCmdSN = binary.BigEndian.Uint32(bhs[28:32])
	p.MaxCmdSN = binary.BigEndian.Uint32(bhs[32:36])
	p.DataSN = binary.BigEndian.Uint32(bhs[36:40])
}
