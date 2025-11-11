package types

import (
	"strings"
	"unicode/utf8"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

func NewMsgLinkAccountPQC(creator sdk.AccAddress, scheme string, pubKey []byte) *MsgLinkAccountPQC {
	return &MsgLinkAccountPQC{
		Creator: creator.String(),
		Scheme:  scheme,
		PubKey:  pubKey,
	}
}

func (m *MsgLinkAccountPQC) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.GetCreator()); err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "creator: %v", err)
	}
	scheme := strings.TrimSpace(m.GetScheme())
	if scheme == "" {
		return errorsmod.Wrap(ErrSchemeUnsupported, "scheme must be provided")
	}
	if len(scheme) > PQCSchemeMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("scheme too long: %d > %d", len(scheme), PQCSchemeMaxLen)
	}
	if !utf8.ValidString(scheme) {
		return sdkerrors.ErrInvalidRequest.Wrap("scheme must be valid UTF-8")
	}
	for _, r := range scheme {
		if r < 0x20 {
			return sdkerrors.ErrInvalidRequest.Wrap("scheme contains control characters")
		}
	}
	if len(m.GetPubKey()) == 0 {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidPubKey, "public key must be provided")
	}
	if len(m.GetPubKey()) > PQCPubKeyMaxLen {
		return sdkerrors.ErrInvalidPubKey.Wrapf("public key too large: %d > %d", len(m.GetPubKey()), PQCPubKeyMaxLen)
	}
	if len(m.GetPowNonce()) == 0 {
		return errorsmod.Wrap(ErrInvalidPow, "pow nonce must be provided")
	}
	if len(m.GetPowNonce()) > PQCPowNonceMaxLen {
		return errorsmod.Wrapf(ErrInvalidPow, "pow nonce too large: %d > %d", len(m.GetPowNonce()), PQCPowNonceMaxLen)
	}
	return nil
}

func (m *MsgLinkAccountPQC) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.GetCreator())
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}
