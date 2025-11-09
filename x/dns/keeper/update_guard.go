package keeper

import (
	"crypto/sha256"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
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
	payload := fmt.Sprintf("%s|%s|%d", identifier, creator, nonce)
	sum := sha256.Sum256([]byte(payload))
	if leadingZeroBits(sum[:]) < difficulty {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid proof-of-work for update")
	}
	return nil
}

func leadingZeroBits(b []byte) uint32 {
	var count uint32
	for _, v := range b {
		if v == 0 {
			count += 8
			continue
		}
		for i := 7; i >= 0; i-- {
			if (v>>uint(i))&1 == 0 {
				count++
			} else {
				return count
			}
		}
	}
	return count
}
