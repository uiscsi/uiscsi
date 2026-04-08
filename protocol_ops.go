package uiscsi

import "context"

// ProtocolOps provides low-level iSCSI protocol methods.
// Obtain via [Session.Protocol].
//
// Most callers should use [Session.Close] for session teardown.
// These methods are for implementors who need explicit control over
// RFC 7143 PDU exchanges.
type ProtocolOps struct {
	s *Session
}

// Logout performs a graceful session logout. It waits for in-flight
// commands to complete, then exchanges Logout/LogoutResp PDUs with the
// target before shutting down. Per RFC 7143 Section 11.14. Most callers
// should use [Session.Close] instead, which calls Logout internally.
func (o *ProtocolOps) Logout(ctx context.Context) error {
	return o.s.s.Logout(ctx)
}

// SendExpStatSNConfirmation sends a NOP-Out that confirms ExpStatSN to the
// target without expecting a response. Per RFC 7143 Section 11.18:
// ITT=0xFFFFFFFF (no response), TTT=0xFFFFFFFF, Immediate=true.
// CmdSN is carried but NOT advanced. This is an advanced operation; most
// callers do not need it.
func (o *ProtocolOps) SendExpStatSNConfirmation() error {
	return o.s.s.SendExpStatSNConfirmation()
}
