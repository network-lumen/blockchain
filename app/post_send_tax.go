package app

import (
	"fmt"
	tokenomicskeeper "lumen/x/tokenomics/keeper"
	tokenomicstypes "lumen/x/tokenomics/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
)

func NewSendTaxPostHandler(
	ak authkeeper.AccountKeeper,
	bk bankkeeper.Keeper,
	tk tokenomicskeeper.Keeper,
) sdk.PostHandler {
	return func(ctx sdk.Context, tx sdk.Tx, simulate bool, success bool) (sdk.Context, error) {
		if ctx.IsCheckTx() || simulate || !success {
			return ctx, nil
		}

		params := tk.GetParams(ctx)
		rate := tokenomicstypes.GetTxTaxRateDec(params)
		if !rate.IsPositive() {
			return ctx, nil
		}

		perPayer, total, err := computeSendTaxes(tx, rate, ak.AddressCodec())
		if err != nil || len(perPayer) == 0 {
			return ctx, err
		}

		for _, record := range perPayer {
			if record == nil || len(record.coins) == 0 {
				continue
			}
			if err := bk.SendCoinsFromAccountToModule(ctx, record.addr, authtypes.FeeCollectorName, record.coins); err != nil {
				return ctx, err
			}
		}

		ctx.EventManager().EmitEvent(sdk.NewEvent(
			"send_tax",
			sdk.NewAttribute("total", total.String()),
			sdk.NewAttribute("rate", rate.String()),
			sdk.NewAttribute("payers", fmt.Sprintf("%d", len(perPayer))),
		))
		return ctx, nil
	}
}
