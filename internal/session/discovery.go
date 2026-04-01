package session

import (
	"strconv"
	"strings"

	"github.com/rkujawa/uiscsi/internal/login"
)

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
