// Package login implements the iSCSI login phase protocol including text
// key-value encoding, parameter negotiation, and error handling per RFC 7143.
package login

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// iSCSI login stages per RFC 7143 Section 11.12.
const (
	stageSecurityNegotiation uint8 = 0
	stageOperationalNeg      uint8 = 1
	stageFullFeaturePhase    uint8 = 3
)

// LoginOption configures the Login function via functional options.
type LoginOption func(*loginConfig)

// loginConfig holds all parameters for a login attempt.
type loginConfig struct {
	targetName    string
	sessionType   string // "Normal" or "Discovery"
	initiatorName string
	chapUser     string
	chapSecret   string
	mutualCHAP   bool
	targetSecret string
	headerDigest []string // preference list
	dataDigest   []string // preference list
	isid         [6]byte
	tsih         uint16 // non-zero for session reinstatement
	logger       *slog.Logger
}

// WithTarget sets the target IQN for the login.
func WithTarget(iqn string) LoginOption {
	return func(c *loginConfig) {
		c.targetName = iqn
	}
}

// WithCHAP enables CHAP authentication with the given credentials.
func WithCHAP(user, secret string) LoginOption {
	return func(c *loginConfig) {
		c.chapUser = user
		c.chapSecret = secret
	}
}

// WithMutualCHAP enables mutual CHAP authentication. The target must also
// authenticate itself using targetSecret.
func WithMutualCHAP(user, secret, targetSecret string) LoginOption {
	return func(c *loginConfig) {
		c.chapUser = user
		c.chapSecret = secret
		c.mutualCHAP = true
		c.targetSecret = targetSecret
	}
}

// WithHeaderDigest sets the header digest preference list (e.g., "CRC32C", "None").
func WithHeaderDigest(prefs ...string) LoginOption {
	return func(c *loginConfig) {
		c.headerDigest = prefs
	}
}

// WithDataDigest sets the data digest preference list (e.g., "CRC32C", "None").
func WithDataDigest(prefs ...string) LoginOption {
	return func(c *loginConfig) {
		c.dataDigest = prefs
	}
}

// WithInitiatorName overrides the default initiator IQN.
func WithInitiatorName(iqn string) LoginOption {
	return func(c *loginConfig) {
		c.initiatorName = iqn
	}
}

// WithSessionType sets the session type ("Normal" or "Discovery").
func WithSessionType(st string) LoginOption {
	return func(c *loginConfig) {
		c.sessionType = st
	}
}

// WithISID sets the ISID for session identification.
func WithISID(isid [6]byte) LoginOption {
	return func(c *loginConfig) {
		c.isid = isid
	}
}

// WithLoginLogger overrides the default slog.Logger for login diagnostics.
func WithLoginLogger(l *slog.Logger) LoginOption {
	return func(c *loginConfig) {
		c.logger = l
	}
}

// WithTSIH sets the TSIH for session reinstatement during reconnect.
// A non-zero TSIH tells the target this is a session reinstatement
// (same ISID, previously negotiated TSIH) per RFC 7143 Section 6.3.5.
func WithTSIH(tsih uint16) LoginOption {
	return func(c *loginConfig) {
		c.tsih = tsih
	}
}

// Login performs the iSCSI login phase on an already-established transport
// connection. It negotiates security (AuthMethod=None or CHAP) and
// operational parameters, returning the negotiated parameters on success.
//
// After a successful login, tc is configured with the negotiated digest
// settings and MaxRecvDataSegmentLength.
func Login(ctx context.Context, tc *transport.Conn, opts ...LoginOption) (*NegotiatedParams, error) {
	cfg := &loginConfig{
		sessionType:   "Normal",
		initiatorName: "iqn.2026-03.com.github.rkujawa:uiscsi",
		headerDigest:  []string{"None"},
		dataDigest:    []string{"None"},
	}
	for _, o := range opts {
		o(cfg)
	}

	// Default logger if none provided.
	if cfg.logger == nil {
		cfg.logger = slog.Default()
	}

	// Generate random ISID if not provided (type 0x40 = random per RFC 7143 Section 11.12.5).
	var zeroISID [6]byte
	if cfg.isid == zeroISID {
		if _, err := rand.Read(cfg.isid[1:]); err != nil {
			return nil, fmt.Errorf("login: generate ISID: %w", err)
		}
		cfg.isid[0] = 0x40 // random type
	}

	ls := &loginState{
		conn:      tc.NetConn(),
		cfg:       cfg,
		params:    &NegotiatedParams{},
		csg:       stageSecurityNegotiation,
		cmdSN:     1,
		expStatSN: 0,
		isid:      cfg.isid,
		tsih:      cfg.tsih,
		logger:    cfg.logger,
	}

	// Initialize CHAP state if credentials provided.
	if cfg.chapUser != "" {
		ls.chap = newCHAPState(cfg.chapUser, cfg.chapSecret, cfg.mutualCHAP, cfg.targetSecret)
	}

	if err := ls.run(ctx); err != nil {
		return nil, err
	}

	// Configure transport after successful login (Pitfall 6: only after login).
	tc.SetDigests(ls.params.HeaderDigest, ls.params.DataDigest)
	tc.SetMaxRecvDSL(ls.params.MaxRecvDataSegmentLength)

	// Hand off post-login sequence numbers and session ID to the session layer.
	ls.params.CmdSN = ls.cmdSN
	ls.params.ExpStatSN = ls.expStatSN
	ls.params.ISID = ls.isid

	return ls.params, nil
}

// loginState holds the mutable state for a login exchange.
type loginState struct {
	conn      net.Conn   // raw TCP, not pumps (Pitfall 5)
	cfg       *loginConfig
	params    *NegotiatedParams
	chap      *chapState // nil if AuthMethod=None
	csg       uint8      // current stage
	cmdSN     uint32     // set once, not incremented during login (Pitfall 10)
	expStatSN uint32     // tracks target's StatSN (Pitfall 9)
	tsih      uint16     // 0 for new session
	isid      [6]byte
	logger    *slog.Logger
}

// run drives the login state machine through SecurityNeg -> OperationalNeg -> FullFeature.
func (ls *loginState) run(ctx context.Context) error {
	ls.logger.Info("login: started",
		"target", ls.cfg.targetName,
		"session_type", ls.cfg.sessionType)

	// Stage 0: Security Negotiation
	if err := ls.doSecurityNegotiation(ctx); err != nil {
		return err
	}

	// Stage 1: Operational Negotiation
	if err := ls.doOperationalNegotiation(ctx); err != nil {
		return err
	}

	ls.logger.Info("login: complete",
		"header_digest", ls.params.HeaderDigest,
		"data_digest", ls.params.DataDigest)

	return nil
}

// doSecurityNegotiation performs stage 0 (security negotiation).
func (ls *loginState) doSecurityNegotiation(ctx context.Context) error {
	ls.logger.Info("login: stage transition",
		"from", "start",
		"to", "security_negotiation")

	if ls.chap == nil {
		// AuthMethod=None: single PDU, transit to operational.
		keys := []KeyValue{
			{Key: "InitiatorName", Value: ls.cfg.initiatorName},
			{Key: "SessionType", Value: ls.cfg.sessionType},
			{Key: "AuthMethod", Value: "None"},
		}
		if ls.cfg.targetName != "" {
			keys = append(keys, KeyValue{Key: "TargetName", Value: ls.cfg.targetName})
		}

		resp, err := ls.sendLogin(ctx, keys, true, stageSecurityNegotiation, stageOperationalNeg)
		if err != nil {
			return err
		}
		if resp.StatusClass != 0 {
			return &LoginError{
				StatusClass:  resp.StatusClass,
				StatusDetail: resp.StatusDetail,
				Message:      statusMessage(resp.StatusClass, resp.StatusDetail),
			}
		}

		// If target says transit to FFP directly (no operational stage), handle it.
		if resp.Transit && resp.NSG == stageFullFeaturePhase {
			ls.logger.Info("login: stage transition",
				"from", "security_negotiation",
				"to", "full_feature_phase",
				"skipped", "operational_negotiation")
			*ls.params = Defaults()
			ls.params.TSIH = resp.TSIH
			ls.params.TargetName = ls.cfg.targetName
			return nil
		}
		ls.logger.Info("login: stage transition",
			"from", "security_negotiation",
			"to", "operational_negotiation")
		return nil
	}

	// CHAP authentication: 3-round-trip exchange per RFC 7143 Section 12.1.3.
	//
	// Round 1: Propose AuthMethod=CHAP (no CHAP_A — sent separately in round 2).
	// LIO and other targets require CHAP_A in a separate PDU after AuthMethod
	// is confirmed; combining them causes immediate rejection.
	keys := []KeyValue{
		{Key: "InitiatorName", Value: ls.cfg.initiatorName},
		{Key: "SessionType", Value: ls.cfg.sessionType},
		{Key: "AuthMethod", Value: "CHAP"},
	}
	if ls.cfg.targetName != "" {
		keys = append(keys, KeyValue{Key: "TargetName", Value: ls.cfg.targetName})
	}

	resp, err := ls.sendLogin(ctx, keys, false, stageSecurityNegotiation, stageSecurityNegotiation)
	if err != nil {
		return err
	}
	if resp.StatusClass != 0 {
		return &LoginError{
			StatusClass:  resp.StatusClass,
			StatusDetail: resp.StatusDetail,
			Message:      statusMessage(resp.StatusClass, resp.StatusDetail),
		}
	}

	// Round 2: Send CHAP_A=5 (MD5 algorithm selection).
	// Target responds with CHAP_A, CHAP_I (identifier), CHAP_C (challenge).
	algoKeys := []KeyValue{
		{Key: "CHAP_A", Value: "5"},
	}
	resp, err = ls.sendLogin(ctx, algoKeys, false, stageSecurityNegotiation, stageSecurityNegotiation)
	if err != nil {
		return err
	}
	if resp.StatusClass != 0 {
		return &LoginError{
			StatusClass:  resp.StatusClass,
			StatusDetail: resp.StatusDetail,
			Message:      statusMessage(resp.StatusClass, resp.StatusDetail),
		}
	}

	// Parse target's CHAP challenge from response.
	respKeys := kvToMap(DecodeTextKV(resp.Data))

	// Round 3: Process challenge and send CHAP response.
	chapResp, err := ls.chap.processChallenge(respKeys)
	if err != nil {
		return fmt.Errorf("login: CHAP: %w", err)
	}

	// Build response keys.
	var chapKeys []KeyValue
	for k, v := range chapResp {
		chapKeys = append(chapKeys, KeyValue{Key: k, Value: v})
	}

	// Send CHAP response with transit to operational.
	resp, err = ls.sendLogin(ctx, chapKeys, true, stageSecurityNegotiation, stageOperationalNeg)
	if err != nil {
		return err
	}
	if resp.StatusClass != 0 {
		return &LoginError{
			StatusClass:  resp.StatusClass,
			StatusDetail: resp.StatusDetail,
			Message:      statusMessage(resp.StatusClass, resp.StatusDetail),
		}
	}

	// Verify mutual CHAP response if requested.
	if ls.chap.isMutual {
		mutualKeys := kvToMap(DecodeTextKV(resp.Data))
		if err := ls.chap.verifyMutualResponse(mutualKeys); err != nil {
			return fmt.Errorf("login: mutual CHAP: %w", err)
		}
	}

	// If target says transit to FFP directly, handle it.
	if resp.Transit && resp.NSG == stageFullFeaturePhase {
		ls.logger.Info("login: stage transition",
			"from", "security_negotiation",
			"to", "full_feature_phase",
			"skipped", "operational_negotiation")
		*ls.params = Defaults()
		ls.params.TSIH = resp.TSIH
		ls.params.TargetName = ls.cfg.targetName
		return nil
	}

	ls.logger.Info("login: stage transition",
		"from", "security_negotiation",
		"to", "operational_negotiation")

	return nil
}

// doOperationalNegotiation performs stage 1 (operational parameter negotiation).
func (ls *loginState) doOperationalNegotiation(ctx context.Context) error {
	ls.logger.Info("login: stage transition",
		"from", "operational_negotiation",
		"to", "full_feature_phase")

	// Start with defaults.
	*ls.params = Defaults()

	// Build operational keys to propose.
	keys := buildInitiatorKeys(ls.cfg)

	resp, err := ls.sendLogin(ctx, keys, true, stageOperationalNeg, stageFullFeaturePhase)
	if err != nil {
		return err
	}
	if resp.StatusClass != 0 {
		return &LoginError{
			StatusClass:  resp.StatusClass,
			StatusDetail: resp.StatusDetail,
			Message:      statusMessage(resp.StatusClass, resp.StatusDetail),
		}
	}

	// Parse target responses and resolve each key.
	targetKVs := DecodeTextKV(resp.Data)
	targetMap := kvToMap(targetKVs)

	// Our proposed values for resolution.
	initiatorMap := kvToMap(keys)

	var resolved []KeyValue
	for _, kv := range targetKVs {
		def, ok := findKeyDef(kv.Key)
		if !ok {
			continue // skip unknown keys
		}
		initiatorVal := initiatorMap[kv.Key]
		result, err := resolveKey(def, initiatorVal, kv.Value)
		if err != nil {
			return fmt.Errorf("login: negotiate %s: %w", kv.Key, err)
		}
		resolved = append(resolved, KeyValue{Key: kv.Key, Value: result})
	}

	// For keys we proposed but target did not respond to, keep defaults.
	// For MaxRecvDataSegmentLength: if target declared it, use target's value.
	if mrdsl, ok := targetMap["MaxRecvDataSegmentLength"]; ok {
		found := false
		for i, kv := range resolved {
			if kv.Key == "MaxRecvDataSegmentLength" {
				resolved[i].Value = mrdsl // declarative: use target's value
				found = true
				break
			}
		}
		if !found {
			resolved = append(resolved, KeyValue{Key: "MaxRecvDataSegmentLength", Value: mrdsl})
		}
	}

	applyNegotiatedKeys(ls.params, resolved)

	// Store TSIH from response.
	ls.params.TSIH = resp.TSIH
	ls.params.TargetName = ls.cfg.targetName

	return nil
}

// sendLogin builds and sends a LoginReq PDU, reads the LoginResp.
func (ls *loginState) sendLogin(ctx context.Context, keys []KeyValue, transit bool, csg, nsg uint8) (*pdu.LoginResp, error) {
	data := EncodeTextKV(keys)

	req := &pdu.LoginReq{
		Header: pdu.Header{
			Final:          true, // login PDUs always have F=1
			DataSegmentLen: uint32(len(data)),
		},
		Transit:    transit,
		CSG:        csg,
		NSG:        nsg,
		VersionMax: 0x00,
		VersionMin: 0x00,
		ISID:       ls.isid,
		TSIH:       ls.tsih,
		CmdSN:      ls.cmdSN,
		ExpStatSN:  ls.expStatSN,
		Data:       data,
	}

	// Set deadline from context.
	if dl, ok := ctx.Deadline(); ok {
		if err := ls.conn.SetDeadline(dl); err != nil {
			return nil, fmt.Errorf("login: set deadline: %w", err)
		}
	}

	// Encode and write the request PDU directly (not via pumps -- Pitfall 5).
	encoded, err := pdu.EncodePDU(req)
	if err != nil {
		return nil, fmt.Errorf("login: encode PDU: %w", err)
	}

	raw := &transport.RawPDU{}
	copy(raw.BHS[:], encoded[:pdu.BHSLength])
	if len(encoded) > pdu.BHSLength {
		raw.DataSegment = encoded[pdu.BHSLength:]
	}

	if err := transport.WriteRawPDU(ls.conn, raw); err != nil {
		return nil, fmt.Errorf("login: write PDU: %w", err)
	}

	// Read response -- no digests during login (Pitfall 6).
	respRaw, err := transport.ReadRawPDU(ls.conn, false, false)
	if err != nil {
		return nil, fmt.Errorf("login: read PDU: %w", err)
	}

	// Decode LoginResp.
	resp := &pdu.LoginResp{}
	resp.UnmarshalBHS(respRaw.BHS)
	resp.Data = respRaw.DataSegment

	// Update expStatSN from response (Pitfall 9).
	ls.expStatSN = resp.StatSN + 1

	// Do NOT increment CmdSN during login (Pitfall 10).

	return resp, nil
}

// buildInitiatorKeys constructs the operational parameter key-value pairs
// that the initiator proposes during login negotiation.
func buildInitiatorKeys(cfg *loginConfig) []KeyValue {
	return []KeyValue{
		{Key: "HeaderDigest", Value: strings.Join(cfg.headerDigest, ",")},
		{Key: "DataDigest", Value: strings.Join(cfg.dataDigest, ",")},
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
}

// findKeyDef looks up a key definition from the registry by name.
func findKeyDef(name string) (KeyDef, bool) {
	for _, def := range keyRegistry {
		if def.Name == name {
			return def, true
		}
	}
	return KeyDef{}, false
}

// kvToMap converts a slice of KeyValue pairs to a map for quick lookup.
func kvToMap(pairs []KeyValue) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		m[p.Key] = p.Value
	}
	return m
}
