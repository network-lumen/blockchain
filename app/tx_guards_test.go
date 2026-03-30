package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"
)

type guardTestCodec struct{}

func (guardTestCodec) StringToBytes(text string) ([]byte, error) {
	if text == "" {
		return nil, errors.New("empty address")
	}
	return []byte(text), nil
}

func (guardTestCodec) BytesToString(bz []byte) (string, error) {
	if len(bz) == 0 {
		return "", errors.New("empty address")
	}
	return string(bz), nil
}

type guardTestTx struct {
	msgs []sdk.Msg
}

func (m guardTestTx) GetMsgs() []sdk.Msg                                 { return m.msgs }
func (guardTestTx) GetMsgsV2() ([]protov2.Message, error)                { return nil, nil }
func (guardTestTx) ValidateBasic() error                                 { return nil }
func (guardTestTx) GetMemo() string                                      { return "" }
func (guardTestTx) GetSignaturesV2() ([]signingtypes.SignatureV2, error) { return nil, nil }
func (guardTestTx) GetPubKeys() ([]cryptotypes.PubKey, error)            { return nil, nil }

type fakeTxGuardState struct {
	govVoteHeights      map[string]uint64
	withdrawAddrHeights map[string]uint64
}

func newFakeTxGuardState() *fakeTxGuardState {
	return &fakeTxGuardState{
		govVoteHeights:      map[string]uint64{},
		withdrawAddrHeights: map[string]uint64{},
	}
}

func govVoteKey(proposalID uint64, voter string) string {
	return fmt.Sprintf("%d|%s", proposalID, voter)
}

func (f *fakeTxGuardState) GetGovVoteLastHeight(_ context.Context, proposalID uint64, voter string) (uint64, bool, error) {
	height, ok := f.govVoteHeights[govVoteKey(proposalID, voter)]
	return height, ok, nil
}

func (f *fakeTxGuardState) SetGovVoteLastHeight(_ context.Context, proposalID uint64, voter string, height uint64) error {
	f.govVoteHeights[govVoteKey(proposalID, voter)] = height
	return nil
}

func (f *fakeTxGuardState) GetWithdrawAddrLastHeight(_ context.Context, delegator string) (uint64, bool, error) {
	height, ok := f.withdrawAddrHeights[delegator]
	return height, ok, nil
}

func (f *fakeTxGuardState) SetWithdrawAddrLastHeight(_ context.Context, delegator string, height uint64) error {
	f.withdrawAddrHeights[delegator] = height
	return nil
}

type fakeGovVoteReader struct {
	hasVote bool
	err     error
}

func (f fakeGovVoteReader) HasVote(_ context.Context, _ uint64, _ sdk.AccAddress) (bool, error) {
	return f.hasVote, f.err
}

type fakeDistributionGuardReader struct {
	currentWithdraw sdk.AccAddress
	currentErr      error
	previewRewards  sdk.Coins
	previewErr      error
}

func (f fakeDistributionGuardReader) GetDelegatorWithdrawAddr(_ context.Context, _ sdk.AccAddress) (sdk.AccAddress, error) {
	return f.currentWithdraw, f.currentErr
}

func (f fakeDistributionGuardReader) PreviewWithdrawDelegationRewards(_ sdk.Context, _ sdk.AccAddress, _ sdk.ValAddress) (sdk.Coins, error) {
	return f.previewRewards, f.previewErr
}

func TestTxGuardDecoratorRejectsGovRevoteDuringCooldown(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 105}, false, log.NewNopLogger())
	state := newFakeTxGuardState()
	state.govVoteHeights[govVoteKey(7, "voter")] = 100

	decorator := newTxGuardDecorator(
		guardTestCodec{},
		guardTestCodec{},
		fakeGovVoteReader{hasVote: true},
		fakeDistributionGuardReader{},
		state,
	)

	tx := guardTestTx{msgs: []sdk.Msg{&govv1.MsgVote{ProposalId: 7, Voter: "voter", Option: govv1.OptionYes}}}
	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gov revote cooldown")
	require.Contains(t, err.Error(), "wait 5 more blocks")
}

func TestTxGuardDecoratorAllowsGovVoteWithoutRecordedCooldown(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 105}, false, log.NewNopLogger())
	decorator := newTxGuardDecorator(
		guardTestCodec{},
		guardTestCodec{},
		fakeGovVoteReader{hasVote: true},
		fakeDistributionGuardReader{},
		newFakeTxGuardState(),
	)

	tx := guardTestTx{msgs: []sdk.Msg{&govv1.MsgVoteWeighted{
		ProposalId: 7,
		Voter:      "voter",
		Options: []*govv1.WeightedVoteOption{
			govv1.NewWeightedVoteOption(govv1.OptionYes, sdkmath.LegacyOneDec()),
		},
	}}}
	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.NoError(t, err)
}

func TestTxGuardDecoratorRejectsSetWithdrawAddressNoOp(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 50}, false, log.NewNopLogger())
	decorator := newTxGuardDecorator(
		guardTestCodec{},
		guardTestCodec{},
		fakeGovVoteReader{},
		fakeDistributionGuardReader{currentWithdraw: sdk.AccAddress([]byte("same"))},
		newFakeTxGuardState(),
	)

	tx := guardTestTx{msgs: []sdk.Msg{&distrtypes.MsgSetWithdrawAddress{
		DelegatorAddress: "delegator",
		WithdrawAddress:  "same",
	}}}
	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.Error(t, err)
	require.Contains(t, err.Error(), "new address matches current withdraw address")
}

func TestTxGuardDecoratorRejectsSetWithdrawAddressDuringCooldown(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 25}, false, log.NewNopLogger())
	state := newFakeTxGuardState()
	state.withdrawAddrHeights["delegator"] = 10

	decorator := newTxGuardDecorator(
		guardTestCodec{},
		guardTestCodec{},
		fakeGovVoteReader{},
		fakeDistributionGuardReader{currentWithdraw: sdk.AccAddress([]byte("old"))},
		state,
	)

	tx := guardTestTx{msgs: []sdk.Msg{&distrtypes.MsgSetWithdrawAddress{
		DelegatorAddress: "delegator",
		WithdrawAddress:  "new",
	}}}
	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.Error(t, err)
	require.Contains(t, err.Error(), "set withdraw address cooldown")
	require.Contains(t, err.Error(), "wait 5 more blocks")
}

func TestTxGuardDecoratorRejectsZeroRewardWithdrawal(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 25}, false, log.NewNopLogger())
	decorator := newTxGuardDecorator(
		guardTestCodec{},
		guardTestCodec{},
		fakeGovVoteReader{},
		fakeDistributionGuardReader{
			previewRewards: sdk.Coins{sdk.NewInt64Coin("ulmn", 0)},
		},
		newFakeTxGuardState(),
	)

	tx := guardTestTx{msgs: []sdk.Msg{&distrtypes.MsgWithdrawDelegatorReward{
		DelegatorAddress: "delegator",
		ValidatorAddress: "validator",
	}}}
	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no rewards available")
}

func TestTxGuardPostHandlerRecordsSuccessfulUpdates(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 42}, false, log.NewNopLogger())
	state := newFakeTxGuardState()
	handler := newTxGuardPostHandler(state)

	tx := guardTestTx{msgs: []sdk.Msg{
		&govv1.MsgVote{ProposalId: 9, Voter: "voter", Option: govv1.OptionYes},
		&distrtypes.MsgSetWithdrawAddress{DelegatorAddress: "delegator", WithdrawAddress: "new"},
	}}

	_, err := handler(ctx, tx, false, true)
	require.NoError(t, err)

	height, found, err := state.GetGovVoteLastHeight(ctx, 9, "voter")
	require.NoError(t, err)
	require.True(t, found)
	require.EqualValues(t, 42, height)

	height, found, err = state.GetWithdrawAddrLastHeight(ctx, "delegator")
	require.NoError(t, err)
	require.True(t, found)
	require.EqualValues(t, 42, height)
}

func TestTxGuardPostHandlerSkipsFailedTransactions(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 42}, false, log.NewNopLogger())
	state := newFakeTxGuardState()
	handler := newTxGuardPostHandler(state)

	tx := guardTestTx{msgs: []sdk.Msg{&govv1.MsgVote{ProposalId: 9, Voter: "voter", Option: govv1.OptionYes}}}

	_, err := handler(ctx, tx, false, false)
	require.NoError(t, err)

	_, found, err := state.GetGovVoteLastHeight(ctx, 9, "voter")
	require.NoError(t, err)
	require.False(t, found)
}
