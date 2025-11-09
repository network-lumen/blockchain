package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	gogoproto "github.com/gogo/protobuf/proto"
)

func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgLinkAccountPQC{},
	)

	registrar.RegisterImplementations((*gogoproto.Message)(nil),
		&PQCSignatures{},
		&PQCSignatureEntry{},
	)

	registrar.RegisterImplementations((*sdktx.TxExtensionOptionI)(nil),
		&PQCSignatures{},
	)
}
