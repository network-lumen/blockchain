package keeper

import (
	errorsmod "cosmossdk.io/errors"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"lumen/x/dns/types"
)

func enforceUpdateRateLimit(lastUpdated, now, limit uint64) error {
	if limit == 0 || lastUpdated == 0 {
		return nil
	}
	if now < lastUpdated {
		return nil
	}
	if now-lastUpdated < limit {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "domain updated too recently")
	}
	return nil
}

func enforceUpdatePoW(identifier, creator string, nonce uint64, difficulty uint32) error {
	if difficulty == 0 {
		return nil
	}
	sum := types.ComputeUpdatePowDigest(identifier, creator, nonce)
	if types.LeadingZeroBits(sum[:]) < difficulty {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid proof-of-work for update")
	}
	return nil
}
