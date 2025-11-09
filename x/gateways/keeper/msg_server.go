package keeper

import "lumen/x/gateways/types"

type msgServer struct{ Keeper }

func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}
