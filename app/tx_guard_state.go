package app

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

const (
	govRevoteCooldownBlocks       int64 = 10
	setWithdrawAddrCooldownBlocks int64 = 20
)

type txGuardState interface {
	GetGovVoteLastHeight(ctx context.Context, proposalID uint64, voter string) (uint64, bool, error)
	SetGovVoteLastHeight(ctx context.Context, proposalID uint64, voter string, height uint64) error
	GetWithdrawAddrLastHeight(ctx context.Context, delegator string) (uint64, bool, error)
	SetWithdrawAddrLastHeight(ctx context.Context, delegator string, height uint64) error
}

type govVoteReader interface {
	HasVote(ctx context.Context, proposalID uint64, voter sdk.AccAddress) (bool, error)
}

type distributionGuardReader interface {
	GetDelegatorWithdrawAddr(ctx context.Context, delAddr sdk.AccAddress) (sdk.AccAddress, error)
	PreviewWithdrawDelegationRewards(ctx sdk.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) (sdk.Coins, error)
}

type govVoteReaderAdapter struct {
	gk *govkeeper.Keeper
}

func (g govVoteReaderAdapter) HasVote(ctx context.Context, proposalID uint64, voter sdk.AccAddress) (bool, error) {
	return g.gk.Votes.Has(ctx, collections.Join(proposalID, voter))
}

type distributionGuardReaderAdapter struct {
	dk distrkeeper.Keeper
}

func (d distributionGuardReaderAdapter) GetDelegatorWithdrawAddr(ctx context.Context, delAddr sdk.AccAddress) (sdk.AccAddress, error) {
	return d.dk.GetDelegatorWithdrawAddr(ctx, delAddr)
}

func (d distributionGuardReaderAdapter) PreviewWithdrawDelegationRewards(ctx sdk.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) (sdk.Coins, error) {
	cacheCtx, _ := ctx.CacheContext()
	return d.dk.WithdrawDelegationRewards(cacheCtx, delAddr, valAddr)
}

func newTxGuardDecorator(
	accountCodec address.Codec,
	validatorCodec address.Codec,
	gov govVoteReader,
	distr distributionGuardReader,
	state txGuardState,
) TxGuardDecorator {
	return TxGuardDecorator{
		accountCodec:   accountCodec,
		validatorCodec: validatorCodec,
		gov:            gov,
		distr:          distr,
		state:          state,
	}
}

func newTxGuardPostHandler(state txGuardState) sdk.PostHandler {
	return func(ctx sdk.Context, tx sdk.Tx, simulate bool, success bool) (sdk.Context, error) {
		if ctx.IsCheckTx() || simulate || !success {
			return ctx, nil
		}

		height := blockHeightUint64(ctx)
		if height == 0 {
			return ctx, nil
		}

		for _, msg := range tx.GetMsgs() {
			switch m := msg.(type) {
			case *govv1.MsgVote:
				if err := state.SetGovVoteLastHeight(ctx, m.GetProposalId(), m.GetVoter(), height); err != nil {
					return ctx, err
				}
			case *govv1.MsgVoteWeighted:
				if err := state.SetGovVoteLastHeight(ctx, m.GetProposalId(), m.GetVoter(), height); err != nil {
					return ctx, err
				}
			case *distrtypes.MsgSetWithdrawAddress:
				if err := state.SetWithdrawAddrLastHeight(ctx, m.DelegatorAddress, height); err != nil {
					return ctx, err
				}
			}
		}

		return ctx, nil
	}
}

func blockHeightUint64(ctx sdk.Context) uint64 {
	if ctx.BlockHeight() <= 0 {
		return 0
	}
	return uint64(ctx.BlockHeight())
}

func cooldownRemaining(current, lastSeen uint64, cooldown int64) uint64 {
	if cooldown <= 0 || lastSeen == 0 {
		return 0
	}

	cooldownU := uint64(cooldown)
	if current >= lastSeen+cooldownU {
		return 0
	}
	return (lastSeen + cooldownU) - current
}
