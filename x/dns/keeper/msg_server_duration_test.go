package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"lumen/x/dns/keeper"
	"lumen/x/dns/types"
)

func TestRegisterDurationExceeded(t *testing.T) {
	f := initFixture(t)

	addr := sdk.AccAddress([]byte("creator________________"))
	creator, err := f.addressCodec.BytesToString(addr)
	require.NoError(t, err)

	msg := &types.MsgRegister{
		Creator:      creator,
		Domain:       "example",
		Ext:          "lumen",
		DurationDays: types.MaxRegistrationDurationDays + 1,
	}

	_, err = keeper.NewMsgServerImpl(f.keeper).Register(f.ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duration_days")
}

func TestRenewDurationExceeded(t *testing.T) {
	f := initFixture(t)

	addr := sdk.AccAddress([]byte("creator________________"))
	creator, err := f.addressCodec.BytesToString(addr)
	require.NoError(t, err)

	name := "example.lumen"
	err = f.keeper.Domain.Set(f.ctx, name, types.Domain{
		Index: name,
		Name:  name,
		Owner: creator,
	})
	require.NoError(t, err)

	msg := &types.MsgRenew{
		Creator:      creator,
		Domain:       "example",
		Ext:          "lumen",
		DurationDays: types.MaxRegistrationDurationDays + 1,
	}

	_, err = keeper.NewMsgServerImpl(f.keeper).Renew(f.ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duration_days")
}
