package app

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

type EnforceZeroMinGasDecorator struct{}

func NewEnforceZeroMinGasDecorator() EnforceZeroMinGasDecorator { return EnforceZeroMinGasDecorator{} }

func (d EnforceZeroMinGasDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	if !ctx.MinGasPrices().IsZero() {
		return ctx, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "gasless chain: minimum-gas-prices must be zero at runtime")
	}
	return next(ctx, tx, simulate)
}
