package pdu

import "encoding/binary"

// NOPOut represents an iSCSI NOP-Out PDU (opcode 0x00).
// Used as a ping mechanism or to acknowledge NOP-In from the target.
// RFC 7143 Section 11.18.
type NOPOut struct {
	Header
	TargetTransferTag uint32
	CmdSN             uint32
	ExpStatSN         uint32
	Data              []byte // Ping data
}

func (*NOPOut) Opcode() OpCode       { return OpNOPOut }
func (p *NOPOut) DataSegment() []byte   { return p.Data }
func (p *NOPOut) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpNOPOut
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	binary.BigEndian.PutUint32(bhs[20:24], p.TargetTransferTag)
	binary.BigEndian.PutUint32(bhs[24:28], p.CmdSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpStatSN)
	return bhs, nil
}
func (p *NOPOut) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.TargetTransferTag = binary.BigEndian.Uint32(bhs[20:24])
	p.CmdSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpStatSN = binary.BigEndian.Uint32(bhs[28:32])
}

// SCSICommand represents an iSCSI SCSI Command PDU (opcode 0x01).
// RFC 7143 Section 11.3.
type SCSICommand struct {
	Header
	Read                       bool
	Write                      bool
	Attr                       uint8 // task attributes (bits 2-0 of flags byte)
	ExpectedDataTransferLength uint32
	CmdSN                      uint32
	ExpStatSN                  uint32
	CDB                        [16]byte
	ImmediateData              []byte
}

func (*SCSICommand) Opcode() OpCode       { return OpSCSICommand }
func (p *SCSICommand) DataSegment() []byte   { return p.ImmediateData }
func (p *SCSICommand) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpSCSICommand
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	// Flags in byte 1: Final already set by marshalHeader, add R/W/Attr
	if p.Read {
		bhs[1] |= 0x40
	}
	if p.Write {
		bhs[1] |= 0x20
	}
	bhs[1] |= p.Attr & 0x07
	binary.BigEndian.PutUint32(bhs[20:24], p.ExpectedDataTransferLength)
	binary.BigEndian.PutUint32(bhs[24:28], p.CmdSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpStatSN)
	copy(bhs[32:48], p.CDB[:])
	return bhs, nil
}
func (p *SCSICommand) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Read = bhs[1]&0x40 != 0
	p.Write = bhs[1]&0x20 != 0
	p.Attr = bhs[1] & 0x07
	p.ExpectedDataTransferLength = binary.BigEndian.Uint32(bhs[20:24])
	p.CmdSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpStatSN = binary.BigEndian.Uint32(bhs[28:32])
	copy(p.CDB[:], bhs[32:48])
}

// TaskMgmtReq represents an iSCSI Task Management Function Request PDU (opcode 0x02).
// RFC 7143 Section 11.5.
type TaskMgmtReq struct {
	Header
	Function           uint8 // bits 6-0 of byte 1
	ReferencedTaskTag  uint32
	CmdSN              uint32
	ExpStatSN          uint32
	RefCmdSN           uint32
}

func (*TaskMgmtReq) Opcode() OpCode       { return OpTaskMgmtReq }
func (*TaskMgmtReq) DataSegment() []byte   { return nil }
func (p *TaskMgmtReq) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpTaskMgmtReq
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	bhs[1] |= p.Function & 0x7f
	binary.BigEndian.PutUint32(bhs[20:24], p.ReferencedTaskTag)
	binary.BigEndian.PutUint32(bhs[24:28], p.CmdSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpStatSN)
	binary.BigEndian.PutUint32(bhs[32:36], p.RefCmdSN)
	return bhs, nil
}
func (p *TaskMgmtReq) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Function = bhs[1] & 0x7f
	p.ReferencedTaskTag = binary.BigEndian.Uint32(bhs[20:24])
	p.CmdSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpStatSN = binary.BigEndian.Uint32(bhs[28:32])
	p.RefCmdSN = binary.BigEndian.Uint32(bhs[32:36])
}

// LoginReq represents an iSCSI Login Request PDU (opcode 0x03).
// RFC 7143 Section 11.12.
type LoginReq struct {
	Header
	Transit    bool  // T bit (byte 1 bit 7)
	Continue   bool  // C bit (byte 1 bit 6)
	CSG        uint8 // Current Stage (byte 1 bits 3-2)
	NSG        uint8 // Next Stage (byte 1 bits 1-0)
	VersionMax uint8 // byte 2
	VersionMin uint8 // byte 3
	ISID       [6]byte // bytes 8-13
	TSIH       uint16  // bytes 14-15
	CID        uint16  // bytes 20-21
	CmdSN      uint32
	ExpStatSN  uint32
	Data       []byte // key-value pairs
}

func (*LoginReq) Opcode() OpCode       { return OpLoginReq }
func (p *LoginReq) DataSegment() []byte   { return p.Data }
func (p *LoginReq) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpLoginReq
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	// Byte 1: T, C, CSG, NSG (Final bit position is reused for T)
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
	bhs[3] = p.VersionMin
	// ISID in bytes 8-13 (overrides LUN field for login)
	copy(bhs[8:14], p.ISID[:])
	// TSIH in bytes 14-15
	binary.BigEndian.PutUint16(bhs[14:16], p.TSIH)
	binary.BigEndian.PutUint16(bhs[20:22], p.CID)
	binary.BigEndian.PutUint32(bhs[24:28], p.CmdSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpStatSN)
	return bhs, nil
}
func (p *LoginReq) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Transit = bhs[1]&0x80 != 0
	p.Continue = bhs[1]&0x40 != 0
	p.CSG = (bhs[1] >> 2) & 0x03
	p.NSG = bhs[1] & 0x03
	p.VersionMax = bhs[2]
	p.VersionMin = bhs[3]
	copy(p.ISID[:], bhs[8:14])
	p.TSIH = binary.BigEndian.Uint16(bhs[14:16])
	p.CID = binary.BigEndian.Uint16(bhs[20:22])
	p.CmdSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpStatSN = binary.BigEndian.Uint32(bhs[28:32])
}

// TextReq represents an iSCSI Text Request PDU (opcode 0x04).
// RFC 7143 Section 11.10.
type TextReq struct {
	Header
	Continue          bool // C bit (byte 1 bit 6)
	TargetTransferTag uint32
	CmdSN             uint32
	ExpStatSN         uint32
	Data              []byte // key-value pairs
}

func (*TextReq) Opcode() OpCode       { return OpTextReq }
func (p *TextReq) DataSegment() []byte   { return p.Data }
func (p *TextReq) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpTextReq
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	if p.Continue {
		bhs[1] |= 0x40
	}
	binary.BigEndian.PutUint32(bhs[20:24], p.TargetTransferTag)
	binary.BigEndian.PutUint32(bhs[24:28], p.CmdSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpStatSN)
	return bhs, nil
}
func (p *TextReq) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Continue = bhs[1]&0x40 != 0
	p.TargetTransferTag = binary.BigEndian.Uint32(bhs[20:24])
	p.CmdSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpStatSN = binary.BigEndian.Uint32(bhs[28:32])
}

// DataOut represents an iSCSI Data-Out PDU (opcode 0x05).
// RFC 7143 Section 11.7.
type DataOut struct {
	Header
	TargetTransferTag uint32
	ExpStatSN         uint32
	DataSN            uint32
	BufferOffset      uint32
	Data              []byte // Write data
}

func (*DataOut) Opcode() OpCode       { return OpDataOut }
func (p *DataOut) DataSegment() []byte   { return p.Data }
func (p *DataOut) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpDataOut
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	binary.BigEndian.PutUint32(bhs[20:24], p.TargetTransferTag)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpStatSN)
	binary.BigEndian.PutUint32(bhs[36:40], p.DataSN)
	binary.BigEndian.PutUint32(bhs[40:44], p.BufferOffset)
	return bhs, nil
}
func (p *DataOut) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.TargetTransferTag = binary.BigEndian.Uint32(bhs[20:24])
	p.ExpStatSN = binary.BigEndian.Uint32(bhs[28:32])
	p.DataSN = binary.BigEndian.Uint32(bhs[36:40])
	p.BufferOffset = binary.BigEndian.Uint32(bhs[40:44])
}

// LogoutReq represents an iSCSI Logout Request PDU (opcode 0x06).
// RFC 7143 Section 11.14.
type LogoutReq struct {
	Header
	ReasonCode uint8 // bits 6-0 of byte 1
	CID        uint16
	CmdSN      uint32
	ExpStatSN  uint32
}

func (*LogoutReq) Opcode() OpCode       { return OpLogoutReq }
func (*LogoutReq) DataSegment() []byte   { return nil }
func (p *LogoutReq) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpLogoutReq
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	bhs[1] |= p.ReasonCode & 0x7f
	binary.BigEndian.PutUint16(bhs[20:22], p.CID)
	binary.BigEndian.PutUint32(bhs[24:28], p.CmdSN)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpStatSN)
	return bhs, nil
}
func (p *LogoutReq) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.ReasonCode = bhs[1] & 0x7f
	p.CID = binary.BigEndian.Uint16(bhs[20:22])
	p.CmdSN = binary.BigEndian.Uint32(bhs[24:28])
	p.ExpStatSN = binary.BigEndian.Uint32(bhs[28:32])
}

// SNACKReq represents an iSCSI SNACK Request PDU (opcode 0x10).
// RFC 7143 Section 11.16.
type SNACKReq struct {
	Header
	Type              uint8 // bits 3-0 of byte 1
	TargetTransferTag uint32
	ExpStatSN         uint32
	BegRun            uint32
	RunLength         uint32
}

func (*SNACKReq) Opcode() OpCode       { return OpSNACKReq }
func (*SNACKReq) DataSegment() []byte   { return nil }
func (p *SNACKReq) MarshalBHS() ([BHSLength]byte, error) {
	var bhs [BHSLength]byte
	p.OpCode_ = OpSNACKReq
	if err := p.marshalHeader(bhs[:]); err != nil {
		return bhs, err
	}
	bhs[1] |= p.Type & 0x0f
	binary.BigEndian.PutUint32(bhs[20:24], p.TargetTransferTag)
	binary.BigEndian.PutUint32(bhs[28:32], p.ExpStatSN)
	binary.BigEndian.PutUint32(bhs[40:44], p.BegRun)
	binary.BigEndian.PutUint32(bhs[44:48], p.RunLength)
	return bhs, nil
}
func (p *SNACKReq) UnmarshalBHS(bhs [BHSLength]byte) {
	p.unmarshalHeader(bhs)
	p.Type = bhs[1] & 0x0f
	p.TargetTransferTag = binary.BigEndian.Uint32(bhs[20:24])
	p.ExpStatSN = binary.BigEndian.Uint32(bhs[28:32])
	p.BegRun = binary.BigEndian.Uint32(bhs[40:44])
	p.RunLength = binary.BigEndian.Uint32(bhs[44:48])
}
