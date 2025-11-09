package types

import sdk "github.com/cosmos/cosmos-sdk/types"

var _ sdk.Msg = &MsgSettle{}

func NewMsgSettle(creator, domain, ext string) *MsgSettle {
	return &MsgSettle{
		Creator: creator,
		Domain:  domain,
		Ext:     ext,
	}
}

func (msg *MsgSettle) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(msg.Creator)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}
