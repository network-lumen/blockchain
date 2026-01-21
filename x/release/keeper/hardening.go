package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	math "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	releasetypes "lumen/x/release/types"
)

const (
	releaseHardeningStateVersion uint64 = 2
	maxExpirationsPerBlock              = 200
	ulmnDenom                           = "ulmn"
)

func (k Keeper) ensureHardeningState(ctx context.Context) error {
	v, err := k.StateVersion.Get(ctx)
	if err == nil && v >= releaseHardeningStateVersion {
		return nil
	}

	// Rebuild latest index so that it only includes VALIDATED releases.
	if err := k.ByTriple.Clear(ctx, nil); err != nil {
		return err
	}
	if err := k.rebuildValidatedIndex(ctx); err != nil {
		return err
	}

	// Rebuild expiry queue based on the current max_pending_ttl.
	params := k.GetParams(ctx)
	if err := k.rebuildExpiryQueue(ctx, params.MaxPendingTtl); err != nil {
		return err
	}

	return k.StateVersion.Set(ctx, releaseHardeningStateVersion)
}

func (k Keeper) rebuildValidatedIndex(ctx context.Context) error {
	return k.Release.Walk(ctx, nil, func(id uint64, r releasetypes.Release) (bool, error) {
		if r.Yanked || r.Status != releasetypes.Release_VALIDATED {
			return false, nil
		}
		for _, a := range r.Artifacts {
			if a == nil {
				continue
			}
			key := tripleKey(r.Channel, a.Platform, a.Kind)
			existingID, err := k.ByTriple.Get(ctx, key)
			if err == nil && existingID > id {
				continue
			}
			if err := k.ByTriple.Set(ctx, key, id); err != nil {
				return true, err
			}
		}
		return false, nil
	})
}

func (k Keeper) rebuildExpiryQueue(ctx context.Context, ttl uint64) error {
	if err := k.ExpiryQueue.Clear(ctx, nil); err != nil {
		return err
	}
	if err := k.ExpiryByID.Clear(ctx, nil); err != nil {
		return err
	}
	if err := k.ExpiryTTL.Set(ctx, ttl); err != nil {
		return err
	}
	if ttl == 0 {
		return nil
	}

	return k.Release.Walk(ctx, nil, func(id uint64, r releasetypes.Release) (bool, error) {
		if r.Yanked || r.Status != releasetypes.Release_PENDING {
			return false, nil
		}
		expiryTime := r.CreatedAt + int64(ttl)
		if err := k.enqueueExpiry(ctx, id, expiryTime); err != nil {
			return true, err
		}
		return false, nil
	})
}

func (k Keeper) ensureExpiryQueueTTL(ctx context.Context, ttl uint64) error {
	prev, err := k.ExpiryTTL.Get(ctx)
	if err == nil && prev == ttl {
		return nil
	}
	return k.rebuildExpiryQueue(ctx, ttl)
}

func (k Keeper) enqueueExpiry(ctx context.Context, id uint64, expiryTime int64) error {
	if err := k.ExpiryByID.Set(ctx, id, expiryTime); err != nil {
		return err
	}
	return k.ExpiryQueue.Set(ctx, collections.Join(expiryTime, id), true)
}

func (k Keeper) dequeueExpiry(ctx context.Context, id uint64) error {
	expiryTime, err := k.ExpiryByID.Get(ctx, id)
	if err == nil {
		_ = k.ExpiryQueue.Remove(ctx, collections.Join(expiryTime, id))
		_ = k.ExpiryByID.Remove(ctx, id)
		return nil
	}
	return nil
}

func (k Keeper) chargeEscrow(ctx context.Context, id uint64, publisher sdk.AccAddress, amountUlmn uint64) error {
	if amountUlmn == 0 {
		return nil
	}
	if k.bank == nil {
		return fmt.Errorf("bank keeper not configured")
	}

	coins := sdk.NewCoins(sdk.NewCoin(ulmnDenom, math.NewIntFromUint64(amountUlmn)))
	if err := k.bank.SendCoinsFromAccountToModule(ctx, publisher, releasetypes.ModuleName, coins); err != nil {
		return err
	}
	if err := k.EscrowAmount.Set(ctx, id, amountUlmn); err != nil {
		return err
	}
	return k.EscrowPublisher.Set(ctx, id, publisher.String())
}

func (k Keeper) refundEscrow(ctx context.Context, id uint64) error {
	amt, err := k.EscrowAmount.Get(ctx, id)
	if err != nil || amt == 0 {
		return nil
	}
	publisherStr, err := k.EscrowPublisher.Get(ctx, id)
	if err != nil {
		return err
	}
	pubBz, err := k.addressCodec.StringToBytes(publisherStr)
	if err != nil {
		return err
	}
	publisher := sdk.AccAddress(pubBz)

	coins := sdk.NewCoins(sdk.NewCoin(ulmnDenom, math.NewIntFromUint64(amt)))
	if err := k.bank.SendCoinsFromModuleToAccount(ctx, releasetypes.ModuleName, publisher, coins); err != nil {
		return err
	}
	_ = k.EscrowAmount.Remove(ctx, id)
	_ = k.EscrowPublisher.Remove(ctx, id)
	return nil
}

func (k Keeper) forfeitEscrowToCommunityPool(ctx context.Context, id uint64) error {
	amt, err := k.EscrowAmount.Get(ctx, id)
	if err != nil || amt == 0 {
		return nil
	}
	if k.distr == nil {
		return fmt.Errorf("distribution keeper not configured")
	}

	coins := sdk.NewCoins(sdk.NewCoin(ulmnDenom, math.NewIntFromUint64(amt)))
	moduleAddr := authtypes.NewModuleAddress(releasetypes.ModuleName)
	if err := k.distr.FundCommunityPool(ctx, coins, moduleAddr); err != nil {
		return err
	}
	_ = k.EscrowAmount.Remove(ctx, id)
	_ = k.EscrowPublisher.Remove(ctx, id)
	return nil
}

func (k Keeper) expireRelease(ctx context.Context, id uint64) error {
	r, err := k.Release.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			_ = k.dequeueExpiry(ctx, id)
			return nil
		}
		return err
	}

	if r.Yanked || r.Status != releasetypes.Release_PENDING {
		_ = k.dequeueExpiry(ctx, id)
		return nil
	}

	r.Status = releasetypes.Release_EXPIRED
	r.EmergencyOk = false
	r.EmergencyUntil = 0
	if err := k.Release.Set(ctx, id, r); err != nil {
		return err
	}

	if err := k.dequeueExpiry(ctx, id); err != nil {
		return err
	}
	if err := k.forfeitEscrowToCommunityPool(ctx, id); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"release_expire",
		sdk.NewAttribute("id", fmt.Sprintf("%d", id)),
	))

	return nil
}

func (k Keeper) EndBlocker(ctx context.Context) error {
	if err := k.ensureHardeningState(ctx); err != nil {
		return err
	}

	params := k.GetParams(ctx)
	ttl := params.MaxPendingTtl
	if err := k.ensureExpiryQueueTTL(ctx, ttl); err != nil {
		return err
	}
	if ttl == 0 {
		return nil
	}

	now := k.nowUnix(ctx)
	// Iterate expiry queue up to (and including) now.
	var toExpire []collections.Pair[int64, uint64]
	iter, err := k.ExpiryQueue.Iterate(ctx, collections.NewPrefixUntilPairRange[int64, uint64](now))
	if err != nil {
		return err
	}
	defer iter.Close()

	for ; iter.Valid() && len(toExpire) < maxExpirationsPerBlock; iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return err
		}
		toExpire = append(toExpire, kv.Key)
	}

	for _, key := range toExpire {
		if err := k.expireRelease(ctx, key.K2()); err != nil {
			if errors.Is(err, collections.ErrNotFound) {
				continue
			}
			return err
		}
	}

	return nil
}

func (k Keeper) enforcePendingAndNotYanked(r releasetypes.Release) error {
	if r.Yanked {
		return errorsmod.Wrap(releasetypes.ErrNotAuthorized, "release is yanked")
	}
	if r.Status != releasetypes.Release_PENDING {
		return errorsmod.Wrap(releasetypes.ErrNotPending, "release must be pending")
	}
	return nil
}
