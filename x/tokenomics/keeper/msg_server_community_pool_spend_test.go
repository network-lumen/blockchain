package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"lumen/x/tokenomics/keeper"
	"lumen/x/tokenomics/types"
)

func TestCommunityPoolSpend_Succeeds(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	authority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		f.keeper.GetAuthority(),
	)
	recipient := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		[]byte("recipient_addr_123456"),
	)

	_, err := server.CommunityPoolSpend(f.sdkCtx, &types.MsgCommunityPoolSpend{
		Authority: authority,
		Recipient: recipient,
		Amount:    sdk.NewCoins(sdk.NewInt64Coin(types.DefaultDenom, 1_000_000)),
	})
	require.NoError(t, err)
	require.Len(t, f.dist.communityPoolSpends, 1)
	require.Equal(t, recipient, f.dist.communityPoolSpends[0].to.String())
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin(types.DefaultDenom, 1_000_000)), f.dist.communityPoolSpends[0].amt)
}

func TestCommunityPoolSpend_InvalidAuthorityFails(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	recipient := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		[]byte("recipient_addr_123456"),
	)

	_, err := server.CommunityPoolSpend(f.sdkCtx, &types.MsgCommunityPoolSpend{
		Authority: recipient,
		Recipient: recipient,
		Amount:    sdk.NewCoins(sdk.NewInt64Coin(types.DefaultDenom, 1)),
	})
	require.ErrorContains(t, err, "invalid authority")
}

func TestCommunityPoolSpend_InvalidAmountFails(t *testing.T) {
	f := initFixture(t)
	server := keeper.NewMsgServerImpl(f.keeper)

	authority := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		f.keeper.GetAuthority(),
	)
	recipient := sdk.MustBech32ifyAddressBytes(
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		[]byte("recipient_addr_123456"),
	)

	testCases := []struct {
		name   string
		amount sdk.Coins
		errMsg string
	}{
		{name: "empty", amount: sdk.NewCoins(), errMsg: "amount must be set"},
		{name: "zero", amount: sdk.NewCoins(sdk.NewInt64Coin(types.DefaultDenom, 0)), errMsg: "amount must be set"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := server.CommunityPoolSpend(f.sdkCtx, &types.MsgCommunityPoolSpend{
				Authority: authority,
				Recipient: recipient,
				Amount:    tc.amount,
			})
			require.ErrorContains(t, err, tc.errMsg)
		})
	}
}
