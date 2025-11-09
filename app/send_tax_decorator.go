package app

import (
	tokenomicskeeper "lumen/x/tokenomics/keeper"
	tokenomicstypes "lumen/x/tokenomics/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
)

type SendTaxDecorator struct {
	authKeeper       authkeeper.AccountKeeper
	tokenomicsKeeper tokenomicskeeper.Keeper
}

func NewSendTaxDecorator(ak authkeeper.AccountKeeper, tk tokenomicskeeper.Keeper) SendTaxDecorator {
	return SendTaxDecorator{
		authKeeper:       ak,
		tokenomicsKeeper: tk,
	}
}

func (d SendTaxDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	params := d.tokenomicsKeeper.GetParams(ctx)
	rate := tokenomicstypes.GetTxTaxRateDec(params)
	if rate.IsZero() {
		return next(ctx, tx, simulate)
	}

	if _, _, err := computeSendTaxes(tx, rate, d.authKeeper.AddressCodec()); err != nil {
		return ctx, err
	}

	return next(ctx, tx, simulate)
}
