package session

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

func TestParseSendTargetsResponse(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []DiscoveryTarget
	}{
		{
			name: "single target single portal",
			data: []byte("TargetName=iqn.2001-04.com.example:storage1\x00TargetAddress=10.0.0.1:3260,1\x00"),
			want: []DiscoveryTarget{
				{
					Name: "iqn.2001-04.com.example:storage1",
					Portals: []Portal{
						{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
					},
				},
			},
		},
		{
			name: "single target multiple portals",
			data: []byte("TargetName=iqn.2001-04.com.example:storage1\x00TargetAddress=10.0.0.1:3260,1\x00TargetAddress=10.0.0.2:3260,2\x00"),
			want: []DiscoveryTarget{
				{
					Name: "iqn.2001-04.com.example:storage1",
					Portals: []Portal{
						{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
						{Address: "10.0.0.2", Port: 3260, GroupTag: 2},
					},
				},
			},
		},
		{
			name: "multiple targets",
			data: []byte("TargetName=iqn.2001-04.com.example:storage1\x00TargetAddress=10.0.0.1:3260,1\x00TargetName=iqn.2001-04.com.example:storage2\x00TargetAddress=10.0.0.2:3260,1\x00TargetAddress=10.0.0.3:3261,2\x00"),
			want: []DiscoveryTarget{
				{
					Name: "iqn.2001-04.com.example:storage1",
					Portals: []Portal{
						{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
					},
				},
				{
					Name: "iqn.2001-04.com.example:storage2",
					Portals: []Portal{
						{Address: "10.0.0.2", Port: 3260, GroupTag: 1},
						{Address: "10.0.0.3", Port: 3261, GroupTag: 2},
					},
				},
			},
		},
		{
			name: "empty input",
			data: nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSendTargetsResponse(tt.data)
			if len(got) != len(tt.want) {
				t.Fatalf("parseSendTargetsResponse() returned %d targets, want %d", len(got), len(tt.want))
			}
			for i, target := range got {
				if target.Name != tt.want[i].Name {
					t.Errorf("target[%d].Name = %q, want %q", i, target.Name, tt.want[i].Name)
				}
				if len(target.Portals) != len(tt.want[i].Portals) {
					t.Fatalf("target[%d].Portals has %d entries, want %d", i, len(target.Portals), len(tt.want[i].Portals))
				}
				for j, portal := range target.Portals {
					wantP := tt.want[i].Portals[j]
					if portal.Address != wantP.Address {
						t.Errorf("target[%d].Portals[%d].Address = %q, want %q", i, j, portal.Address, wantP.Address)
					}
					if portal.Port != wantP.Port {
						t.Errorf("target[%d].Portals[%d].Port = %d, want %d", i, j, portal.Port, wantP.Port)
					}
					if portal.GroupTag != wantP.GroupTag {
						t.Errorf("target[%d].Portals[%d].GroupTag = %d, want %d", i, j, portal.GroupTag, wantP.GroupTag)
					}
				}
			}
		})
	}
}

func TestParsePortal(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Portal
	}{
		{
			name: "IPv4 with port and group tag",
			raw:  "10.0.0.1:3260,1",
			want: Portal{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
		},
		{
			name: "IPv6 with port and group tag",
			raw:  "[2001:db8::1]:3260,1",
			want: Portal{Address: "2001:db8::1", Port: 3260, GroupTag: 1},
		},
		{
			name: "no port defaults to 3260",
			raw:  "10.0.0.1,1",
			want: Portal{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
		},
		{
			name: "no group tag defaults to 1",
			raw:  "10.0.0.1:3260",
			want: Portal{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePortal(tt.raw)
			if got.Address != tt.want.Address {
				t.Errorf("parsePortal(%q).Address = %q, want %q", tt.raw, got.Address, tt.want.Address)
			}
			if got.Port != tt.want.Port {
				t.Errorf("parsePortal(%q).Port = %d, want %d", tt.raw, got.Port, tt.want.Port)
			}
			if got.GroupTag != tt.want.GroupTag {
				t.Errorf("parsePortal(%q).GroupTag = %d, want %d", tt.raw, got.GroupTag, tt.want.GroupTag)
			}
		})
	}
}

// writeTextRespPDU builds and writes a TextResp PDU to the conn.
func writeTextRespPDU(t *testing.T, conn net.Conn, resp *pdu.TextResp) {
	t.Helper()
	resp.Header.OpCode_ = pdu.OpTextResp
	resp.Header.Final = !resp.Continue
	resp.Header.DataSegmentLen = uint32(len(resp.Data))
	raw := buildRawPDU(t, resp)
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("write TextResp: %v", err)
	}
}

// readTextReqPDU reads and decodes a TextReq PDU from the conn.
func readTextReqPDU(t *testing.T, conn net.Conn) *pdu.TextReq {
	t.Helper()
	raw, err := transport.ReadRawPDU(conn, false, false)
	if err != nil {
		t.Fatalf("read TextReq: %v", err)
	}
	req := &pdu.TextReq{}
	req.UnmarshalBHS(raw.BHS)
	req.Data = raw.DataSegment
	return req
}

func TestSendTargetsSingleResponse(t *testing.T) {
	sess, targetConn := newTestSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	responseData := []byte("TargetName=iqn.2001-04.com.example:disk1\x00TargetAddress=10.0.0.1:3260,1\x00TargetName=iqn.2001-04.com.example:disk2\x00TargetAddress=10.0.0.2:3260,1\x00")

	// Mock target goroutine: read TextReq, send TextResp with Final=true.
	go func() {
		req := readTextReqPDU(t, targetConn)
		writeTextRespPDU(t, targetConn, &pdu.TextResp{
			Header: pdu.Header{
				InitiatorTaskTag: req.InitiatorTaskTag,
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            1,
			ExpCmdSN:          req.CmdSN + 1,
			MaxCmdSN:          req.CmdSN + 10,
			Data:              responseData,
		})
	}()

	targets, err := sess.SendTargets(ctx)
	if err != nil {
		t.Fatalf("SendTargets: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Name != "iqn.2001-04.com.example:disk1" {
		t.Errorf("target[0].Name = %q, want iqn.2001-04.com.example:disk1", targets[0].Name)
	}
	if targets[1].Name != "iqn.2001-04.com.example:disk2" {
		t.Errorf("target[1].Name = %q, want iqn.2001-04.com.example:disk2", targets[1].Name)
	}
	if len(targets[0].Portals) != 1 || targets[0].Portals[0].Address != "10.0.0.1" {
		t.Errorf("target[0] portal mismatch: %+v", targets[0].Portals)
	}
}

func TestSendTargetsContinuation(t *testing.T) {
	sess, targetConn := newTestSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Split response across two PDUs to test C-bit continuation (Pitfall 6).
	part1 := []byte("TargetName=iqn.2001-04.com.example:disk1\x00TargetAddress=10.0.0.1:3260,1\x00")
	part2 := []byte("TargetName=iqn.2001-04.com.example:disk2\x00TargetAddress=10.0.0.2:3260,1\x00")

	go func() {
		// Read initial TextReq.
		req := readTextReqPDU(t, targetConn)

		// Send first TextResp with Continue=true.
		writeTextRespPDU(t, targetConn, &pdu.TextResp{
			Header: pdu.Header{
				InitiatorTaskTag: req.InitiatorTaskTag,
			},
			Continue:          true,
			TargetTransferTag: 0x00000042, // TTT for continuation
			StatSN:            1,
			ExpCmdSN:          req.CmdSN + 1,
			MaxCmdSN:          req.CmdSN + 10,
			Data:              part1,
		})

		// Read continuation TextReq (should echo TTT).
		contReq := readTextReqPDU(t, targetConn)
		if contReq.TargetTransferTag != 0x00000042 {
			t.Errorf("continuation TTT = 0x%08X, want 0x00000042", contReq.TargetTransferTag)
		}

		// Send final TextResp.
		writeTextRespPDU(t, targetConn, &pdu.TextResp{
			Header: pdu.Header{
				InitiatorTaskTag: contReq.InitiatorTaskTag,
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            2,
			ExpCmdSN:          contReq.CmdSN + 1,
			MaxCmdSN:          contReq.CmdSN + 10,
			Data:              part2,
		})
	}()

	targets, err := sess.SendTargets(ctx)
	if err != nil {
		t.Fatalf("SendTargets: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Name != "iqn.2001-04.com.example:disk1" {
		t.Errorf("target[0].Name = %q", targets[0].Name)
	}
	if targets[1].Name != "iqn.2001-04.com.example:disk2" {
		t.Errorf("target[1].Name = %q", targets[1].Name)
	}
}

func TestSendTargetsEmpty(t *testing.T) {
	sess, targetConn := newTestSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		req := readTextReqPDU(t, targetConn)
		writeTextRespPDU(t, targetConn, &pdu.TextResp{
			Header: pdu.Header{
				InitiatorTaskTag: req.InitiatorTaskTag,
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            1,
			ExpCmdSN:          req.CmdSN + 1,
			MaxCmdSN:          req.CmdSN + 10,
		})
	}()

	targets, err := sess.SendTargets(ctx)
	if err != nil {
		t.Fatalf("SendTargets: %v", err)
	}

	if len(targets) != 0 {
		t.Fatalf("expected 0 targets, got %d", len(targets))
	}
}

func TestDiscoverIntegration(t *testing.T) {
	// Start a mock iSCSI target on loopback that handles:
	// 1. Login (discovery session)
	// 2. SendTargets TextReq/TextResp
	// 3. Logout

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	targetData := []byte("TargetName=iqn.2001-04.com.example:storage1\x00TargetAddress=10.0.0.1:3260,1\x00")

	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()

		// Phase 1: Handle login PDUs.
		// Read LoginReq (security negotiation).
		loginRaw, readErr := transport.ReadRawPDU(conn, false, false)
		if readErr != nil {
			t.Logf("mock target: read login: %v", readErr)
			return
		}
		loginReq := &pdu.LoginReq{}
		loginReq.UnmarshalBHS(loginRaw.BHS)

		// Respond with login success, transit to operational.
		loginResp := &pdu.LoginResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: loginReq.InitiatorTaskTag,
			},
			Transit:       true,
			CSG:           0,
			NSG:           1,
			VersionMax:    0,
			VersionActive: 0,
			ISID:          loginReq.ISID,
			TSIH:          1,
			StatSN:        0,
			ExpCmdSN:      1,
			MaxCmdSN:      10,
		}
		loginResp.Header.OpCode_ = pdu.OpLoginResp
		loginResp.Header.DataSegmentLen = 0
		loginBHS, _ := loginResp.MarshalBHS()
		if writeErr := transport.WriteRawPDU(conn, &transport.RawPDU{BHS: loginBHS}); writeErr != nil {
			t.Logf("mock target: write login resp: %v", writeErr)
			return
		}

		// Read operational negotiation LoginReq.
		opRaw, readErr := transport.ReadRawPDU(conn, false, false)
		if readErr != nil {
			t.Logf("mock target: read op login: %v", readErr)
			return
		}
		opReq := &pdu.LoginReq{}
		opReq.UnmarshalBHS(opRaw.BHS)

		// Respond with operational parameters and transit to FFP.
		opRespData := login.EncodeTextKV([]login.KeyValue{
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
		})
		opResp := &pdu.LoginResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: opReq.InitiatorTaskTag,
				DataSegmentLen:   uint32(len(opRespData)),
			},
			Transit:       true,
			CSG:           1,
			NSG:           3,
			VersionMax:    0,
			VersionActive: 0,
			ISID:          opReq.ISID,
			TSIH:          1,
			StatSN:        1,
			ExpCmdSN:      1,
			MaxCmdSN:      10,
			Data:          opRespData,
		}
		opResp.Header.OpCode_ = pdu.OpLoginResp
		opBHS, _ := opResp.MarshalBHS()
		opPDU := &transport.RawPDU{BHS: opBHS}
		if len(opRespData) > 0 {
			opPDU.DataSegment = opRespData
		}
		if writeErr := transport.WriteRawPDU(conn, opPDU); writeErr != nil {
			t.Logf("mock target: write op resp: %v", writeErr)
			return
		}

		// Phase 2: Handle SendTargets TextReq.
		textRaw, readErr := transport.ReadRawPDU(conn, false, false)
		if readErr != nil {
			t.Logf("mock target: read text req: %v", readErr)
			return
		}
		textReq := &pdu.TextReq{}
		textReq.UnmarshalBHS(textRaw.BHS)

		textResp := &pdu.TextResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: textReq.InitiatorTaskTag,
				DataSegmentLen:   uint32(len(targetData)),
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            2,
			ExpCmdSN:          textReq.CmdSN + 1,
			MaxCmdSN:          textReq.CmdSN + 10,
			Data:              targetData,
		}
		textResp.Header.OpCode_ = pdu.OpTextResp
		textBHS, _ := textResp.MarshalBHS()
		textPDU := &transport.RawPDU{BHS: textBHS}
		if len(targetData) > 0 {
			textPDU.DataSegment = targetData
		}
		if writeErr := transport.WriteRawPDU(conn, textPDU); writeErr != nil {
			t.Logf("mock target: write text resp: %v", writeErr)
			return
		}

		// Phase 3: Handle Logout.
		logoutRaw, readErr := transport.ReadRawPDU(conn, false, false)
		if readErr != nil {
			t.Logf("mock target: read logout: %v", readErr)
			return
		}
		logoutReq := &pdu.LogoutReq{}
		logoutReq.UnmarshalBHS(logoutRaw.BHS)

		logoutResp := &pdu.LogoutResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: logoutReq.InitiatorTaskTag,
			},
			Response: 0, // success
			StatSN:   3,
			ExpCmdSN: logoutReq.CmdSN + 1,
			MaxCmdSN: logoutReq.CmdSN + 10,
		}
		logoutResp.Header.OpCode_ = pdu.OpLogoutResp
		logoutBHS, _ := logoutResp.MarshalBHS()
		if writeErr := transport.WriteRawPDU(conn, &transport.RawPDU{BHS: logoutBHS}); writeErr != nil {
			t.Logf("mock target: write logout resp: %v", writeErr)
			return
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	targets, discoverErr := Discover(ctx, listener.Addr().String())
	if discoverErr != nil {
		t.Fatalf("Discover: %v", discoverErr)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Name != "iqn.2001-04.com.example:storage1" {
		t.Errorf("target[0].Name = %q, want iqn.2001-04.com.example:storage1", targets[0].Name)
	}
	if len(targets[0].Portals) != 1 {
		t.Fatalf("expected 1 portal, got %d", len(targets[0].Portals))
	}
	if targets[0].Portals[0].Address != "10.0.0.1" {
		t.Errorf("portal address = %q, want 10.0.0.1", targets[0].Portals[0].Address)
	}
}
