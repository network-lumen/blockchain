package app

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdklog "cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	protov2 "google.golang.org/protobuf/proto"
)

func TestRateLimitDecoratorGlobalCap(t *testing.T) {
	decorator := newTestRateLimiter()
	decorator.perBlockMax = 100
	decorator.perWindowMax = 100
	decorator.globalMax = 3

	var current int64
	decorator.nowFn = func() time.Time {
		return time.Unix(current, 0)
	}

	ctx, next := newRateLimitContext(t)

	for i := 0; i < decorator.globalMax; i++ {
		current = int64(i)
		tx := rateLimitTx{signers: [][]byte{addrBytes(byte(i + 1))}}
		_, err := decorator.AnteHandle(ctx, tx, false, next)
		require.NoError(t, err)
	}

	current = 10
	tx := rateLimitTx{signers: [][]byte{addrBytes(0xFF)}}
	_, err := decorator.AnteHandle(ctx, tx, false, next)
	require.ErrorIs(t, err, sdkerrors.ErrUnauthorized)
}

func TestRateLimitDecoratorPerAccountWindowCap(t *testing.T) {
	decorator := newTestRateLimiter()
	decorator.perWindowMax = 2
	decorator.perBlockMax = 5
	decorator.windowSec = 10

	var current int64
	decorator.nowFn = func() time.Time {
		return time.Unix(current, 0)
	}

	ctx, next := newRateLimitContext(t)
	signer := addrBytes(0xAB)

	current = 0
	_, err := decorator.AnteHandle(ctx, rateLimitTx{signers: [][]byte{signer}}, false, next)
	require.NoError(t, err)

	current = 1
	_, err = decorator.AnteHandle(ctx, rateLimitTx{signers: [][]byte{signer}}, false, next)
	require.NoError(t, err)

	current = 2
	_, err = decorator.AnteHandle(ctx, rateLimitTx{signers: [][]byte{signer}}, false, next)
	require.ErrorIs(t, err, sdkerrors.ErrUnauthorized)

	current = 20
	_, err = decorator.AnteHandle(ctx, rateLimitTx{signers: [][]byte{signer}}, false, next)
	require.NoError(t, err)
}

func TestRateLimitDecoratorPerBlockCap(t *testing.T) {
	decorator := newTestRateLimiter()
	decorator.perBlockMax = 1
	decorator.perWindowMax = 10

	ctx, next := newRateLimitContext(t)
	signer := addrBytes(0xCC)

	_, err := decorator.AnteHandle(ctx, rateLimitTx{signers: [][]byte{signer}}, false, next)
	require.NoError(t, err)

	_, err = decorator.AnteHandle(ctx, rateLimitTx{signers: [][]byte{signer}}, false, next)
	require.ErrorIs(t, err, sdkerrors.ErrUnauthorized)

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 1)
	_, err = decorator.AnteHandle(ctx, rateLimitTx{signers: [][]byte{signer}}, false, next)
	require.NoError(t, err)
}

func newRateLimitContext(t *testing.T) (sdk.Context, sdk.AnteHandler) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey("rate_limit_test")
	memKey := storetypes.NewTransientStoreKey("rate_limit_test_transient")
	ctx := sdktestutil.DefaultContextWithDB(t, storeKey, memKey).Ctx
	ctx = ctx.WithLogger(sdklog.NewNopLogger()).WithIsCheckTx(true).WithBlockHeight(1)

	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		return ctx, nil
	}
	return ctx, next
}

func newTestRateLimiter() *RateLimitDecorator {
	return &RateLimitDecorator{
		perBlock:      make(map[string]int),
		perWindowHist: make(map[string][]int64),
		globalHist:    make([]int64, 0, 16),
		perBlockMax:   5,
		perWindowMax:  20,
		windowSec:     10,
		globalMax:     100,
		nowFn: func() time.Time {
			return time.Unix(0, 0)
		},
		akAddr: authcodec.NewBech32Codec(AccountAddressPrefix),
	}
}

type rateLimitTx struct {
	signers [][]byte
}

var _ sdk.Tx = rateLimitTx{}
var _ authsigning.SigVerifiableTx = rateLimitTx{}

func (rateLimitTx) GetMsgs() []sdk.Msg                                   { return nil }
func (rateLimitTx) GetMsgsV2() ([]protov2.Message, error)                { return nil, nil }
func (rateLimitTx) ValidateBasic() error                                 { return nil }
func (rateLimitTx) GetMemo() string                                      { return "" }
func (rateLimitTx) GetSignaturesV2() ([]signingtypes.SignatureV2, error) { return nil, nil }
func (rateLimitTx) GetPubKeys() ([]cryptotypes.PubKey, error)            { return nil, nil }
func (m rateLimitTx) GetSigners() ([][]byte, error)                      { return m.signers, nil }
func (rateLimitTx) GetGas() uint64                                       { return 0 }
func (rateLimitTx) GetFee() sdk.Coins                                    { return nil }
func (rateLimitTx) FeePayer() []byte                                     { return nil }
func (rateLimitTx) FeeGranter() []byte                                   { return nil }
func (rateLimitTx) GetTimeoutHeight() uint64                             { return 0 }
func (rateLimitTx) GetTimeoutTimeStamp() time.Time                       { return time.Time{} }
func (rateLimitTx) GetUnordered() bool                                   { return false }

func addrBytes(b byte) []byte {
	return bytes.Repeat([]byte{b}, testAddrLen)
}

const testAddrLen = 20
