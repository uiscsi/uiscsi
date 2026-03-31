package login

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestChapResponse(t *testing.T) {
	tests := []struct {
		name      string
		id        byte
		secret    []byte
		challenge []byte
	}{
		{
			name:      "basic computation",
			id:        0x42,
			secret:    []byte("secret"),
			challenge: []byte{0x01, 0x02, 0x03},
		},
		{
			name:      "zero id",
			id:        0,
			secret:    []byte("password"),
			challenge: []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C},
		},
		{
			name:      "empty secret",
			id:        0xFF,
			secret:    []byte{},
			challenge: []byte{0xAA, 0xBB},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chapResponse(tc.id, tc.secret, tc.challenge)

			// Compute expected MD5(id || secret || challenge) independently.
			h := md5.New()
			h.Write([]byte{tc.id})
			h.Write(tc.secret)
			h.Write(tc.challenge)
			var expected [16]byte
			copy(expected[:], h.Sum(nil))

			if got != expected {
				t.Errorf("chapResponse(%d, %q, %x) = %x, want %x", tc.id, tc.secret, tc.challenge, got, expected)
			}
		})
	}
}

func TestEncodeCHAPBinary(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name:  "two bytes",
			input: []byte{0xAB, 0xCD},
			want:  "0xabcd",
		},
		{
			name:  "single byte",
			input: []byte{0x00},
			want:  "0x00",
		},
		{
			name:  "sixteen bytes",
			input: []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0xFE, 0xDC, 0xBA, 0x98, 0x76, 0x54, 0x32, 0x10},
			want:  "0x0123456789abcdeffedcba9876543210",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeCHAPBinary(tc.input)
			if got != tc.want {
				t.Errorf("encodeCHAPBinary(%x) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDecodeCHAPBinary(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []byte
		wantErr bool
	}{
		{
			name:  "lowercase 0x prefix",
			input: "0xABCD",
			want:  []byte{0xAB, 0xCD},
		},
		{
			name:  "uppercase 0X prefix",
			input: "0XABCD",
			want:  []byte{0xAB, 0xCD},
		},
		{
			name:  "base64 0b prefix",
			input: "0b" + base64.StdEncoding.EncodeToString([]byte{0xAB, 0xCD}),
			want:  []byte{0xAB, 0xCD},
		},
		{
			name:    "no recognized prefix",
			input:   "noprefx",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeCHAPBinary(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("decodeCHAPBinary(%q) expected error, got %x", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeCHAPBinary(%q) unexpected error: %v", tc.input, err)
			}
			if hex.EncodeToString(got) != hex.EncodeToString(tc.want) {
				t.Errorf("decodeCHAPBinary(%q) = %x, want %x", tc.input, got, tc.want)
			}
		})
	}
}

func TestCHAPExchangeOneWay(t *testing.T) {
	user := "testuser"
	secret := "testsecret"

	cs := newCHAPState(user, secret, false, "")

	// Simulate target sending CHAP_A, CHAP_I, CHAP_C.
	challengeBytes := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	targetKeys := map[string]string{
		"CHAP_A": "5",
		"CHAP_I": "42",
		"CHAP_C": encodeCHAPBinary(challengeBytes),
	}

	resp, err := cs.processChallenge(targetKeys)
	if err != nil {
		t.Fatalf("processChallenge returned error: %v", err)
	}

	// Verify CHAP_N is the username.
	if resp["CHAP_N"] != user {
		t.Errorf("CHAP_N = %q, want %q", resp["CHAP_N"], user)
	}

	// Verify CHAP_R is correct.
	expectedResp := chapResponse(42, []byte(secret), challengeBytes)
	expectedR := encodeCHAPBinary(expectedResp[:])
	if resp["CHAP_R"] != expectedR {
		t.Errorf("CHAP_R = %q, want %q", resp["CHAP_R"], expectedR)
	}

	// One-way: no initiator CHAP_I or CHAP_C in response.
	if _, ok := resp["CHAP_I"]; ok {
		t.Error("one-way CHAP should not include CHAP_I in response")
	}
	if _, ok := resp["CHAP_C"]; ok {
		t.Error("one-way CHAP should not include CHAP_C in response")
	}
}

func TestCHAPExchangeMutual(t *testing.T) {
	user := "testuser"
	secret := "testsecret"
	mutualSecret := "targetsecret"

	cs := newCHAPState(user, secret, true, mutualSecret)

	// Simulate target sending CHAP_A, CHAP_I, CHAP_C.
	challengeBytes := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	targetKeys := map[string]string{
		"CHAP_A": "5",
		"CHAP_I": "10",
		"CHAP_C": encodeCHAPBinary(challengeBytes),
	}

	resp, err := cs.processChallenge(targetKeys)
	if err != nil {
		t.Fatalf("processChallenge returned error: %v", err)
	}

	// Must include CHAP_N and CHAP_R (initiator auth response).
	if resp["CHAP_N"] != user {
		t.Errorf("CHAP_N = %q, want %q", resp["CHAP_N"], user)
	}
	if _, ok := resp["CHAP_R"]; !ok {
		t.Fatal("mutual CHAP response missing CHAP_R")
	}

	// Must include initiator's CHAP_I and CHAP_C for mutual auth.
	initiatorI, ok := resp["CHAP_I"]
	if !ok {
		t.Fatal("mutual CHAP response missing CHAP_I")
	}
	initiatorC, ok := resp["CHAP_C"]
	if !ok {
		t.Fatal("mutual CHAP response missing CHAP_C")
	}

	// Decode initiator challenge to compute expected target response.
	initiatorChallenge, err := decodeCHAPBinary(initiatorC)
	if err != nil {
		t.Fatalf("failed to decode initiator CHAP_C: %v", err)
	}

	// Parse initiator ID.
	var initiatorID byte
	for _, c := range initiatorI {
		initiatorID = initiatorID*10 + byte(c-'0')
	}

	// Simulate correct target response.
	targetResp := chapResponse(initiatorID, []byte(mutualSecret), initiatorChallenge)
	targetRespKeys := map[string]string{
		"CHAP_N": "targetname",
		"CHAP_R": encodeCHAPBinary(targetResp[:]),
	}

	// Verify should succeed with correct response.
	if err := cs.verifyMutualResponse(targetRespKeys); err != nil {
		t.Errorf("verifyMutualResponse with correct response returned error: %v", err)
	}

	// Verify should fail with wrong response.
	wrongKeys := map[string]string{
		"CHAP_N": "targetname",
		"CHAP_R": "0xdeadbeefdeadbeefdeadbeefdeadbeef",
	}
	if err := cs.verifyMutualResponse(wrongKeys); err == nil {
		t.Error("verifyMutualResponse with wrong response should return error")
	}
}

func TestCHAPExchangeUnsupportedAlgorithm(t *testing.T) {
	cs := newCHAPState("user", "secret", false, "")

	targetKeys := map[string]string{
		"CHAP_A": "6",
		"CHAP_I": "1",
		"CHAP_C": "0x0102030405060708",
	}

	_, err := cs.processChallenge(targetKeys)
	if err == nil {
		t.Error("processChallenge with unsupported algorithm should return error")
	}
}
