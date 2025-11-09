package types

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateEndpoint(t *testing.T) {
	valids := []string{
		"abc.xyz",
		"my-domain123.app",
		"valid-longlabel-12345.dev",
		"a23.bz",
		"foo123.bar",
	}

	invalids := []string{
		"-abc.xyz",
		"abc-.xyz",
		"ab.xyz",
		"abc.x",
		"abc..xyz",
		"abc.toolongextensionbeyondlimit",
		"abc_xyz.dev",
		"ab--cd.xyz",
		"",
	}

	for _, s := range valids {
		require.NoError(t, ValidateEndpoint(s), s)
	}
	for _, s := range invalids {
		require.Error(t, ValidateEndpoint(s), s)
	}

	t.Run("label length 63 OK", func(t *testing.T) {
		lbl := strings.Repeat("a", 63)
		require.NoError(t, ValidateEndpoint(lbl+".com"))
	})

	t.Run("label length 64 FAIL", func(t *testing.T) {
		lbl := strings.Repeat("a", 64)
		require.Error(t, ValidateEndpoint(lbl+".com"))
	})

	t.Run("total length >253 FAIL", func(t *testing.T) {
		lbl := strings.Repeat("a", 240)
		require.Error(t, ValidateEndpoint(lbl+".com"))
	})

	t.Run("double hyphen reserved", func(t *testing.T) {
		require.Error(t, ValidateEndpoint("ab--cd.xyz"))
	})
}
