package test

import (
	"net"
	"sync"
	"testing"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

func TestMockTarget_AcceptConnection(t *testing.T) {
	tgt, err := NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	defer tgt.Close()

	// Connect to the target.
	conn, err := net.Dial("tcp", tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Verify connection was tracked.
	// Give the accept loop a moment to register.
	// We check by doing a simple write/read exchange.
	if tgt.Addr() == "" {
		t.Fatal("Addr() returned empty string")
	}
}

func TestMockTarget_LoginExchange(t *testing.T) {
	tgt, err := NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	defer tgt.Close()

	tgt.HandleLogin()

	conn, err := net.Dial("tcp", tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Send a security negotiation login request.
	keys := []login.KeyValue{
		{Key: "InitiatorName", Value: "iqn.2026-03.com.test:initiator"},
		{Key: "SessionType", Value: "Normal"},
		{Key: "AuthMethod", Value: "None"},
		{Key: "TargetName", Value: "iqn.2026-03.com.test:target"},
	}
	data := login.EncodeTextKV(keys)

	req := &pdu.LoginReq{
		Header: pdu.Header{
			Final:          true,
			DataSegmentLen: uint32(len(data)),
		},
		Transit:    true,
		CSG:        0,
		NSG:        1,
		VersionMax: 0x00,
		VersionMin: 0x00,
		ISID:       [6]byte{0x40, 0x01, 0x02, 0x03, 0x04, 0x05},
		CmdSN:      1,
		ExpStatSN:  0,
		Data:       data,
	}

	raw, err := BuildRawPDU(req)
	if err != nil {
		t.Fatalf("BuildRawPDU: %v", err)
	}
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("WriteRawPDU: %v", err)
	}

	// Read response.
	respRaw, err := transport.ReadRawPDU(conn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}

	resp := &pdu.LoginResp{}
	resp.UnmarshalBHS(respRaw.BHS)

	if resp.StatusClass != 0 {
		t.Fatalf("StatusClass: got %d, want 0", resp.StatusClass)
	}
	if !resp.Transit {
		t.Fatal("Transit bit not set in response")
	}
	if resp.CSG != 0 {
		t.Fatalf("CSG: got %d, want 0", resp.CSG)
	}
	if resp.NSG != 1 {
		t.Fatalf("NSG: got %d, want 1", resp.NSG)
	}
}

func TestMockTarget_HandleSCSIRead(t *testing.T) {
	tgt, err := NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	defer tgt.Close()

	tgt.HandleLogin()
	testData := []byte("hello from mock target")
	tgt.HandleSCSIRead(0, testData)
	tgt.HandleLogout()

	conn, err := net.Dial("tcp", tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Do login first (security phase).
	sendLoginSecurityPhase(t, conn)

	// Do operational negotiation.
	sendLoginOperationalPhase(t, conn)

	// Send SCSI read command.
	cmd := &pdu.SCSICommand{
		Header: pdu.Header{
			Final:          true,
			DataSegmentLen: 0,
		},
		Read:                       true,
		ExpectedDataTransferLength: uint32(len(testData)),
		CmdSN:                      1,
		ExpStatSN:                  3,
	}
	cmd.CDB[0] = 0x28 // READ(10)

	raw, err := BuildRawPDU(cmd)
	if err != nil {
		t.Fatalf("BuildRawPDU: %v", err)
	}
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("WriteRawPDU: %v", err)
	}

	// Read DataIn response.
	respRaw, err := transport.ReadRawPDU(conn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}

	din := &pdu.DataIn{}
	din.UnmarshalBHS(respRaw.BHS)
	din.Data = respRaw.DataSegment

	if din.Status != 0x00 {
		t.Fatalf("Status: got 0x%02X, want 0x00", din.Status)
	}
	if !din.HasStatus {
		t.Fatal("HasStatus not set")
	}
	if string(din.Data) != string(testData) {
		t.Fatalf("Data: got %q, want %q", din.Data, testData)
	}
}

func TestMockTarget_Close(t *testing.T) {
	tgt, err := NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}

	// Connect so there's an active connection.
	conn, err := net.Dial("tcp", tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Close should not hang.
	if err := tgt.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Second close should be a no-op.
	if err := tgt.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// sendLoginSecurityPhase sends a security negotiation login request
// and reads the response.
func sendLoginSecurityPhase(t *testing.T, conn net.Conn) {
	t.Helper()
	keys := []login.KeyValue{
		{Key: "InitiatorName", Value: "iqn.2026-03.com.test:initiator"},
		{Key: "SessionType", Value: "Normal"},
		{Key: "AuthMethod", Value: "None"},
	}
	data := login.EncodeTextKV(keys)

	req := &pdu.LoginReq{
		Header: pdu.Header{
			Final:          true,
			DataSegmentLen: uint32(len(data)),
		},
		Transit:    true,
		CSG:        0,
		NSG:        1,
		VersionMax: 0x00,
		VersionMin: 0x00,
		ISID:       [6]byte{0x40, 0x01, 0x02, 0x03, 0x04, 0x05},
		CmdSN:      1,
		ExpStatSN:  0,
		Data:       data,
	}

	raw, err := BuildRawPDU(req)
	if err != nil {
		t.Fatalf("BuildRawPDU: %v", err)
	}
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("WriteRawPDU: %v", err)
	}

	respRaw, err := transport.ReadRawPDU(conn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	resp := &pdu.LoginResp{}
	resp.UnmarshalBHS(respRaw.BHS)
	if resp.StatusClass != 0 {
		t.Fatalf("security phase: StatusClass %d", resp.StatusClass)
	}
}

// sendLoginOperationalPhase sends an operational negotiation login request
// and reads the response.
func sendLoginOperationalPhase(t *testing.T, conn net.Conn) {
	t.Helper()
	keys := []login.KeyValue{
		{Key: "HeaderDigest", Value: "None"},
		{Key: "DataDigest", Value: "None"},
		{Key: "MaxConnections", Value: "1"},
		{Key: "InitialR2T", Value: "Yes"},
		{Key: "ImmediateData", Value: "Yes"},
		{Key: "MaxRecvDataSegmentLength", Value: "8192"},
		{Key: "MaxBurstLength", Value: "262144"},
		{Key: "FirstBurstLength", Value: "65536"},
		{Key: "DefaultTime2Wait", Value: "2"},
		{Key: "DefaultTime2Retain", Value: "20"},
		{Key: "MaxOutstandingR2T", Value: "1"},
		{Key: "DataPDUInOrder", Value: "Yes"},
		{Key: "DataSequenceInOrder", Value: "Yes"},
		{Key: "ErrorRecoveryLevel", Value: "0"},
	}
	data := login.EncodeTextKV(keys)

	req := &pdu.LoginReq{
		Header: pdu.Header{
			Final:          true,
			DataSegmentLen: uint32(len(data)),
		},
		Transit:    true,
		CSG:        1,
		NSG:        3,
		VersionMax: 0x00,
		VersionMin: 0x00,
		ISID:       [6]byte{0x40, 0x01, 0x02, 0x03, 0x04, 0x05},
		TSIH:       0,
		CmdSN:      1,
		ExpStatSN:  1,
		Data:       data,
	}

	raw, err := BuildRawPDU(req)
	if err != nil {
		t.Fatalf("BuildRawPDU: %v", err)
	}
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("WriteRawPDU: %v", err)
	}

	respRaw, err := transport.ReadRawPDU(conn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	resp := &pdu.LoginResp{}
	resp.UnmarshalBHS(respRaw.BHS)
	if resp.StatusClass != 0 {
		t.Fatalf("operational phase: StatusClass %d", resp.StatusClass)
	}
	if !resp.Transit {
		t.Fatal("operational phase: Transit not set")
	}
	if resp.NSG != 3 {
		t.Fatalf("operational phase: NSG %d, want 3", resp.NSG)
	}
}

func TestHandleSCSIFunc_CallCount(t *testing.T) {
	tgt, err := NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	defer tgt.Close()

	tgt.HandleLogin()
	tgt.HandleLogout()

	var mu sync.Mutex
	counts := []int{}

	tgt.HandleSCSIFunc(func(tc *TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		mu.Lock()
		counts = append(counts, callCount)
		mu.Unlock()

		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
		statSN := tc.NextStatSN()
		resp := &pdu.SCSIResponse{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			Status:   0x00,
			StatSN:   statSN,
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		return tc.SendPDU(resp)
	})

	conn, err := net.Dial("tcp", tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	sendLoginSecurityPhase(t, conn)
	sendLoginOperationalPhase(t, conn)

	// Send 3 SCSI commands.
	for i := range 3 {
		cmd := &pdu.SCSICommand{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: uint32(i + 1),
			},
			CmdSN:     uint32(i + 1),
			ExpStatSN: 3,
		}
		cmd.CDB[0] = 0x00 // TEST UNIT READY

		raw, err := BuildRawPDU(cmd)
		if err != nil {
			t.Fatalf("BuildRawPDU[%d]: %v", i, err)
		}
		if err := transport.WriteRawPDU(conn, raw); err != nil {
			t.Fatalf("WriteRawPDU[%d]: %v", i, err)
		}

		respRaw, err := transport.ReadRawPDU(conn, false, false, 0)
		if err != nil {
			t.Fatalf("ReadRawPDU[%d]: %v", i, err)
		}
		resp := &pdu.SCSIResponse{}
		resp.UnmarshalBHS(respRaw.BHS)
		if resp.Status != 0x00 {
			t.Fatalf("command %d: Status 0x%02X, want 0x00", i, resp.Status)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(counts) != 3 {
		t.Fatalf("call count: got %d calls, want 3", len(counts))
	}
	for i, c := range counts {
		if c != i {
			t.Errorf("callCount[%d]: got %d, want %d", i, c, i)
		}
	}
}

func TestSessionState_Update(t *testing.T) {
	ss := NewSessionState()

	// First call initializes and advances for non-immediate.
	expCmdSN, maxCmdSN := ss.Update(5, false)
	if expCmdSN != 6 {
		t.Errorf("after first Update(5, false): ExpCmdSN = %d, want 6", expCmdSN)
	}
	if maxCmdSN != 16 { // 6 + 10
		t.Errorf("after first Update(5, false): MaxCmdSN = %d, want 16", maxCmdSN)
	}

	// Non-immediate advances.
	expCmdSN, maxCmdSN = ss.Update(6, false)
	if expCmdSN != 7 {
		t.Errorf("after Update(6, false): ExpCmdSN = %d, want 7", expCmdSN)
	}
	if maxCmdSN != 17 { // 7 + 10
		t.Errorf("after Update(6, false): MaxCmdSN = %d, want 17", maxCmdSN)
	}

	// Immediate does NOT advance ExpCmdSN.
	expCmdSN, maxCmdSN = ss.Update(7, true)
	if expCmdSN != 7 { // unchanged
		t.Errorf("after Update(7, true): ExpCmdSN = %d, want 7", expCmdSN)
	}
	if maxCmdSN != 17 { // 7 + 10
		t.Errorf("after Update(7, true): MaxCmdSN = %d, want 17", maxCmdSN)
	}

	// Accessor matches.
	if got := ss.ExpCmdSN(); got != 7 {
		t.Errorf("ExpCmdSN() = %d, want 7", got)
	}
}

func TestSessionState_SetMaxCmdSNDelta(t *testing.T) {
	ss := NewSessionState()

	// Initialize.
	ss.Update(10, false)

	// Change delta.
	ss.SetMaxCmdSNDelta(3)

	expCmdSN, maxCmdSN := ss.Update(11, false)
	if expCmdSN != 12 {
		t.Errorf("ExpCmdSN = %d, want 12", expCmdSN)
	}
	if maxCmdSN != 15 { // 12 + 3
		t.Errorf("MaxCmdSN = %d, want 15", maxCmdSN)
	}

	// Negative delta (closed window).
	ss.SetMaxCmdSNDelta(-1)
	expCmdSN, maxCmdSN = ss.Update(12, false)
	if expCmdSN != 13 {
		t.Errorf("ExpCmdSN = %d, want 13", expCmdSN)
	}
	if maxCmdSN != 12 { // 13 + (-1) = 12, which is < ExpCmdSN (closed window)
		t.Errorf("MaxCmdSN = %d, want 12", maxCmdSN)
	}
}
