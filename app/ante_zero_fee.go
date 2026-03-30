package app

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const editValidatorFixedFeeUlmn int64 = 1_000_000

// SelectiveFeeDecorator keeps native Lumen application txs gasless, except for
// specific fee-bearing paths with explicit rules.
type SelectiveFeeDecorator struct{}

func NewSelectiveFeeDecorator() SelectiveFeeDecorator { return SelectiveFeeDecorator{} }

func (d SelectiveFeeDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		return next(ctx, tx, simulate)
	}

	policy, err := classifyTxFeePolicy(tx.GetMsgs())
	if err != nil {
		return ctx, err
	}

	fees := feeTx.GetFee()
	switch policy {
	case txFeePolicyGasless:
		if !fees.IsZero() {
			return ctx, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "gasless tx must have zero fee")
		}
		return next(ctx, tx, simulate)
	case txFeePolicyIBC:
		if len(fees) != 1 || !fees[0].IsPositive() {
			return ctx, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "ibc tx must pay a positive ulmn fee")
		}
		if fees[0].Denom != sdk.DefaultBondDenom {
			return ctx, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "ibc tx fee denom must be %s", sdk.DefaultBondDenom)
		}
	case txFeePolicyEditValidator:
		expectedFee := sdk.NewCoins(sdk.NewInt64Coin(sdk.DefaultBondDenom, editValidatorFixedFeeUlmn))
		if !fees.Equal(expectedFee) {
			return ctx, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "edit-validator tx must pay exactly %s", expectedFee)
		}
	}

	return next(ctx, tx, simulate)
}
