package app

import (
	"bytes"

	"cosmossdk.io/core/address"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

type TxGuardDecorator struct {
	accountCodec   address.Codec
	validatorCodec address.Codec
	gov            govVoteReader
	distr          distributionGuardReader
	state          txGuardState
}

func (d TxGuardDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	for _, msg := range tx.GetMsgs() {
		switch m := msg.(type) {
		case *govv1.MsgVote:
			if err := d.checkGovVoteCooldown(ctx, m.ProposalId, m.Voter); err != nil {
				return ctx, err
			}
		case *govv1.MsgVoteWeighted:
			if err := d.checkGovVoteCooldown(ctx, m.ProposalId, m.Voter); err != nil {
				return ctx, err
			}
		case *distrtypes.MsgSetWithdrawAddress:
			if err := d.checkSetWithdrawAddress(ctx, m.DelegatorAddress, m.WithdrawAddress); err != nil {
				return ctx, err
			}
		case *distrtypes.MsgWithdrawDelegatorReward:
			if err := d.checkWithdrawDelegatorReward(ctx, m.DelegatorAddress, m.ValidatorAddress); err != nil {
				return ctx, err
			}
		}
	}

	return next(ctx, tx, simulate)
}

func (d TxGuardDecorator) checkGovVoteCooldown(ctx sdk.Context, proposalID uint64, voter string) error {
	voterBz, err := d.accountCodec.StringToBytes(voter)
	if err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid voter address: %s", err)
	}

	hasVote, err := d.gov.HasVote(ctx, proposalID, voterBz)
	if err != nil || !hasVote {
		return err
	}

	lastHeight, found, err := d.state.GetGovVoteLastHeight(ctx, proposalID, voter)
	if err != nil || !found {
		return err
	}

	if remaining := cooldownRemaining(blockHeightUint64(ctx), lastHeight, govRevoteCooldownBlocks); remaining > 0 {
		return errorsmod.Wrapf(
			sdkerrors.ErrInvalidRequest,
			"gov revote cooldown: proposal %d voter %s must wait %d more blocks",
			proposalID,
			voter,
			remaining,
		)
	}

	return nil
}

func (d TxGuardDecorator) checkSetWithdrawAddress(ctx sdk.Context, delegator, withdraw string) error {
	delegatorBz, err := d.accountCodec.StringToBytes(delegator)
	if err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid delegator address: %s", err)
	}
	withdrawBz, err := d.accountCodec.StringToBytes(withdraw)
	if err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid withdraw address: %s", err)
	}

	currentWithdraw, err := d.distr.GetDelegatorWithdrawAddr(ctx, delegatorBz)
	if err != nil {
		return err
	}
	if bytes.Equal(currentWithdraw, withdrawBz) {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "set withdraw address: new address matches current withdraw address")
	}

	lastHeight, found, err := d.state.GetWithdrawAddrLastHeight(ctx, delegator)
	if err != nil || !found {
		return err
	}

	if remaining := cooldownRemaining(blockHeightUint64(ctx), lastHeight, setWithdrawAddrCooldownBlocks); remaining > 0 {
		return errorsmod.Wrapf(
			sdkerrors.ErrInvalidRequest,
			"set withdraw address cooldown: delegator %s must wait %d more blocks",
			delegator,
			remaining,
		)
	}

	return nil
}

func (d TxGuardDecorator) checkWithdrawDelegatorReward(ctx sdk.Context, delegator, validator string) error {
	delegatorBz, err := d.accountCodec.StringToBytes(delegator)
	if err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid delegator address: %s", err)
	}
	validatorBz, err := d.validatorCodec.StringToBytes(validator)
	if err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid validator address: %s", err)
	}

	rewards, err := d.distr.PreviewWithdrawDelegationRewards(ctx, delegatorBz, sdk.ValAddress(validatorBz))
	if err != nil {
		return err
	}
	if rewards.IsZero() {
		return errorsmod.Wrapf(
			sdkerrors.ErrInvalidRequest,
			"withdraw delegator reward: no rewards available for delegator %s on validator %s",
			delegator,
			validator,
		)
	}

	return nil
}
