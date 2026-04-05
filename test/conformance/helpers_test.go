package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	"github.com/rkujawa/uiscsi/internal/pdu"
	testutil "github.com/rkujawa/uiscsi/test"
	"github.com/rkujawa/uiscsi/test/pducapture"
)

// writeTestSetup encapsulates the common boilerplate for write-path conformance
// tests: a MockTarget with login/logout/NOP-Out handlers and a PDU Recorder.
type writeTestSetup struct {
	Target   *testutil.MockTarget
	Recorder *pducapture.Recorder
}

// newWriteTestSetup creates a MockTarget configured with the given negotiation
// parameters, registers HandleLogin/HandleLogout/HandleNOPOut, and creates a
// PDU Recorder. The target is closed automatically via t.Cleanup.
func newWriteTestSetup(t *testing.T, negCfg testutil.NegotiationConfig) *writeTestSetup {
	t.Helper()

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.SetNegotiationConfig(negCfg)
	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	return &writeTestSetup{
		Target:   tgt,
		Recorder: &pducapture.Recorder{},
	}
}

// dialWithOverrides dials the MockTarget with the PDU hook, a 30s keepalive
// interval, and the given operational overrides map. The session is closed
// automatically via t.Cleanup.
func (s *writeTestSetup) dialWithOverrides(t *testing.T, ctx context.Context, overrides map[string]string) *uiscsi.Session {
	t.Helper()

	sess, err := uiscsi.Dial(ctx, s.Target.Addr(),
		uiscsi.WithPDUHook(s.Recorder.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
		uiscsi.WithOperationalOverrides(overrides),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	return sess
}

// negotiationOverrides converts the boolean fields of a NegotiationConfig into
// the string map format expected by WithOperationalOverrides. Nil fields are
// omitted from the result.
func negotiationOverrides(cfg testutil.NegotiationConfig) map[string]string {
	m := make(map[string]string)
	if cfg.ImmediateData != nil {
		if *cfg.ImmediateData {
			m["ImmediateData"] = "Yes"
		} else {
			m["ImmediateData"] = "No"
		}
	}
	if cfg.InitialR2T != nil {
		if *cfg.InitialR2T {
			m["InitialR2T"] = "Yes"
		} else {
			m["InitialR2T"] = "No"
		}
	}
	return m
}

// sendR2TAndConsume sends an R2T for the specified offset/length and reads
// Data-Out PDUs until the F-bit. It returns the current ExpCmdSN/MaxCmdSN
// from the target session state for use in subsequent response PDUs.
func sendR2TAndConsume(tc *testutil.TargetConn, tgt *testutil.MockTarget, cmd *pdu.SCSICommand, ttt uint32, offset uint32, length uint32) (expCmdSN, maxCmdSN uint32, err error) {
	expCmdSN, maxCmdSN = tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

	r2t := &pdu.R2T{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: cmd.InitiatorTaskTag,
		},
		TargetTransferTag:         ttt,
		StatSN:                    tc.StatSN(),
		ExpCmdSN:                  expCmdSN,
		MaxCmdSN:                  maxCmdSN,
		R2TSN:                     0,
		BufferOffset:              offset,
		DesiredDataTransferLength: length,
	}
	if err := tc.SendPDU(r2t); err != nil {
		return 0, 0, err
	}

	if _, err := testutil.ReadDataOutPDUs(tc); err != nil {
		return 0, 0, err
	}

	return expCmdSN, maxCmdSN, nil
}

// sendSCSIResponse sends a standard success (status 0x00) SCSI Response PDU
// for the given command with the provided ExpCmdSN/MaxCmdSN.
func sendSCSIResponse(tc *testutil.TargetConn, cmd *pdu.SCSICommand, expCmdSN, maxCmdSN uint32) error {
	resp := &pdu.SCSIResponse{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: cmd.InitiatorTaskTag,
		},
		Status:   0x00,
		StatSN:   tc.NextStatSN(),
		ExpCmdSN: expCmdSN,
		MaxCmdSN: maxCmdSN,
	}
	return tc.SendPDU(resp)
}
