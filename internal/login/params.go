package login

// NegotiatedParams holds the resolved values for all mandatory iSCSI
// operational parameters after login negotiation (RFC 7143 Section 13).
// Fields are typed (not map[string]string) for compile-time safety.
type NegotiatedParams struct {
	HeaderDigest             bool
	DataDigest               bool
	MaxConnections           uint32
	InitialR2T               bool
	ImmediateData            bool
	MaxRecvDataSegmentLength uint32
	MaxBurstLength           uint32
	FirstBurstLength         uint32
	DefaultTime2Wait         uint32
	DefaultTime2Retain       uint32
	MaxOutstandingR2T        uint32
	DataPDUInOrder           bool
	DataSequenceInOrder      bool
	ErrorRecoveryLevel       uint32
	TargetName               string
	TSIH                     uint16
	ISID                     [6]byte
	CmdSN                    uint32 // Post-login CmdSN for session layer handoff
	ExpStatSN                uint32 // Post-login ExpStatSN for session layer handoff
}

// Defaults returns a NegotiatedParams populated with the RFC 7143
// default values for all mandatory operational parameters.
func Defaults() NegotiatedParams {
	return NegotiatedParams{
		HeaderDigest:             false,
		DataDigest:               false,
		MaxConnections:           1,
		InitialR2T:               true,
		ImmediateData:            true,
		MaxRecvDataSegmentLength: 8192,
		MaxBurstLength:           262144,
		FirstBurstLength:         65536,
		DefaultTime2Wait:         2,
		DefaultTime2Retain:       20,
		MaxOutstandingR2T:        1,
		DataPDUInOrder:           true,
		DataSequenceInOrder:      true,
		ErrorRecoveryLevel:       0,
		CmdSN:                    1,
		ExpStatSN:                0,
	}
}
