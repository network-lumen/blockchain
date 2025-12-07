package keeper

import (
	"errors"
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func (k Keeper) BeginBlocker(ctx sdk.Context) {
	if !k.HasBankKeeper() {
		return
	}

	k.handleSlashEvents(ctx)

	params := k.GetParams(ctx)
	if params.InitialRewardPerBlockLumn == 0 || params.SupplyCapLumn == 0 {
		return
	}

	multiplier := sdkmath.NewIntWithDecimal(1, int(params.Decimals))
	if multiplier.IsZero() {
		return
	}

	initialRewardUlmn := sdkmath.NewIntFromUint64(params.InitialRewardPerBlockLumn).Mul(multiplier)
	if !initialRewardUlmn.IsPositive() {
		return
	}

	halvingInterval := params.HalvingIntervalBlocks
	height := uint64(ctx.BlockHeight())
	var epoch uint64
	if halvingInterval > 0 {
		epoch = height / halvingInterval
	} else {
		epoch = 0
	}

	reward := initialRewardUlmn
	for i := uint64(0); i < epoch && reward.IsPositive(); i++ {
		reward = reward.QuoRaw(2)
	}

	if !reward.IsPositive() {
		return
	}

	capUlmn := sdkmath.NewIntFromUint64(params.SupplyCapLumn).Mul(multiplier)
	totalMinted := k.GetTotalMintedUlmn(ctx)
	if totalMinted.GTE(capUlmn) {
		return
	}

	remaining := capUlmn.Sub(totalMinted)
	toMint := reward
	if toMint.GT(remaining) {
		toMint = remaining
	}

	if !toMint.IsPositive() {
		return
	}

	coins := sdk.NewCoins(sdk.NewCoin(params.Denom, toMint))
	k.MintToFeeCollector(ctx, coins)

	if err := k.SetTotalMintedUlmn(ctx, totalMinted.Add(toMint)); err != nil {
		panic(err)
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"block_reward",
			sdk.NewAttribute("block", fmt.Sprintf("%d", ctx.BlockHeight())),
			sdk.NewAttribute("minted_ulmn", toMint.String()),
			sdk.NewAttribute("total_minted_ulmn", k.GetTotalMintedUlmn(ctx).String()),
		),
	)

	if params.DistributionIntervalBlocks > 0 &&
		k.HasDistributionKeeper() &&
		k.HasStakingKeeper() &&
		height > 0 && height%params.DistributionIntervalBlocks == 0 {

		k.staking.IterateValidators(ctx, func(_ int64, val stakingtypes.ValidatorI) bool {
			valAddrBz, err := k.staking.ValidatorAddressCodec().StringToBytes(val.GetOperator())
			if err != nil {
				panic(err)
			}
			if _, err := k.distr.WithdrawValidatorCommission(ctx, sdk.ValAddress(valAddrBz)); err != nil {
				if errors.Is(err, distrtypes.ErrNoValidatorCommission) {
					return false
				}
				panic(err)
			}
			return false
		})
	}
}

func (k Keeper) handleSlashEvents(ctx sdk.Context) {
	if !k.HasDistributionKeeper() {
		return
	}

	events := ctx.EventManager().Events()
	if len(events) == 0 {
		return
	}

	for _, ev := range events {
		if ev.Type != slashingtypes.EventTypeSlash {
			continue
		}

		var reason string
		var burnedRaw string

		for _, attr := range ev.Attributes {
			key := string(attr.Key)
			switch key {
			case slashingtypes.AttributeKeyReason:
				reason = string(attr.Value)
			case slashingtypes.AttributeKeyBurnedCoins:
				burnedRaw = string(attr.Value)
			}
		}

		if reason != slashingtypes.AttributeValueDoubleSign {
			continue
		}
		if strings.TrimSpace(burnedRaw) == "" {
			continue
		}

		coins, err := sdk.ParseCoinsNormalized(burnedRaw)
		if err != nil || !coins.IsAllPositive() {
			continue
		}

		k.MintToCommunityPool(ctx, coins)
	}
}
