package login

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// captureHandler is a slog.Handler that records all log entries for test assertions.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r)
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler       { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler             { return h }

// hasMessage checks if any captured record has the given message substring.
func (h *captureHandler) hasMessage(msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if contains(r.Message, msg) {
			return true
		}
	}
	return false
}

// hasLevelMessage checks if any captured record has the given level and message substring.
func (h *captureHandler) hasLevelMessage(level slog.Level, msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Level == level && contains(r.Message, msg) {
			return true
		}
	}
	return false
}

// mockTargetConfig configures the mock iSCSI target for testing.
type mockTargetConfig struct {
	authMethod   string // "None" or "CHAP"
	chapUser     string // expected CHAP username
	chapSecret   string // initiator's secret (for verifying CHAP_R)
	mutualUser   string // target's CHAP username
	mutualSecret string // target's secret (for mutual CHAP_R)
	headerDigest string // "None" or "CRC32C"
	dataDigest   string // "None" or "CRC32C"
	statusClass  uint8  // override to force error responses
	statusDetail uint8
	tsih         uint16 // assigned TSIH

	// operational param overrides
	maxBurstLength   uint32
	firstBurstLength uint32

	// If set, use a wrong mutual CHAP response to test failure.
	wrongMutualResp bool
}

// runMockTarget runs a minimal iSCSI target that handles login PDU exchange.
func runMockTarget(t *testing.T, ln net.Listener, cfg mockTargetConfig) {
	t.Helper()
	conn, err := ln.Accept()
	if err != nil {
		t.Logf("mock target: accept error: %v", err)
		return
	}
	defer conn.Close()

	var statSN uint32

	for {
		// Read incoming login request.
		raw, err := transport.ReadRawPDU(conn, false, false)
		if err != nil {
			t.Logf("mock target: read error: %v", err)
			return
		}

		req := &pdu.LoginReq{}
		req.UnmarshalBHS(raw.BHS)
		req.Data = raw.DataSegment
		keys := kvToMap(DecodeTextKV(req.Data))

		// Force error if configured.
		if cfg.statusClass != 0 {
			sendMockLoginResp(t, conn, req, nil, statSN, cfg.tsih, cfg.statusClass, cfg.statusDetail, false, req.CSG, req.CSG)
			return
		}

		switch req.CSG {
		case stageSecurityNegotiation:
			if cfg.authMethod == "" || cfg.authMethod == "None" {
				// AuthMethod=None: accept and transit to operational.
				respKeys := []KeyValue{
					{Key: "AuthMethod", Value: "None"},
				}
				sendMockLoginResp(t, conn, req, respKeys, statSN, cfg.tsih, 0, 0, true, stageSecurityNegotiation, stageOperationalNeg)
				statSN++
				continue
			}

			// CHAP authentication: 3-round-trip flow per RFC 7143.
			if _, hasAuthMethod := keys["AuthMethod"]; hasAuthMethod {
				// Round 1: Confirm AuthMethod=CHAP.
				authRespKeys := []KeyValue{
					{Key: "AuthMethod", Value: "CHAP"},
				}
				sendMockLoginResp(t, conn, req, authRespKeys, statSN, 0, 0, 0, false, stageSecurityNegotiation, stageSecurityNegotiation)
				statSN++

				// Round 2: Read CHAP_A, send challenge.
				rawAlgo, err := transport.ReadRawPDU(conn, false, false)
				if err != nil {
					t.Logf("mock target: read CHAP_A: %v", err)
					return
				}
				reqAlgo := &pdu.LoginReq{}
				reqAlgo.UnmarshalBHS(rawAlgo.BHS)

				var idBuf [1]byte
				if _, err := rand.Read(idBuf[:]); err != nil {
					t.Errorf("mock target: rand: %v", err)
					return
				}
				challenge := make([]byte, 16)
				if _, err := rand.Read(challenge); err != nil {
					t.Errorf("mock target: rand: %v", err)
					return
				}

				respKeys := []KeyValue{
					{Key: "CHAP_A", Value: "5"},
					{Key: "CHAP_I", Value: strconv.Itoa(int(idBuf[0]))},
					{Key: "CHAP_C", Value: encodeCHAPBinary(challenge)},
				}
				sendMockLoginResp(t, conn, reqAlgo, respKeys, statSN, 0, 0, 0, false, stageSecurityNegotiation, stageSecurityNegotiation)
				statSN++

				// Round 3: Read CHAP response from initiator.
				raw2, err := transport.ReadRawPDU(conn, false, false)
				if err != nil {
					t.Logf("mock target: read CHAP response: %v", err)
					return
				}
				req2 := &pdu.LoginReq{}
				req2.UnmarshalBHS(raw2.BHS)
				req2.Data = raw2.DataSegment
				chapKeys := kvToMap(DecodeTextKV(req2.Data))

				// Verify CHAP response.
				expectedResp := chapResponse(idBuf[0], []byte(cfg.chapSecret), challenge)
				gotResp, err := decodeCHAPBinary(chapKeys["CHAP_R"])
				if err != nil || len(gotResp) != 16 {
					sendMockLoginResp(t, conn, req2, nil, statSN, 0, 2, 1, false, stageSecurityNegotiation, stageSecurityNegotiation)
					statSN++
					return
				}

				match := true
				for i := range 16 {
					if gotResp[i] != expectedResp[i] {
						match = false
						break
					}
				}

				if !match {
					// Auth failure.
					sendMockLoginResp(t, conn, req2, nil, statSN, 0, 2, 1, false, stageSecurityNegotiation, stageSecurityNegotiation)
					statSN++
					return
				}

				// Build mutual CHAP response if requested.
				var mutualRespKeys []KeyValue
				if cfg.mutualSecret != "" {
					// Parse initiator's challenge.
					iIDStr := chapKeys["CHAP_I"]
					iCStr := chapKeys["CHAP_C"]
					if iIDStr != "" && iCStr != "" {
						iID, _ := strconv.Atoi(iIDStr)
						iChallenge, _ := decodeCHAPBinary(iCStr)

						var mutualResp [16]byte
						if cfg.wrongMutualResp {
							// Send wrong response for testing.
							if _, err := rand.Read(mutualResp[:]); err != nil {
								t.Errorf("mock target: rand: %v", err)
								return
							}
						} else {
							mutualResp = chapResponse(byte(iID), []byte(cfg.mutualSecret), iChallenge)
						}

						mutualRespKeys = []KeyValue{
							{Key: "CHAP_N", Value: cfg.mutualUser},
							{Key: "CHAP_R", Value: encodeCHAPBinary(mutualResp[:])},
						}
					}
				}

				sendMockLoginResp(t, conn, req2, mutualRespKeys, statSN, cfg.tsih, 0, 0, true, stageSecurityNegotiation, stageOperationalNeg)
				statSN++
				continue
			}

		case stageOperationalNeg:
			// Build operational response keys.
			respKeys := buildOperationalResponse(cfg)
			sendMockLoginResp(t, conn, req, respKeys, statSN, cfg.tsih, 0, 0, true, stageOperationalNeg, stageFullFeaturePhase)
			statSN++
			return // login complete
		}
	}
}

// buildOperationalResponse builds the target's operational parameter response.
func buildOperationalResponse(cfg mockTargetConfig) []KeyValue {
	hd := cfg.headerDigest
	if hd == "" {
		hd = "None"
	}
	dd := cfg.dataDigest
	if dd == "" {
		dd = "None"
	}
	mbl := cfg.maxBurstLength
	if mbl == 0 {
		mbl = 262144
	}
	fbl := cfg.firstBurstLength
	if fbl == 0 {
		fbl = 65536
	}

	return []KeyValue{
		{Key: "HeaderDigest", Value: hd},
		{Key: "DataDigest", Value: dd},
		{Key: "MaxConnections", Value: "1"},
		{Key: "InitialR2T", Value: "Yes"},
		{Key: "ImmediateData", Value: "Yes"},
		{Key: "MaxRecvDataSegmentLength", Value: "8192"},
		{Key: "MaxBurstLength", Value: fmt.Sprintf("%d", mbl)},
		{Key: "FirstBurstLength", Value: fmt.Sprintf("%d", fbl)},
		{Key: "DefaultTime2Wait", Value: "2"},
		{Key: "DefaultTime2Retain", Value: "20"},
		{Key: "MaxOutstandingR2T", Value: "1"},
		{Key: "DataPDUInOrder", Value: "Yes"},
		{Key: "DataSequenceInOrder", Value: "Yes"},
		{Key: "ErrorRecoveryLevel", Value: "0"},
	}
}

// sendMockLoginResp builds and sends a LoginResp PDU from the mock target.
func sendMockLoginResp(t *testing.T, conn net.Conn, req *pdu.LoginReq, keys []KeyValue, statSN uint32, tsih uint16, statusClass, statusDetail uint8, transit bool, csg, nsg uint8) {
	t.Helper()

	data := EncodeTextKV(keys)

	resp := &pdu.LoginResp{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: req.Header.InitiatorTaskTag,
			DataSegmentLen:   uint32(len(data)),
		},
		Transit:       transit,
		CSG:           csg,
		NSG:           nsg,
		VersionMax:    0x00,
		VersionActive: 0x00,
		ISID:          req.ISID,
		TSIH:          tsih,
		StatSN:        statSN,
		ExpCmdSN:      req.CmdSN,
		MaxCmdSN:      req.CmdSN + 1,
		StatusClass:   statusClass,
		StatusDetail:  statusDetail,
		Data:          data,
	}

	encoded, err := pdu.EncodePDU(resp)
	if err != nil {
		t.Errorf("mock target: encode response: %v", err)
		return
	}

	raw := &transport.RawPDU{}
	copy(raw.BHS[:], encoded[:pdu.BHSLength])
	if len(encoded) > pdu.BHSLength {
		raw.DataSegment = encoded[pdu.BHSLength:]
	}

	// Fix DataSegmentLength in BHS -- EncodePDU includes padding, but we
	// need the raw data segment without padding for RawPDU.
	binary.BigEndian.PutUint16(raw.BHS[6:8], uint16(len(data)))
	raw.BHS[5] = byte(len(data) >> 16)
	raw.DataSegment = data

	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Errorf("mock target: write response: %v", err)
		return
	}
}

// startMockTarget creates a TCP listener, starts the mock target in a goroutine,
// and returns the listener address for the client to connect to.
func startMockTarget(t *testing.T, cfg mockTargetConfig) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go runMockTarget(t, ln, cfg)
	return ln.Addr().String()
}

func TestLoginAuthNone(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod: "None",
		tsih:       0x1234,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	params, err := Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if params == nil {
		t.Fatal("Login returned nil params")
	}
	if params.TSIH != 0x1234 {
		t.Errorf("TSIH = 0x%04x, want 0x1234", params.TSIH)
	}
	if params.HeaderDigest {
		t.Error("HeaderDigest should be false")
	}
	if params.DataDigest {
		t.Error("DataDigest should be false")
	}
	if params.MaxBurstLength != 262144 {
		t.Errorf("MaxBurstLength = %d, want 262144", params.MaxBurstLength)
	}
}

func TestLoginCHAP(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod: "CHAP",
		chapUser:   "initiator",
		chapSecret: "secret123",
		tsih:       0x5678,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	params, err := Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
		WithCHAP("initiator", "secret123"),
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if params == nil {
		t.Fatal("Login returned nil params")
	}
	if params.TSIH != 0x5678 {
		t.Errorf("TSIH = 0x%04x, want 0x5678", params.TSIH)
	}
}

func TestLoginCHAPWrongPassword(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod: "CHAP",
		chapUser:   "initiator",
		chapSecret: "correct_password",
		tsih:       0xABCD,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	_, err = Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
		WithCHAP("initiator", "wrong_password"),
	)
	if err == nil {
		t.Fatal("Login should have failed with wrong password")
	}

	var loginErr *LoginError
	if !errors.As(err, &loginErr) {
		t.Fatalf("expected *LoginError, got %T: %v", err, err)
	}
	if loginErr.StatusClass != 2 {
		t.Errorf("StatusClass = %d, want 2", loginErr.StatusClass)
	}
	if loginErr.StatusDetail != 1 {
		t.Errorf("StatusDetail = %d, want 1", loginErr.StatusDetail)
	}
}

func TestLoginMutualCHAP(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod:   "CHAP",
		chapUser:     "initiator",
		chapSecret:   "isecret",
		mutualUser:   "target",
		mutualSecret: "tsecret",
		tsih:         0x9999,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	params, err := Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
		WithMutualCHAP("initiator", "isecret", "tsecret"),
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if params == nil {
		t.Fatal("Login returned nil params")
	}
	if params.TSIH != 0x9999 {
		t.Errorf("TSIH = 0x%04x, want 0x9999", params.TSIH)
	}
}

func TestLoginMutualCHAPTargetAuthFail(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod:      "CHAP",
		chapUser:        "initiator",
		chapSecret:      "isecret",
		mutualUser:      "target",
		mutualSecret:    "tsecret",
		wrongMutualResp: true,
		tsih:            0x7777,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	_, err = Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
		WithMutualCHAP("initiator", "isecret", "tsecret"),
	)
	if err == nil {
		t.Fatal("Login should have failed with wrong mutual CHAP response")
	}
	if got := err.Error(); !contains(got, "mutual") {
		t.Errorf("error should mention mutual auth, got: %v", err)
	}
}

func TestLoginDigestNegotiation(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod:   "None",
		headerDigest: "CRC32C",
		dataDigest:   "None",
		tsih:         0x2222,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	params, err := Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
		WithHeaderDigest("CRC32C", "None"),
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !params.HeaderDigest {
		t.Error("HeaderDigest should be true after CRC32C negotiation")
	}
	if params.DataDigest {
		t.Error("DataDigest should be false")
	}
}

func TestLoginDigestBothCRC32C(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod:   "None",
		headerDigest: "CRC32C",
		dataDigest:   "CRC32C",
		tsih:         0x3333,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	params, err := Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
		WithHeaderDigest("CRC32C", "None"),
		WithDataDigest("CRC32C", "None"),
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !params.HeaderDigest {
		t.Error("HeaderDigest should be true")
	}
	if !params.DataDigest {
		t.Error("DataDigest should be true")
	}
}

func TestLoginCustomOperationalParams(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod:       "None",
		maxBurstLength:   131072,
		firstBurstLength: 32768,
		tsih:             0x4444,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	params, err := Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if params.MaxBurstLength != 131072 {
		t.Errorf("MaxBurstLength = %d, want 131072", params.MaxBurstLength)
	}
	if params.FirstBurstLength != 32768 {
		t.Errorf("FirstBurstLength = %d, want 32768", params.FirstBurstLength)
	}
}

func TestLoginTargetError(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod:   "None",
		statusClass:  3,
		statusDetail: 0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	_, err = Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
	)
	if err == nil {
		t.Fatal("Login should have failed with target error")
	}

	var loginErr *LoginError
	if !errors.As(err, &loginErr) {
		t.Fatalf("expected *LoginError, got %T: %v", err, err)
	}
	if loginErr.StatusClass != 3 {
		t.Errorf("StatusClass = %d, want 3", loginErr.StatusClass)
	}
}

func TestLoginContextCancellation(t *testing.T) {
	t.Parallel()

	// Start a listener that accepts but never responds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Hold connection open but never respond.
		defer conn.Close()
		<-time.After(30 * time.Second)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	tc, err := transport.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	_, err = Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
	)
	if err == nil {
		t.Fatal("Login should have failed with context cancellation/timeout")
	}
}

// contains checks if substr is present in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoginStageTransitionLogging(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod: "None",
		tsih:       0xAAAA,
	})

	handler := &captureHandler{}
	logger := slog.New(handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	params, err := Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
		WithLoginLogger(logger),
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if params == nil {
		t.Fatal("Login returned nil params")
	}

	// Verify login start was logged.
	if !handler.hasLevelMessage(slog.LevelInfo, "login: started") {
		t.Error("expected Info log with 'login: started'")
	}

	// Verify security negotiation stage transition was logged.
	if !handler.hasLevelMessage(slog.LevelInfo, "login: stage transition") {
		t.Error("expected Info log with 'login: stage transition'")
	}

	// Verify login complete was logged.
	if !handler.hasLevelMessage(slog.LevelInfo, "login: complete") {
		t.Error("expected Info log with 'login: complete'")
	}
}

func TestLoginStageTransitionLoggingCHAP(t *testing.T) {
	t.Parallel()

	addr := startMockTarget(t, mockTargetConfig{
		authMethod: "CHAP",
		chapUser:   "initiator",
		chapSecret: "secret123",
		tsih:       0xBBBB,
	})

	handler := &captureHandler{}
	logger := slog.New(handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer tc.Close()

	params, err := Login(ctx, tc,
		WithTarget("iqn.2026-03.com.test:target"),
		WithCHAP("initiator", "secret123"),
		WithLoginLogger(logger),
	)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if params == nil {
		t.Fatal("Login returned nil params")
	}

	// Verify all stage transitions logged for CHAP path.
	if !handler.hasLevelMessage(slog.LevelInfo, "login: started") {
		t.Error("expected Info log with 'login: started'")
	}
	if !handler.hasLevelMessage(slog.LevelInfo, "login: stage transition") {
		t.Error("expected Info log with 'login: stage transition'")
	}
	if !handler.hasLevelMessage(slog.LevelInfo, "login: complete") {
		t.Error("expected Info log with 'login: complete'")
	}
}
