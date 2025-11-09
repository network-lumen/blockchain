package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"lumen/x/pqc/types"
)

func TestDefaultParamsValidate(t *testing.T) {
	params := types.DefaultParams()
	require.NoError(t, params.Validate())
}

func TestParamsInvalidScheme(t *testing.T) {
	params := types.Params{
		Policy:    types.PqcPolicy_PQC_POLICY_REQUIRED,
		MinScheme: "unknown",
	}
	err := params.Validate()
	require.Error(t, err)
}

func TestParamsRejectNonRequiredPolicy(t *testing.T) {
	params := types.Params{
		Policy:             types.PqcPolicy_PQC_POLICY_OPTIONAL,
		MinScheme:          types.DefaultMinScheme,
		AllowAccountRotate: false,
	}
	err := params.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "REQUIRED")
}
