package session

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// SendTargets sends a SendTargets text request on the session and returns
// the discovered targets. Per RFC 7143 Section 11.10, this uses the Text
// Request/Response PDU exchange. Multi-PDU responses with the Continue bit
// (C=1) are handled by sending continuation requests with the target's TTT.
func (s *Session) SendTargets(ctx context.Context) ([]DiscoveryTarget, error) {
	// Build SendTargets=All key-value data.
	data := login.EncodeTextKV([]login.KeyValue{
		{Key: "SendTargets", Value: "All"},
	})

	// Acquire CmdSN slot (text requests are non-immediate).
	cmdSN, err := s.window.acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("session: SendTargets: acquire CmdSN: %w", err)
	}

	// Allocate ITT and register persistent channel (may receive multiple PDUs for continuation).
	itt := s.router.AllocateITT()
	pduCh := s.router.RegisterPersistent(itt)
	defer s.router.Unregister(itt)

	// Build initial TextReq PDU.
	textReq := &pdu.TextReq{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
			DataSegmentLen:   uint32(len(data)),
		},
		TargetTransferTag: 0xFFFFFFFF, // initiator-initiated exchange
		CmdSN:             cmdSN,
		ExpStatSN:         s.getExpStatSN(),
		Data:              data,
	}

	bhs, err := textReq.MarshalBHS()
	if err != nil {
		return nil, fmt.Errorf("session: SendTargets: encode TextReq: %w", err)
	}

	raw := &transport.RawPDU{BHS: bhs}
	if len(data) > 0 {
		raw.DataSegment = data
	}
	s.stampDigests(raw)

	// Send to write pump.
	select {
	case s.writeCh <- raw:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Accumulate response data across potentially multiple TextResp PDUs.
	var accumulated []byte

	// Set timeout: use context deadline or default 30s.
	timeout := 30 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
	}

	for {
		timer := time.NewTimer(timeout)
		var respRaw *transport.RawPDU

		select {
		case respRaw = <-pduCh:
			timer.Stop()
		case <-timer.C:
			return nil, fmt.Errorf("session: SendTargets: timeout waiting for TextResp")
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}

		// Decode TextResp from raw PDU.
		decoded, decErr := pdu.DecodeBHS(respRaw.BHS)
		if decErr != nil {
			return nil, fmt.Errorf("session: SendTargets: decode TextResp: %w", decErr)
		}

		textResp, ok := decoded.(*pdu.TextResp)
		if !ok {
			return nil, fmt.Errorf("session: SendTargets: unexpected PDU type %T", decoded)
		}

		// Update session sequence numbers.
		s.window.update(textResp.ExpCmdSN, textResp.MaxCmdSN)
		s.updateStatSN(textResp.StatSN)

		// Accumulate data segment.
		textResp.Data = respRaw.DataSegment
		accumulated = append(accumulated, textResp.Data...)

		// If Continue bit is clear (Final), we have all data.
		if !textResp.Continue {
			break
		}

		// C-bit continuation (Pitfall 6): send another TextReq with TTT from response.
		contCmdSN, contErr := s.window.acquire(ctx)
		if contErr != nil {
			return nil, fmt.Errorf("session: SendTargets continuation: acquire CmdSN: %w", contErr)
		}

		contReq := &pdu.TextReq{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: itt,
			},
			TargetTransferTag: textResp.TargetTransferTag,
			CmdSN:             contCmdSN,
			ExpStatSN:         s.getExpStatSN(),
		}

		contBHS, contEncErr := contReq.MarshalBHS()
		if contEncErr != nil {
			return nil, fmt.Errorf("session: SendTargets continuation: encode TextReq: %w", contEncErr)
		}

		contRaw := &transport.RawPDU{BHS: contBHS}
		select {
		case s.writeCh <- contRaw:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return parseSendTargetsResponse(accumulated), nil
}

// Discover performs a complete SendTargets discovery against the iSCSI target
// at addr. It dials a TCP connection, performs a Discovery session login,
// issues SendTargets, logs out, and returns the discovered targets. This is
// a convenience function for one-shot target enumeration per design doc D-06.
func Discover(ctx context.Context, addr string, opts ...login.LoginOption) ([]DiscoveryTarget, error) {
	// Connect to target.
	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("discover: dial %s: %w", addr, err)
	}

	// Prepend WithSessionType("Discovery") so user opts can override if needed.
	allOpts := make([]login.LoginOption, 0, len(opts)+1)
	allOpts = append(allOpts, login.WithSessionType("Discovery"))
	allOpts = append(allOpts, opts...)

	// Perform discovery login.
	params, loginErr := login.Login(ctx, tc, allOpts...)
	if loginErr != nil {
		_ = tc.Close()
		return nil, fmt.Errorf("discover: login: %w", loginErr)
	}

	// Create session.
	sess := NewSession(tc, *params)

	// SendTargets.
	targets, stErr := sess.SendTargets(ctx)
	if stErr != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("discover: SendTargets: %w", stErr)
	}

	// Clean logout. If logout fails, just close and return targets anyway.
	// Logout errors are non-fatal for discovery sessions — we already have
	// the target list. Swallow the error intentionally.
	logoutErr := sess.Logout(ctx)
	_ = sess.Close()
	if logoutErr != nil {
		return targets, nil //nolint:nilerr // intentional: discovery succeeded; logout error is non-fatal
	}

	return targets, nil
}

// parseSendTargetsResponse parses the data segment of a SendTargets response
// into a slice of DiscoveryTarget. The data is null-delimited key-value pairs
// in the iSCSI text format. "TargetName" starts a new target; "TargetAddress"
// adds a portal to the current target.
func parseSendTargetsResponse(data []byte) []DiscoveryTarget {
	kvs := login.DecodeTextKV(data)
	if len(kvs) == 0 {
		return nil
	}

	var targets []DiscoveryTarget
	var current *DiscoveryTarget

	for _, kv := range kvs {
		switch kv.Key {
		case "TargetName":
			targets = append(targets, DiscoveryTarget{Name: kv.Value})
			current = &targets[len(targets)-1]
		case "TargetAddress":
			if current != nil {
				current.Portals = append(current.Portals, parsePortal(kv.Value))
			}
		}
	}

	return targets
}

// parsePortal parses an iSCSI target address string in the format
// "addr:port,tpgt" into a Portal. Handles IPv6 bracket notation
// "[addr]:port,tpgt". Defaults: port=3260, group tag=1.
func parsePortal(s string) Portal {
	p := Portal{
		Port:     3260,
		GroupTag: 1,
	}

	// Split off the group tag (after last comma).
	addrPort := s
	if idx := strings.LastIndex(s, ","); idx >= 0 {
		tag, err := strconv.Atoi(s[idx+1:])
		if err == nil {
			p.GroupTag = tag
		}
		addrPort = s[:idx]
	}

	// Handle IPv6 bracket notation: [addr]:port
	if strings.HasPrefix(addrPort, "[") {
		closeBracket := strings.Index(addrPort, "]")
		if closeBracket < 0 {
			// Malformed, treat whole thing as address.
			p.Address = addrPort
			return p
		}
		p.Address = addrPort[1:closeBracket]
		rest := addrPort[closeBracket+1:]
		if strings.HasPrefix(rest, ":") {
			port, err := strconv.Atoi(rest[1:])
			if err == nil {
				p.Port = port
			}
		}
		return p
	}

	// IPv4: split on last colon for port.
	if idx := strings.LastIndex(addrPort, ":"); idx >= 0 {
		port, err := strconv.Atoi(addrPort[idx+1:])
		if err == nil {
			p.Port = port
			p.Address = addrPort[:idx]
			return p
		}
	}

	// No port found, whole string is address.
	p.Address = addrPort
	return p
}
