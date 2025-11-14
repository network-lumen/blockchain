package types_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"lumen/x/pqc/types"
)

func TestDefaultParamsValidate(t *testing.T) {
	params := types.DefaultParams()
	require.NoError(t, params.Validate())
}

func TestParamsInvalidScheme(t *testing.T) {
	params := types.Params{
		Policy:            types.PqcPolicy_PQC_POLICY_REQUIRED,
		MinScheme:         "unknown",
		MinBalanceForLink: types.DefaultMinBalanceForLink,
	}
	err := params.Validate()
	require.Error(t, err)
}

func TestParamsRejectNonRequiredPolicy(t *testing.T) {
	params := types.Params{
		Policy:            types.PqcPolicy_PQC_POLICY_OPTIONAL,
		MinScheme:         types.DefaultMinScheme,
		MinBalanceForLink: types.DefaultMinBalanceForLink,
	}
	err := params.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "REQUIRED")
}

func TestParamsInvalidMinBalance(t *testing.T) {
	params := types.DefaultParams()
	params.MinBalanceForLink = sdk.Coin{}
	require.Error(t, params.Validate())
}

func TestParamsPowDifficultyBounds(t *testing.T) {
	params := types.DefaultParams()
	params.PowDifficultyBits = 300
	require.Error(t, params.Validate())
}
