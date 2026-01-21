package keeper_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"lumen/x/release/keeper"
	"lumen/x/release/types"
)

func TestPublishForcesPendingAndWipesEmergency(t *testing.T) {
	f := newReleaseFixture(t)

	publisherBz := sdk.AccAddress(bytes.Repeat([]byte{0x11}, 20))
	publisher, err := f.addrCodec.BytesToString(publisherBz)
	require.NoError(t, err)

	params := types.NewParams(
		[]string{publisher},
		[]string{"stable", "beta"},
		8, 8, 4, 512,
		0,   // publish_fee_ulmn
		0,   // max_pending_ttl
		nil, // dao_publishers (ignored)
		0,
		false,
	)
	require.NoError(t, f.keeper.SetParams(f.ctx, params))

	msg := &types.MsgPublishRelease{
		Creator: publisher,
		Release: types.Release{
			Version:        "1.2.3",
			Channel:        "stable",
			Notes:          "hi",
			Status:         types.Release_VALIDATED, // must be ignored
			EmergencyOk:    true,                    // must be wiped
			EmergencyUntil: 999_999,
			Artifacts: []*types.Artifact{
				{Platform: "linux-amd64", Kind: "daemon", Sha256Hex: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Urls: []string{"https://example.com/bin"}},
			},
		},
	}
	resp, err := f.msgSrv.PublishRelease(f.ctx, msg)
	require.NoError(t, err)
	require.Equal(t, uint64(1), resp.Id)

	r, err := f.keeper.Release.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.Equal(t, types.Release_PENDING, r.Status)
	require.False(t, r.EmergencyOk)
	require.Zero(t, r.EmergencyUntil)
	require.Equal(t, publisher, r.Publisher)
	require.Equal(t, f.ctx.BlockTime().Unix(), r.CreatedAt)
}

func TestEscrowRefundOnValidate(t *testing.T) {
	f := newReleaseFixture(t)

	publisherBz := sdk.AccAddress(bytes.Repeat([]byte{0x22}, 20))
	publisher, err := f.addrCodec.BytesToString(publisherBz)
	require.NoError(t, err)

	const fee uint64 = 1000
	params := types.NewParams(
		[]string{publisher},
		[]string{"stable", "beta"},
		8, 8, 4, 512,
		fee,
		0,
		nil,
		0,
		false,
	)
	require.NoError(t, f.keeper.SetParams(f.ctx, params))

	f.bank.setBalance(publisher, sdk.NewCoins(sdk.NewInt64Coin("ulmn", 10_000)))

	before := f.bank.balances[publisher].AmountOf("ulmn")
	_, err = f.msgSrv.PublishRelease(f.ctx, &types.MsgPublishRelease{
		Creator: publisher,
		Release: types.Release{
			Version:   "1.0.0",
			Channel:   "stable",
			Notes:     "v1",
			Artifacts: []*types.Artifact{{Platform: "linux", Kind: "daemon", Sha256Hex: strings.Repeat("a", 64), Urls: []string{"https://example.com/bin"}}},
		},
	})
	require.NoError(t, err)

	afterPublish := f.bank.balances[publisher].AmountOf("ulmn")
	require.True(t, afterPublish.LT(before))

	_, err = f.msgSrv.ValidateRelease(f.ctx, &types.MsgValidateRelease{Authority: f.authority, Id: 1})
	require.NoError(t, err)

	afterValidate := f.bank.balances[publisher].AmountOf("ulmn")
	require.True(t, afterValidate.Equal(before))

	// Escrow record removed.
	_, err = f.keeper.EscrowAmount.Get(f.ctx, 1)
	require.Error(t, err)

	r, err := f.keeper.Release.Get(f.ctx, 1)
	require.NoError(t, err)
	require.Equal(t, types.Release_VALIDATED, r.Status)
}

func TestEscrowForfeitOnRejectAndYank(t *testing.T) {
	f := newReleaseFixture(t)

	publisherBz := sdk.AccAddress(bytes.Repeat([]byte{0x33}, 20))
	publisher, err := f.addrCodec.BytesToString(publisherBz)
	require.NoError(t, err)

	const fee uint64 = 1000
	params := types.NewParams(
		[]string{publisher},
		[]string{"stable", "beta"},
		8, 8, 4, 512,
		fee,
		0,
		nil,
		0,
		false,
	)
	require.NoError(t, f.keeper.SetParams(f.ctx, params))
	f.bank.setBalance(publisher, sdk.NewCoins(sdk.NewInt64Coin("ulmn", 10_000)))

	_, err = f.msgSrv.PublishRelease(f.ctx, &types.MsgPublishRelease{
		Creator: publisher,
		Release: types.Release{
			Version:   "1.0.0",
			Channel:   "stable",
			Artifacts: []*types.Artifact{{Platform: "linux", Kind: "daemon", Sha256Hex: strings.Repeat("a", 64), Urls: []string{"https://example.com/bin"}}},
		},
	})
	require.NoError(t, err)

	cpBefore := f.distr.communityPool.AmountOf("ulmn")
	_, err = f.msgSrv.RejectRelease(f.ctx, &types.MsgRejectRelease{Authority: f.authority, Id: 1})
	require.NoError(t, err)
	require.True(t, f.distr.communityPool.AmountOf("ulmn").GTE(cpBefore.AddRaw(int64(fee))))

	// New pending publish, then yank.
	_, err = f.msgSrv.PublishRelease(f.ctx, &types.MsgPublishRelease{
		Creator: publisher,
		Release: types.Release{
			Version:   "1.0.1",
			Channel:   "stable",
			Artifacts: []*types.Artifact{{Platform: "linux", Kind: "daemon", Sha256Hex: strings.Repeat("b", 64), Urls: []string{"https://example.com/bin"}}},
		},
	})
	require.NoError(t, err)
	cpMid := f.distr.communityPool.AmountOf("ulmn")
	_, err = f.msgSrv.YankRelease(f.ctx, &types.MsgYankRelease{Creator: publisher, Id: 2})
	require.NoError(t, err)
	require.True(t, f.distr.communityPool.AmountOf("ulmn").GTE(cpMid.AddRaw(int64(fee))))

	r, err := f.keeper.Release.Get(f.ctx, 2)
	require.NoError(t, err)
	require.True(t, r.Yanked)
}

func TestExpiryEndBlockExpiresPending(t *testing.T) {
	f := newReleaseFixture(t)

	publisherBz := sdk.AccAddress(bytes.Repeat([]byte{0x44}, 20))
	publisher, err := f.addrCodec.BytesToString(publisherBz)
	require.NoError(t, err)

	const fee uint64 = 1000
	const ttl uint64 = 10
	params := types.NewParams(
		[]string{publisher},
		[]string{"stable", "beta"},
		8, 8, 4, 512,
		fee,
		ttl,
		nil,
		0,
		false,
	)
	require.NoError(t, f.keeper.SetParams(f.ctx, params))
	f.bank.setBalance(publisher, sdk.NewCoins(sdk.NewInt64Coin("ulmn", 10_000)))

	_, err = f.msgSrv.PublishRelease(f.ctx, &types.MsgPublishRelease{
		Creator: publisher,
		Release: types.Release{
			Version:   "1.0.0",
			Channel:   "stable",
			Artifacts: []*types.Artifact{{Platform: "linux", Kind: "daemon", Sha256Hex: strings.Repeat("a", 64), Urls: []string{"https://example.com/bin"}}},
		},
	})
	require.NoError(t, err)

	cpBefore := f.distr.communityPool.AmountOf("ulmn")
	f.ctx = f.ctx.WithBlockTime(f.ctx.BlockTime().Add(time.Duration(ttl+1) * time.Second))
	require.NoError(t, f.keeper.EndBlocker(f.ctx))

	r, err := f.keeper.Release.Get(f.ctx, 1)
	require.NoError(t, err)
	require.Equal(t, types.Release_EXPIRED, r.Status)
	require.True(t, f.distr.communityPool.AmountOf("ulmn").GTE(cpBefore.AddRaw(int64(fee))))
}

func TestMirrorPendingOnly(t *testing.T) {
	f := newReleaseFixture(t)

	publisherBz := sdk.AccAddress(bytes.Repeat([]byte{0x55}, 20))
	publisher, err := f.addrCodec.BytesToString(publisherBz)
	require.NoError(t, err)

	params := types.NewParams(
		[]string{publisher},
		[]string{"stable", "beta"},
		8, 8, 4, 512,
		0,
		0,
		nil,
		0,
		false,
	)
	require.NoError(t, f.keeper.SetParams(f.ctx, params))

	_, err = f.msgSrv.PublishRelease(f.ctx, &types.MsgPublishRelease{
		Creator: publisher,
		Release: types.Release{
			Version:   "1.0.0",
			Channel:   "stable",
			Artifacts: []*types.Artifact{{Platform: "linux", Kind: "daemon", Sha256Hex: strings.Repeat("a", 64), Urls: []string{"https://example.com/bin"}}},
		},
	})
	require.NoError(t, err)

	_, err = f.msgSrv.ValidateRelease(f.ctx, &types.MsgValidateRelease{Authority: f.authority, Id: 1})
	require.NoError(t, err)

	_, err = f.msgSrv.MirrorRelease(f.ctx, &types.MsgMirrorRelease{Creator: publisher, Id: 1, ArtifactIndex: 0, NewUrls: []string{"https://m1"}})
	require.Error(t, err)
}

func TestLatestNeverReturnsPending(t *testing.T) {
	f := newReleaseFixture(t)

	publisherBz := sdk.AccAddress(bytes.Repeat([]byte{0x66}, 20))
	publisher, err := f.addrCodec.BytesToString(publisherBz)
	require.NoError(t, err)

	params := types.NewParams(
		[]string{publisher},
		[]string{"stable", "beta"},
		8, 8, 4, 512,
		0,
		0,
		nil,
		0,
		false,
	)
	require.NoError(t, f.keeper.SetParams(f.ctx, params))

	_, err = f.msgSrv.PublishRelease(f.ctx, &types.MsgPublishRelease{
		Creator: publisher,
		Release: types.Release{
			Version:   "1.0.0",
			Channel:   "stable",
			Artifacts: []*types.Artifact{{Platform: "linux", Kind: "daemon", Sha256Hex: strings.Repeat("a", 64), Urls: []string{"https://example.com/bin"}}},
		},
	})
	require.NoError(t, err)

	// Simulate a polluted index pointing to a pending release.
	require.NoError(t, f.keeper.ByTriple.Set(f.ctx, "stable|linux|daemon", 1))

	qs := keeper.NewQueryServerImpl(f.keeper)
	_, err = qs.Latest(f.ctx, &types.QueryLatestRequest{Channel: "stable", Platform: "linux", Kind: "daemon"})
	require.Error(t, err)

	_, err = f.msgSrv.ValidateRelease(f.ctx, &types.MsgValidateRelease{Authority: f.authority, Id: 1})
	require.NoError(t, err)

	resp, err := qs.Latest(f.ctx, &types.QueryLatestRequest{Channel: "stable", Platform: "linux", Kind: "daemon"})
	require.NoError(t, err)
	require.Equal(t, types.Release_VALIDATED, resp.Release.Status)
	require.Equal(t, uint64(1), resp.Release.Id)
}
