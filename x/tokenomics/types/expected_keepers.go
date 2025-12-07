package types

import (
	"context"

	"cosmossdk.io/core/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

type BankKeeper interface {
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error
}

type DistributionKeeper interface {
	WithdrawValidatorCommission(ctx context.Context, valAddr sdk.ValAddress) (sdk.Coins, error)
	FundCommunityPool(ctx context.Context, amount sdk.Coins, depositor sdk.AccAddress) error
}

type StakingKeeper interface {
	IterateValidators(ctx context.Context, fn func(index int64, validator stakingtypes.ValidatorI) (stop bool))
	ValidatorAddressCodec() address.Codec
}

type SlashingKeeper interface {
	GetParams(ctx context.Context) (slashingtypes.Params, error)
	SetParams(ctx context.Context, params slashingtypes.Params) error
}
