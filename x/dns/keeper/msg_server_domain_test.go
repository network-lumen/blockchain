package keeper_test

import (
	"strconv"
	"testing"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/require"

	"lumen/x/dns/keeper"
	"lumen/x/dns/types"
)

func TestDomainMsgServerCreate(t *testing.T) {
	f := initFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)
	creator, err := f.addressCodec.BytesToString([]byte("signerAddr__________________"))
	require.NoError(t, err)
	records := []*types.Record{{Key: "txt", Value: "hello"}}

	for i := 0; i < 5; i++ {
		expected := &types.MsgCreateDomain{
			Creator: creator,
			Index:   strconv.Itoa(i),
			Name:    strconv.Itoa(i),
			Owner:   creator,
			Records: records,
		}
		_, err := srv.CreateDomain(f.ctx, expected)
		require.NoError(t, err)
		rst, err := f.keeper.Domain.Get(f.ctx, expected.Index)
		require.NoError(t, err)
		require.Equal(t, expected.Creator, rst.Creator)
	}
}

func TestDomainMsgServerUpdate(t *testing.T) {
	f := initFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	creator, err := f.addressCodec.BytesToString([]byte("signerAddr__________________"))
	require.NoError(t, err)

	unauthorizedAddr, err := f.addressCodec.BytesToString([]byte("unauthorizedAddr___________"))
	require.NoError(t, err)

	expected := &types.MsgCreateDomain{
		Creator: creator,
		Index:   strconv.Itoa(0),
		Name:    "0",
		Owner:   creator,
		Records: []*types.Record{{Key: "txt", Value: "hello"}},
	}
	_, err = srv.CreateDomain(f.ctx, expected)
	require.NoError(t, err)

	tests := []struct {
		desc    string
		request *types.MsgUpdateDomain
		err     error
	}{
		{
			desc: "invalid address",
			request: &types.MsgUpdateDomain{
				Creator: "invalid",
				Index:   strconv.Itoa(0),
				Owner:   creator,
			},
			err: sdkerrors.ErrInvalidAddress,
		},
		{
			desc: "unauthorized",
			request: &types.MsgUpdateDomain{
				Creator: unauthorizedAddr,
				Index:   strconv.Itoa(0),
				Owner:   creator,
			},
			err: sdkerrors.ErrUnauthorized,
		},
		{
			desc: "key not found",
			request: &types.MsgUpdateDomain{
				Creator: creator,
				Index:   strconv.Itoa(100000),
				Owner:   creator,
			},
			err: sdkerrors.ErrKeyNotFound,
		},
		{
			desc: "completed",
			request: &types.MsgUpdateDomain{
				Creator:  creator,
				Index:    strconv.Itoa(0),
				Name:     "0",
				Owner:    creator,
				Records:  []*types.Record{{Key: "txt", Value: "updated"}},
				ExpireAt: 0,
				PowNonce: 0,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			_, err = srv.UpdateDomain(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				rst, err := f.keeper.Domain.Get(f.ctx, expected.Index)
				require.NoError(t, err)
				require.Equal(t, expected.Creator, rst.Creator)
			}
		})
	}
}

func TestDomainMsgServerDelete(t *testing.T) {
	f := initFixture(t)
	srv := keeper.NewMsgServerImpl(f.keeper)

	creator, err := f.addressCodec.BytesToString([]byte("signerAddr__________________"))
	require.NoError(t, err)

	unauthorizedAddr, err := f.addressCodec.BytesToString([]byte("unauthorizedAddr___________"))
	require.NoError(t, err)

	_, err = srv.CreateDomain(f.ctx, &types.MsgCreateDomain{
		Creator: creator,
		Index:   strconv.Itoa(0),
		Name:    "0",
		Owner:   creator,
		Records: []*types.Record{{Key: "txt", Value: "hello"}},
	})
	require.NoError(t, err)

	tests := []struct {
		desc    string
		request *types.MsgDeleteDomain
		err     error
	}{
		{
			desc: "invalid address",
			request: &types.MsgDeleteDomain{Creator: "invalid",
				Index: strconv.Itoa(0),
			},
			err: sdkerrors.ErrInvalidAddress,
		},
		{
			desc: "unauthorized",
			request: &types.MsgDeleteDomain{Creator: unauthorizedAddr,
				Index: strconv.Itoa(0),
			},
			err: sdkerrors.ErrUnauthorized,
		},
		{
			desc: "key not found",
			request: &types.MsgDeleteDomain{Creator: creator,
				Index: strconv.Itoa(100000),
			},
			err: sdkerrors.ErrKeyNotFound,
		},
		{
			desc: "completed",
			request: &types.MsgDeleteDomain{Creator: creator,
				Index: strconv.Itoa(0),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			_, err = srv.DeleteDomain(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				found, err := f.keeper.Domain.Has(f.ctx, tc.request.Index)
				require.NoError(t, err)
				require.False(t, found)
			}
		})
	}
}
