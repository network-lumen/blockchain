package keeper

import "lumen/x/tokenomics/types"

type msgServer struct{ Keeper }

var _ types.MsgServer = msgServer{}

func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return msgServer{Keeper: keeper}
}
