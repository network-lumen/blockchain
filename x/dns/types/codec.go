package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateAuction{},
		&MsgUpdateAuction{},
		&MsgDeleteAuction{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateDomain{},
		&MsgUpdateDomain{},
		&MsgDeleteDomain{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgBid{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgTransfer{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRenew{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdate{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegister{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
