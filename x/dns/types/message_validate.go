package types

import (
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = (*MsgRegister)(nil)
	_ sdk.Msg = (*MsgUpdate)(nil)
	_ sdk.Msg = (*MsgRenew)(nil)
	_ sdk.Msg = (*MsgTransfer)(nil)
	_ sdk.Msg = (*MsgBid)(nil)
	_ sdk.Msg = (*MsgSettle)(nil)
	_ sdk.Msg = (*MsgCreateDomain)(nil)
	_ sdk.Msg = (*MsgUpdateDomain)(nil)
	_ sdk.Msg = (*MsgDeleteDomain)(nil)
	_ sdk.Msg = (*MsgCreateAuction)(nil)
	_ sdk.Msg = (*MsgUpdateAuction)(nil)
	_ sdk.Msg = (*MsgDeleteAuction)(nil)
)

func (msg *MsgRegister) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if msg.Owner != "" {
		if _, err := sdk.AccAddressFromBech32(msg.Owner); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrapf("invalid owner address (%s)", err)
		}
	}
	if err := validateDomainAndExt(msg.Domain, msg.Ext); err != nil {
		return err
	}
	if err := ValidateRecords(msg.Records); err != nil {
		return err
	}
	return nil
}

func (msg *MsgUpdate) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if err := validateDomainAndExt(msg.Domain, msg.Ext); err != nil {
		return err
	}
	if len(msg.Records) > 0 {
		if err := ValidateRecords(msg.Records); err != nil {
			return err
		}
	}
	return nil
}

func (msg *MsgRenew) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if err := validateDomainAndExt(msg.Domain, msg.Ext); err != nil {
		return err
	}
	return nil
}

func (msg *MsgTransfer) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.NewOwner); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid new owner address (%s)", err)
	}
	return validateDomainAndExt(msg.Domain, msg.Ext)
}

func (msg *MsgBid) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if err := validateDomainAndExt(msg.Domain, msg.Ext); err != nil {
		return err
	}
	if strings.TrimSpace(msg.Amount) == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("amount required")
	}
	return nil
}

func (msg *MsgSettle) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	return validateDomainAndExt(msg.Domain, msg.Ext)
}

func (msg *MsgCreateDomain) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.Owner); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid owner address (%s)", err)
	}
	if err := validateIndexLike("index", msg.Index); err != nil {
		return err
	}
	if err := validateIndexLike("name", msg.Name); err != nil {
		return err
	}
	if err := ValidateRecords(msg.Records); err != nil {
		return err
	}
	return nil
}

func (msg *MsgUpdateDomain) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.Owner); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid owner address (%s)", err)
	}
	if err := validateIndexLike("index", msg.Index); err != nil {
		return err
	}
	if err := validateIndexLike("name", msg.Name); err != nil {
		return err
	}
	if len(msg.Records) > 0 {
		if err := ValidateRecords(msg.Records); err != nil {
			return err
		}
	}
	return nil
}

func (msg *MsgDeleteDomain) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	return validateIndexLike("index", msg.Index)
}

func (msg *MsgCreateAuction) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if err := validateIndexLike("index", msg.Index); err != nil {
		return err
	}
	if err := validateIndexLike("name", msg.Name); err != nil {
		return err
	}
	return nil
}

func (msg *MsgUpdateAuction) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	if err := validateIndexLike("index", msg.Index); err != nil {
		return err
	}
	if err := validateIndexLike("name", msg.Name); err != nil {
		return err
	}
	return nil
}

func (msg *MsgDeleteAuction) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address (%s)", err)
	}
	return validateIndexLike("index", msg.Index)
}

func validateDomainAndExt(domain, ext string) error {
	d := NormalizeDomain(domain)
	e := NormalizeExt(ext)
	return ValidateDomainParts(d, e)
}

func validateIndexLike(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s required", field)
	}
	if len(value) > DNSFQDNMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s too long: %d > %d", field, len(value), DNSFQDNMaxLen)
	}
	return nil
}
