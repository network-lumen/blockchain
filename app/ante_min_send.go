package app

import (
	"context"

	tokenomicstypes "lumen/x/tokenomics/types"

	"cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type minSendAccountKeeper interface {
	AddressCodec() address.Codec
	GetAccount(ctx context.Context, addr sdk.AccAddress) sdk.AccountI
}

type minSendTokenomicsKeeper interface {
	GetParams(ctx context.Context) tokenomicstypes.Params
}

type MinSendDecorator struct {
	ak minSendAccountKeeper
	tk minSendTokenomicsKeeper
}

func NewMinSendDecorator(
	ak minSendAccountKeeper,
	tk minSendTokenomicsKeeper,
) MinSendDecorator {
	return MinSendDecorator{ak: ak, tk: tk}
}

func (d MinSendDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	params := d.tk.GetParams(ctx)
	if params.Denom == "" || params.MinSendUlmn == 0 {
		return next(ctx, tx, simulate)
	}

	minAmount := sdkmath.NewIntFromUint64(params.MinSendUlmn)
	denom := params.Denom

	for _, msg := range tx.GetMsgs() {
		switch m := msg.(type) {
		case *banktypes.MsgSend:
			skip, err := d.skipSend(ctx, m.FromAddress, m.ToAddress)
			if err != nil {
				return ctx, err
			}
			if skip {
				continue
			}
			if err := enforceMinAmount(m.Amount, denom, minAmount); err != nil {
				return ctx, err
			}
		case *banktypes.MsgMultiSend:
			skip, err := d.multiSendByModule(ctx, m)
			if err != nil {
				return ctx, err
			}
			if skip {
				continue
			}
			for _, output := range m.Outputs {
				isModule, err := d.isModuleAddress(ctx, output.Address)
				if err != nil {
					return ctx, err
				}
				if isModule {
					continue
				}
				if err := enforceMinAmount(output.Coins, denom, minAmount); err != nil {
					return ctx, err
				}
			}
		}
	}

	return next(ctx, tx, simulate)
}

func enforceMinAmount(coins sdk.Coins, denom string, min sdkmath.Int) error {
	if amt := coins.AmountOf(denom); amt.IsPositive() && amt.LT(min) {
		return sdkerrors.ErrInvalidRequest.Wrapf("minimum send is %s%s", min.String(), denom)
	}
	return nil
}

func (d MinSendDecorator) skipSend(ctx sdk.Context, from, to string) (bool, error) {
	isModuleFrom, err := d.isModuleAddress(ctx, from)
	if err != nil {
		return false, err
	}
	if isModuleFrom {
		return true, nil
	}
	isModuleTo, err := d.isModuleAddress(ctx, to)
	if err != nil {
		return false, err
	}
	return isModuleTo, nil
}

func (d MinSendDecorator) multiSendByModule(ctx sdk.Context, msg *banktypes.MsgMultiSend) (bool, error) {
	for _, input := range msg.Inputs {
		isModule, err := d.isModuleAddress(ctx, input.Address)
		if err != nil {
			return false, err
		}
		if isModule {
			return true, nil
		}
	}
	return false, nil
}

func (d MinSendDecorator) isModuleAddress(ctx sdk.Context, addr string) (bool, error) {
	if addr == "" {
		return false, nil
	}
	bz, err := d.ak.AddressCodec().StringToBytes(addr)
	if err != nil {
		return false, sdkerrors.ErrInvalidAddress.Wrapf("invalid address %s", addr)
	}
	acc := d.ak.GetAccount(ctx, sdk.AccAddress(bz))
	if acc == nil {
		return false, nil
	}
	_, ok := acc.(sdk.ModuleAccountI)
	return ok, nil
}
