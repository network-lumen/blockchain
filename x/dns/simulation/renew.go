package simulation

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"lumen/x/dns/keeper"
	"lumen/x/dns/types"
)

func SimulateMsgRenew(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		dom, ownerAcc, found, err := pickRandomDomain(ctx, k, ak, accs)
		if err != nil {
			return simtypes.OperationMsg{}, nil, err
		}
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, "renew", "no domain owners found"), nil, nil
		}
		domainPart, ext, ok := splitFQDN(dom.Name)
		if !ok {
			return simtypes.NoOpMsg(types.ModuleName, "renew", "invalid fqdn"), nil, nil
		}

		msg := &types.MsgRenew{
			Creator:      ownerAcc.Address.String(),
			Domain:       domainPart,
			Ext:          ext,
			DurationDays: 30,
		}

		txCtx := simulation.OperationInput{
			R:               r,
			App:             app,
			TxGen:           txGen,
			Cdc:             nil,
			Msg:             msg,
			Context:         ctx,
			SimAccount:      ownerAcc,
			ModuleName:      types.ModuleName,
			CoinsSpentInMsg: sdk.NewCoins(),
			AccountKeeper:   ak,
			Bankkeeper:      bk,
		}
		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}
