package types

import (
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestMsgPublishReleaseNotesLimit(t *testing.T) {
	msg := MsgPublishRelease{
		Creator: sdk.AccAddress(make([]byte, 20)).String(),
		Release: Release{
			Version: "1.2.3",
			Channel: "beta",
			Notes:   strings.Repeat("n", ReleaseNotesMaxLen),
			Artifacts: []*Artifact{
				{
					Platform:   "linux-amd64",
					Kind:       "daemon",
					Sha256Hex:  strings.Repeat("a", 64),
					Urls:       []string{"https://example.com/bin"},
					Signatures: []*Signature{},
				},
			},
		},
	}
	require.NoError(t, msg.ValidateBasic())

	msg.Release.Notes = strings.Repeat("n", ReleaseNotesMaxLen+1)
	err := msg.ValidateBasic()
	require.Error(t, err)
	require.Contains(t, err.Error(), "notes too long")
}

func TestValidateArtifactURL(t *testing.T) {
	valid := []string{
		"https://example.com/bin",
		"http://mirror.local/path",
		"ipfs://QmYwAPJzv5CZsnAzt8auVZRnG8Jw7Y8Yj9n1kY7zyBbs6u",
		"lumen://ipfs/QmYwAPJzv5CZsnAzt8auVZRnG8Jw7Y8Yj9n1kY7zyBbs6u",
		"lumen://ipns/mainnet.release",
		"lumen://client.lumen",
	}
	for _, u := range valid {
		require.NoError(t, ValidateArtifactURL(u), "expected url to be allowed: %s", u)
	}

	invalid := map[string]string{
		"":                        "url required",
		strings.Repeat("a", 2000): "url too long",
		"ftp://example.com/file":  "scheme",
		"https://":                "host required",
		"ipfs://":                 "cid required",
		"lumen://ipns/UPPER":      "ipns name",
		"lumen://bad/domain":      "fqdn",
		"lumen://nodot":           "domain",
		"lumen://domain.$$$":      "extension",
		"lumen://ipfs/not valid":  "ipfs",
		"lumen://ipns/has space":  "ipns",
		"http://":                 "host required",
	}
	for u, msg := range invalid {
		err := ValidateArtifactURL(u)
		require.Error(t, err, "expected error for %s", u)
		require.Contains(t, err.Error(), msg, "unexpected error for %s", u)
	}
}
