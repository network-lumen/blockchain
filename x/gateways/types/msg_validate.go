package types

import (
	"strings"
	"unicode/utf8"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = (*MsgRegisterGateway)(nil)
	_ sdk.Msg = (*MsgUpdateGateway)(nil)
	_ sdk.Msg = (*MsgCreateContract)(nil)
	_ sdk.Msg = (*MsgClaimPayment)(nil)
	_ sdk.Msg = (*MsgCancelContract)(nil)
	_ sdk.Msg = (*MsgFinalizeContract)(nil)
	_ sdk.Msg = (*MsgUpdateParams)(nil)
)

func (m *MsgRegisterGateway) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid operator address (%s)", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.Payout); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid payout address (%s)", err)
	}
	if err := validateMetadata("metadata", m.Metadata, GatewayMetadataMaxLen); err != nil {
		return err
	}
	return nil
}

func (m *MsgRegisterGateway) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Operator)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateGateway) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid operator address (%s)", err)
	}
	if m.GatewayId == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("gateway_id required")
	}
	if m.Payout != nil {
		if strings.TrimSpace(m.Payout.Value) == "" {
			return sdkerrors.ErrInvalidRequest.Wrap("payout must not be empty")
		}
		if _, err := sdk.AccAddressFromBech32(m.Payout.Value); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrapf("invalid payout address (%s)", err)
		}
	}
	if m.Metadata != nil {
		if err := validateMetadata("metadata", m.Metadata.Value, GatewayMetadataMaxLen); err != nil {
			return err
		}
	}
	return nil
}

func (m *MsgUpdateGateway) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Operator)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func (m *MsgCreateContract) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Client); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid client address (%s)", err)
	}
	if m.GatewayId == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("gateway_id required")
	}
	if m.PriceUlmn == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("price_ulmn must be > 0")
	}
	if m.MonthsTotal == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("months_total must be > 0")
	}
	if err := validateMetadata("metadata", m.Metadata, ContractMetadataMaxLen); err != nil {
		return err
	}
	return nil
}

func (m *MsgCreateContract) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Client)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func (m *MsgClaimPayment) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid operator address (%s)", err)
	}
	return nil
}

func (m *MsgClaimPayment) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Operator)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func (m *MsgCancelContract) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Client); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid client address (%s)", err)
	}
	return nil
}

func (m *MsgCancelContract) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Client)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func (m *MsgFinalizeContract) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Finalizer); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid finalizer address (%s)", err)
	}
	return nil
}

func (m *MsgFinalizeContract) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Finalizer)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid authority address (%s)", err)
	}
	return ValidateParams(m.Params)
}

func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Authority)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func validateMetadata(field, value string, max int) error {
	if value == "" {
		return nil
	}
	if len(value) > max {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s too long: %d > %d", field, len(value), max)
	}
	if !utf8.ValidString(value) {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s must be valid UTF-8", field)
	}
	for _, r := range value {
		if r < 0x20 {
			return sdkerrors.ErrInvalidRequest.Wrapf("%s contains control characters", field)
		}
	}
	return nil
}
