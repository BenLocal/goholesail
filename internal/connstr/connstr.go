// Package connstr encodes and decodes goholesail connection strings.
//
// Format: ghs://<mode><version>_<base64url(json)>
//
//	mode 's' = private (Secret required), 'p' = public.
//
// The mode/version prefix is a fast, human-visible hint; the authoritative
// values live in the JSON payload and are cross-checked on decode.
package connstr

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const scheme = "ghs://"

// ConnString carries everything a client needs to reach a host. All fields are
// comparable so values can be compared directly in tests.
type ConnString struct {
	Version int    `json:"v"`
	Private bool   `json:"priv"`
	HostID  string `json:"host"`
	Hub     string `json:"hub"`
	Secret  string `json:"secret,omitempty"`
}

func mode(private bool) string {
	if private {
		return "s"
	}
	return "p"
}

// Encode serializes a ConnString to its ghs:// textual form.
func Encode(cs ConnString) (string, error) {
	b, err := json.Marshal(cs)
	if err != nil {
		return "", fmt.Errorf("connstr: marshal: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	return fmt.Sprintf("%s%s%d_%s", scheme, mode(cs.Private), cs.Version, payload), nil
}

// Decode parses a ghs:// string, verifying the prefix agrees with the payload.
func Decode(s string) (ConnString, error) {
	var zero ConnString
	if !strings.HasPrefix(s, scheme) {
		return zero, fmt.Errorf("connstr: missing %q scheme", scheme)
	}
	rest := strings.TrimPrefix(s, scheme)
	usc := strings.IndexByte(rest, '_')
	if usc < 2 {
		return zero, fmt.Errorf("connstr: malformed prefix in %q", s)
	}
	prefix, payload := rest[:usc], rest[usc+1:]
	prefixMode := prefix[:1]
	prefixVer, err := strconv.Atoi(prefix[1:])
	if err != nil {
		return zero, fmt.Errorf("connstr: bad version in prefix: %w", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return zero, fmt.Errorf("connstr: decode payload: %w", err)
	}
	var cs ConnString
	if err := json.Unmarshal(raw, &cs); err != nil {
		return zero, fmt.Errorf("connstr: unmarshal payload: %w", err)
	}
	if prefixMode != mode(cs.Private) {
		return zero, fmt.Errorf("connstr: prefix mode %q disagrees with payload", prefixMode)
	}
	if prefixVer != cs.Version {
		return zero, fmt.Errorf("connstr: prefix version %d disagrees with payload %d", prefixVer, cs.Version)
	}
	return cs, nil
}
