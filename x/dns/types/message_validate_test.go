package types

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDomainAndExtLimits(t *testing.T) {
	err := validateDomainAndExt(strings.Repeat("a", DNSLabelMaxLen), "lm")
	require.NoError(t, err)

	err = validateDomainAndExt(strings.Repeat("a", DNSLabelMaxLen+1), "lm")
	require.Error(t, err)
}

func TestValidateRecordsPayload(t *testing.T) {
	recs := make([]*Record, 0, DNSRecordsMax)
	for i := 0; i < 4; i++ {
		recs = append(recs, &Record{Key: "txt", Value: strings.Repeat("a", 100)})
	}
	require.NoError(t, ValidateRecords(recs))

	big := []*Record{{Key: "txt", Value: strings.Repeat("a", DNSRecordsPayloadMaxBytes+1)}}
	require.Error(t, ValidateRecords(big))
}
