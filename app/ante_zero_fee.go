package app

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// ZeroFeeDecorator rejects any transaction that includes a non-zero fee.
type ZeroFeeDecorator struct{}

func NewZeroFeeDecorator() ZeroFeeDecorator { return ZeroFeeDecorator{} }

func (d ZeroFeeDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	feeTx, ok := tx.(sdk.FeeTx)
	if ok {
		if fees := feeTx.GetFee(); !fees.IsZero() {
			return ctx, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "gasless tx must have zero fee")
		}
	}
	return next(ctx, tx, simulate)
}
