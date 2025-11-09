package app

import (
	"strings"

	"cosmossdk.io/core/address"
	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type sendTaxRecord struct {
	addr  sdk.AccAddress
	coins sdk.Coins
}

func computeSendTaxes(tx sdk.Tx, rate sdkmath.LegacyDec, addrCodec address.Codec) (map[string]*sendTaxRecord, sdk.Coins, error) {
	if !rate.IsPositive() {
		return nil, nil, nil
	}

	perPayer := make(map[string]*sendTaxRecord)
	var total sdk.Coins

	addTax := func(bech string, denom string, amt sdkmath.Int) error {
		if amt.IsZero() {
			return errorsmod.Wrapf(ErrSendAmountTooSmall, "denom %s transfer too small for tax", denom)
		}

		if amt.IsNegative() {
			return errorsmod.Wrapf(ErrSendAmountTooSmall, "negative tax for denom %s", denom)
		}

		bz, err := addrCodec.StringToBytes(bech)
		if err != nil {
			return err
		}
		key := string(bz)
		rec, ok := perPayer[key]
		if !ok {
			rec = &sendTaxRecord{addr: sdk.AccAddress(bz), coins: sdk.NewCoins()}
			perPayer[key] = rec
		}
		rec.coins = rec.coins.Add(sdk.NewCoin(denom, amt))
		total = total.Add(sdk.NewCoin(denom, amt))
		return nil
	}

	msgs := tx.GetMsgs()
	for _, msg := range msgs {
		switch m := msg.(type) {
		case *banktypes.MsgSend:
			payer := strings.TrimSpace(m.ToAddress)
			if payer == "" {
				continue
			}
			for _, coin := range m.Amount {
				if !coin.Amount.IsPositive() {
					continue
				}
				tax := rate.MulInt(coin.Amount).TruncateInt()
				if !tax.IsPositive() {
					return nil, nil, errorsmod.Wrapf(ErrSendAmountTooSmall, "coin %s too small to apply tax", coin.String())
				}
				if tax.GTE(coin.Amount) {
					return nil, nil, errorsmod.Wrapf(ErrSendAmountTooSmall, "coin %s leaves no amount after tax", coin.String())
				}
				if err := addTax(payer, coin.Denom, tax); err != nil {
					return nil, nil, err
				}
			}
		case *banktypes.MsgMultiSend:
			for _, output := range m.Outputs {
				payer := strings.TrimSpace(output.Address)
				if payer == "" {
					continue
				}
				for _, coin := range output.Coins {
					if !coin.Amount.IsPositive() {
						continue
					}
					tax := rate.MulInt(coin.Amount).TruncateInt()
					if !tax.IsPositive() {
						return nil, nil, errorsmod.Wrapf(ErrSendAmountTooSmall, "coin %s too small to apply tax", coin.String())
					}
					if tax.GTE(coin.Amount) {
						return nil, nil, errorsmod.Wrapf(ErrSendAmountTooSmall, "coin %s leaves no amount after tax", coin.String())
					}
					if err := addTax(payer, coin.Denom, tax); err != nil {
						return nil, nil, err
					}
				}
			}
		default:
		}
	}

	if len(perPayer) == 0 {
		return nil, nil, nil
	}

	return perPayer, total, nil
}
