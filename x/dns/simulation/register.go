package simulation

import (
	"fmt"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"lumen/x/dns/keeper"
	"lumen/x/dns/types"
)

func SimulateMsgRegister(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		var msg *types.MsgRegister

		for attempt := 0; attempt < 5; attempt++ {
			rawDomain := fmt.Sprintf("sim-%d", r.Intn(10_000_000))
			ext := "sim"
			fqdn := normalizeFQDN(rawDomain, ext)
			if _, err := k.Domain.Get(ctx, fqdn); err == nil {
				continue
			}
			msg = &types.MsgRegister{
				Creator:      simAccount.Address.String(),
				Domain:       rawDomain,
				Ext:          ext,
				Records:      randomTXTRecords(r),
				DurationDays: 30,
				Owner:        simAccount.Address.String(),
			}
			break
		}
		if msg == nil {
			return simtypes.NoOpMsg(types.ModuleName, "register", "no available domain"), nil, nil
		}

		txCtx := simulation.OperationInput{
			R:               r,
			App:             app,
			TxGen:           txGen,
			Cdc:             nil,
			Msg:             msg,
			Context:         ctx,
			SimAccount:      simAccount,
			ModuleName:      types.ModuleName,
			CoinsSpentInMsg: sdk.NewCoins(),
			AccountKeeper:   ak,
			Bankkeeper:      bk,
		}
		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}
