package simulation

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	sdkmath "cosmossdk.io/math"

	"lumen/x/dns/keeper"
	"lumen/x/dns/types"
)

func SimulateMsgBid(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)

		var auctions []types.Auction
		if err := k.Auction.Walk(ctx, nil, func(_ string, auc types.Auction) (bool, error) {
			auctions = append(auctions, auc)
			return false, nil
		}); err != nil {
			return simtypes.OperationMsg{}, nil, err
		}
		if len(auctions) == 0 {
			return simtypes.NoOpMsg(types.ModuleName, "bid", "no auctions"), nil, nil
		}
		auc := auctions[r.Intn(len(auctions))]
		domainPart, ext, ok := splitFQDN(auc.Name)
		if !ok {
			return simtypes.NoOpMsg(types.ModuleName, "bid", "invalid auction name"), nil, nil
		}

		params, err := k.Params.Get(ctx)
		if err != nil {
			return simtypes.OperationMsg{}, nil, err
		}
		_, minBid, err := params.PriceQuote(len(domainPart), len(ext), 365)
		if err != nil {
			return simtypes.OperationMsg{}, nil, err
		}
		bid := minBid
		if auc.HighestBid != "" {
			if cur, ok := sdkmath.NewIntFromString(auc.HighestBid); ok {
				inc := sdkmath.NewIntFromUint64(uint64(r.Intn(10) + 1))
				bid = cur.Add(inc)
			}
		}

		msg := &types.MsgBid{
			Creator: simAccount.Address.String(),
			Domain:  domainPart,
			Ext:     ext,
			Amount:  bid.String(),
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
