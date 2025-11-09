package app

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	gatewaysmodulekeeper "lumen/x/gateways/keeper"
	gatewaytypes "lumen/x/gateways/types"

	"cosmossdk.io/core/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

type RateLimitDecorator struct {
	akAddr address.Codec
	mu     sync.Mutex

	lastHeight    int64
	perBlock      map[string]int
	perWindowHist map[string][]int64

	perBlockMax  int
	perWindowMax int
	windowSec    int64

	globalHist []int64
	globalMax  int

	nowFn          func() time.Time
	gatewaysKeeper *gatewaysmodulekeeper.Keeper
}

const rateLimitMaxAccounts = 50000

func (d *RateLimitDecorator) Init(ak AddressCodecProvider) *RateLimitDecorator {
	d.akAddr = ak.AddressCodec()
	d.perBlock = make(map[string]int)
	d.perWindowHist = make(map[string][]int64)
	d.globalHist = make([]int64, 0, 128)
	d.perBlockMax = clampInt(mustIntEnv("LUMEN_RL_PER_BLOCK", 5), 1, 1000)
	d.perWindowMax = clampInt(mustIntEnv("LUMEN_RL_PER_WINDOW", 20), 1, 1000)
	d.windowSec = clampInt64(mustInt64Env("LUMEN_RL_WINDOW_SEC", 10), 1, 600)
	d.globalMax = clampInt(mustIntEnv("LUMEN_RL_GLOBAL_MAX", 300), 10, 100000)
	d.nowFn = time.Now
	return d
}

func (d *RateLimitDecorator) WithGatewaysKeeper(pk *gatewaysmodulekeeper.Keeper) *RateLimitDecorator {
	d.gatewaysKeeper = pk
	return d
}

type AddressCodecProvider interface{ AddressCodec() address.Codec }

func (d *RateLimitDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	if simulate || !ctx.IsCheckTx() {
		return next(ctx, tx, simulate)
	}
	signer := firstSigner(tx, d.akAddr)
	if signer == "" {
		return next(ctx, tx, simulate)
	}

	for _, m := range tx.GetMsgs() {
		if _, ok := m.(*gatewaytypes.MsgRegisterGateway); ok {
			return next(ctx, tx, simulate)
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.resetBlockIfNeeded(ctx.BlockHeight())

	now := d.nowFn().Unix()
	cutoff := now - d.windowSec

	d.pruneGlobal(cutoff)
	if len(d.globalHist) >= d.globalMax {
		return ctx, sdkerrors.ErrUnauthorized.Wrap("rate limit: global cap reached")
	}

	if err := d.applyPerBlockLimit(signer); err != nil {
		return ctx, err
	}
	if err := d.applyPerWindowLimit(signer, now, cutoff); err != nil {
		return ctx, err
	}

	d.globalHist = append(d.globalHist, now)

	return next(ctx, tx, simulate)
}

func (d *RateLimitDecorator) applyPerBlockLimit(signer string) error {
	count := d.perBlock[signer]
	if count >= d.perBlockMax {
		return sdkerrors.ErrUnauthorized.Wrap("rate limit: per-block cap reached")
	}
	d.perBlock[signer] = count + 1
	return nil
}

func (d *RateLimitDecorator) applyPerWindowLimit(signer string, now, cutoff int64) error {
	hist, existed := d.perWindowHist[signer]
	hist = pruneInt64(hist, cutoff)
	if len(hist) >= d.perWindowMax {
		return sdkerrors.ErrUnauthorized.Wrap("rate limit: account cap reached")
	}
	if len(hist) == 0 {
		if existed {
			delete(d.perWindowHist, signer)
			existed = false
		}
		if !existed && len(d.perWindowHist) >= rateLimitMaxAccounts {
			return sdkerrors.ErrUnauthorized.Wrap("rate limit: capacity reached")
		}
	}
	hist = append(hist, now)
	d.perWindowHist[signer] = hist
	return nil
}

func (d *RateLimitDecorator) pruneGlobal(cutoff int64) {
	if len(d.globalHist) == 0 {
		return
	}
	n := d.globalHist[:0]
	for _, ts := range d.globalHist {
		if ts >= cutoff {
			n = append(n, ts)
		}
	}
	d.globalHist = n
}

func pruneInt64(values []int64, cutoff int64) []int64 {
	if len(values) == 0 {
		return values
	}
	n := values[:0]
	for _, ts := range values {
		if ts >= cutoff {
			n = append(n, ts)
		}
	}
	return n
}

func (d *RateLimitDecorator) resetBlockIfNeeded(height int64) {
	if d.lastHeight == height {
		return
	}
	d.perBlock = make(map[string]int)
	d.lastHeight = height
}

func mustIntEnv(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return def
}

func mustInt64Env(key string, def int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			return parsed
		}
	}
	return def
}

func clampInt(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func clampInt64(val, min, max int64) int64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func firstSigner(tx sdk.Tx, coder address.Codec) string {
	sv, ok := tx.(authsigning.SigVerifiableTx)
	if !ok {
		return ""
	}
	addrs, err := sv.GetSigners()
	if err != nil || len(addrs) == 0 {
		return ""
	}
	bech, err := coder.BytesToString(addrs[0])
	if err != nil {
		return ""
	}
	return bech
}
