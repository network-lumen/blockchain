package keys

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// DecodeKey parses a hex, standard base64, or raw base64 string into bytes.
func DecodeKey(input string) ([]byte, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil, fmt.Errorf("empty key")
	}

	if len(s)%2 == 0 {
		if bz, err := hex.DecodeString(s); err == nil {
			return bz, nil
		}
	}
	if bz, err := base64.StdEncoding.DecodeString(s); err == nil {
		return bz, nil
	}
	if bz, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return bz, nil
	}
	return nil, fmt.Errorf("failed to decode key: expected hex or base64 encoding")
}
