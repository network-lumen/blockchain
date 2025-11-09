package app

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	txv1beta1 "cosmossdk.io/api/cosmos/tx/v1beta1"
	storetypes "cosmossdk.io/store/types"
	txsigning "cosmossdk.io/x/tx/signing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectestutil "github.com/cosmos/cosmos-sdk/codec/testutil"
	"github.com/cosmos/cosmos-sdk/std"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"google.golang.org/protobuf/proto"

	"lumen/crypto/pqc/dilithium"
	gatewaytypes "lumen/x/gateways/types"
	pqcmodule "lumen/x/pqc/module"
	pqcsigning "lumen/x/pqc/signing"
	pqctypes "lumen/x/pqc/types"
)

type mockAccountKeeper struct {
	accounts map[string]sdk.AccountI
}

func (m mockAccountKeeper) GetAccount(ctx context.Context, addr sdk.AccAddress) sdk.AccountI {
	return m.accounts[addr.String()]
}

type mockPQCKeeper struct {
	params   pqctypes.Params
	accounts map[string]pqctypes.AccountPQC
	scheme   dilithium.Scheme
}

func (m mockPQCKeeper) GetParams(ctx context.Context) pqctypes.Params {
	return m.params
}

func (m mockPQCKeeper) GetAccountPQC(ctx context.Context, addr sdk.AccAddress) (pqctypes.AccountPQC, bool, error) {
	info, ok := m.accounts[addr.String()]
	return info, ok, nil
}

func (m mockPQCKeeper) Scheme() dilithium.Scheme {
	return m.scheme
}

type pqcWrappedTx struct {
	authsigning.Tx
	sigs map[string]*pqctypes.PQCSignatureEntry
}

func (w pqcWrappedTx) GetPQCSignatures() map[string]*pqctypes.PQCSignatureEntry {
	return w.sigs
}

func TestPQCDualSignDecoratorRequiredMissingKey(t *testing.T) {
	env := newPQCTestEnv(t)
	params := defaultParams()
	decorator := env.decorator(params)

	addr := env.addresses[0]
	delete(env.pqc.accounts, addr.String())

	baseTx, _, txBytes, _ := env.buildBaseTx([]sdk.Msg{testdata.NewTestMsg(addr)})
	ctx := env.newContext(txBytes)

	tx := pqcWrappedTx{Tx: baseTx, sigs: nil}

	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.ErrorIs(t, err, pqctypes.ErrPQCRequired)
	require.Len(t, ctx.EventManager().Events(), 0)
}

func TestPQCDualSignDecoratorRequiredMissingSignature(t *testing.T) {
	env := newPQCTestEnv(t)
	params := defaultParams()
	decorator := env.decorator(params)

	addr := env.addresses[0]

	baseTx, _, txBytes, _ := env.buildBaseTx([]sdk.Msg{env.msg(addr)})
	ctx := env.newContext(txBytes)

	tx := pqcWrappedTx{Tx: baseTx, sigs: nil}
	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.ErrorIs(t, err, pqctypes.ErrMissingExtension)
}

func TestPQCDualSignDecoratorRequiredWrongScheme(t *testing.T) {
	env := newPQCTestEnv(t)
	params := defaultParams()
	decorator := env.decorator(params)

	addr := env.addresses[0]

	baseTx, signers, txBytes, txRaw := env.buildBaseTx([]sdk.Msg{env.msg(addr)})
	ctx := env.newContext(txBytes)

	sigs := env.makeSignatures(decorator, txRaw, signers)
	for _, s := range sigs {
		s.Scheme = "invalid"
	}

	tx := pqcWrappedTx{Tx: baseTx, sigs: sigs}
	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.ErrorIs(t, err, pqctypes.ErrInvalidScheme)
}

func TestPQCDualSignDecoratorRequiredBadSignature(t *testing.T) {
	env := newPQCTestEnv(t)
	params := defaultParams()
	decorator := env.decorator(params)

	addr := env.addresses[0]

	baseTx, signers, txBytes, txRaw := env.buildBaseTx([]sdk.Msg{env.msg(addr)})
	ctx := env.newContext(txBytes)

	sigs := env.makeSignatures(decorator, txRaw, signers)
	for _, s := range sigs {
		s.Signature[0] ^= 0xFF
	}

	tx := pqcWrappedTx{Tx: baseTx, sigs: sigs}
	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.ErrorIs(t, err, pqctypes.ErrPQCVerifyFailed)
}

func TestPQCDualSignDecoratorRequiredSignatureEntryMissing(t *testing.T) {
	env := newPQCTestEnv(t)
	params := defaultParams()
	decorator := env.decorator(params)

	addr := env.addresses[0]
	other := env.addresses[1]

	baseTx, _, txBytes, txRaw := env.buildBaseTx([]sdk.Msg{env.msg(addr)})
	ctx := env.newContext(txBytes)

	sigs := env.makeSignatures(decorator, txRaw, []sdk.AccAddress{other})
	tx := pqcWrappedTx{Tx: baseTx, sigs: sigs}

	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.ErrorIs(t, err, pqctypes.ErrPQCRequired)
}

func TestPQCDualSignDecoratorRequiredMultiSigner(t *testing.T) {
	env := newPQCTestEnv(t)
	params := defaultParams()
	decorator := env.decorator(params)

	msgs := []sdk.Msg{env.msg(env.addresses[0]), env.msg(env.addresses[1])}
	baseTx, signers, txBytes, txRaw := env.buildBaseTx(msgs)
	ctx := env.newContext(txBytes)

	sigs := env.makeSignatures(decorator, txRaw, signers)
	tx := pqcWrappedTx{Tx: baseTx, sigs: sigs}

	newCtx, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.NoError(t, err)

	events := newCtx.EventManager().Events()
	require.Len(t, events, len(signers))
	for _, ev := range events {
		require.Equal(t, pqctypes.EventTypeVerified, ev.Type)
	}
}

func TestPQCDualSignDecoratorGovMsgRequiresPQC(t *testing.T) {
	env := newPQCTestEnv(t)
	params := defaultParams()
	decorator := env.decorator(params)

	addr := env.addresses[0]
	msg := &gatewaytypes.MsgUpdateParams{
		Authority: addr.String(),
		Params:    gatewaytypes.DefaultParams(),
	}

	baseTx, signers, txBytes, txRaw := env.buildBaseTx([]sdk.Msg{msg})
	ctx := env.newContext(txBytes)

	tx := pqcWrappedTx{Tx: baseTx, sigs: nil}
	_, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.ErrorIs(t, err, pqctypes.ErrMissingExtension)

	sigs := env.makeSignatures(decorator, txRaw, signers)
	tx = pqcWrappedTx{Tx: baseTx, sigs: sigs}
	_, err = decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.NoError(t, err)
}

func TestPQCDualSignDecoratorOptionalMissingSignature(t *testing.T) {
	env := newPQCTestEnv(t)
	params := defaultParams()
	params.Policy = pqctypes.PqcPolicy_PQC_POLICY_OPTIONAL
	decorator := env.decorator(params)

	addr := env.addresses[0]

	baseTx, signers, txBytes, txRaw := env.buildBaseTx([]sdk.Msg{env.msg(addr)})
	ctx := env.newContext(txBytes)

	sigs := env.makeSignatures(decorator, txRaw, signers)
	delete(sigs, addr.String())

	tx := pqcWrappedTx{Tx: baseTx, sigs: sigs}
	newCtx, err := decorator.AnteHandle(ctx, tx, false, nextAnte)
	require.NoError(t, err)

	events := newCtx.EventManager().Events()
	require.Len(t, events, 1)
	require.Equal(t, pqctypes.EventTypeMissing, events[0].Type)
}

func TestPQCDualSignDecoratorSignDocPrefix(t *testing.T) {
	env := newPQCTestEnv(t)

	addr := env.addresses[0]

	_, _, txBytes, txRaw := env.buildBaseTx([]sdk.Msg{env.msg(addr)})
	ctx := env.newContext(txBytes)

	account := env.ak.accounts[addr.String()]
	signBytes, err := pqcsigning.ComputeSignBytes(ctx.ChainID(), account.GetAccountNumber(), txRaw)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(signBytes, []byte(pqctypes.PQCSignDocPrefix)))
}

/* ---------- test scaffolding ---------- */

type pqcTestEnv struct {
	t         *testing.T
	encCfg    moduletestutil.TestEncodingConfig
	ctx       sdk.Context
	ak        mockAccountKeeper
	pqc       mockPQCKeeper
	scheme    dilithium.Scheme
	privKeys  map[string]dilithium.PrivateKey
	addresses []sdk.AccAddress
}

func newPQCTestEnv(t *testing.T) *pqcTestEnv {
	t.Helper()

	encCfg := makeTestEncodingConfig(t, pqcmodule.AppModule{})
	storeKey := storetypes.NewKVStoreKey("pqc_test")
	memKey := storetypes.NewTransientStoreKey("pqc_test_transient")
	ctx := sdktestutil.DefaultContextWithDB(t, storeKey, memKey).Ctx
	ctx = ctx.WithBlockHeight(100).WithBlockTime(time.Unix(1000, 0)).WithChainID("pqc-test")

	scheme := dilithium.Default()

	env := &pqcTestEnv{
		t:        t,
		encCfg:   encCfg,
		ctx:      ctx,
		ak:       mockAccountKeeper{accounts: make(map[string]sdk.AccountI)},
		pqc:      mockPQCKeeper{accounts: make(map[string]pqctypes.AccountPQC), scheme: scheme},
		scheme:   scheme,
		privKeys: make(map[string]dilithium.PrivateKey),
	}

	for i := 0; i < 2; i++ {
		addr := sdk.AccAddress([]byte{byte(i + 1), 0x01, 0x02, 0x03, 0x04})
		base := authtypes.NewBaseAccountWithAddress(addr)
		if err := base.SetAccountNumber(uint64(i + 1)); err != nil {
			t.Fatalf("SetAccountNumber: %v", err)
		}
		if err := base.SetSequence(uint64(i + 1)); err != nil {
			t.Fatalf("SetSequence: %v", err)
		}
		env.ak.accounts[addr.String()] = base
		env.addresses = append(env.addresses, addr)

		pub, priv, err := scheme.GenerateKey(bytes.Repeat([]byte{byte(i + 1)}, 32))
		require.NoError(t, err)
		env.privKeys[addr.String()] = priv
		env.pqc.accounts[addr.String()] = pqctypes.AccountPQC{
			Addr:    addr.String(),
			Scheme:  pqctypes.SchemeDilithium3,
			PubKey:  pub,
			AddedAt: env.ctx.BlockTime().Unix(),
		}
	}

	return env
}

func makeTestEncodingConfig(t *testing.T, modules ...module.AppModuleBasic) moduletestutil.TestEncodingConfig {
	t.Helper()

	amino := codec.NewLegacyAmino()
	codecOptions := codectestutil.CodecOptions{
		AccAddressPrefix: AccountAddressPrefix,
		ValAddressPrefix: AccountAddressPrefix + "valoper",
	}
	interfaceRegistry := codecOptions.NewInterfaceRegistry()
	protoCodec := codec.NewProtoCodec(interfaceRegistry)

	signOpts := txsigning.Options{
		AddressCodec:          authcodec.NewBech32Codec(AccountAddressPrefix),
		ValidatorAddressCodec: authcodec.NewBech32Codec(AccountAddressPrefix + "valoper"),
	}
	txConfig, err := authtx.NewTxConfigWithOptions(protoCodec, authtx.ConfigOptions{
		SigningOptions: &signOpts,
	})
	require.NoError(t, err)

	encCfg := moduletestutil.TestEncodingConfig{
		InterfaceRegistry: interfaceRegistry,
		Codec:             protoCodec,
		TxConfig:          txConfig,
		Amino:             amino,
	}

	mb := module.NewBasicManager(modules...)
	std.RegisterLegacyAminoCodec(encCfg.Amino)
	std.RegisterInterfaces(encCfg.InterfaceRegistry)
	mb.RegisterLegacyAminoCodec(encCfg.Amino)
	mb.RegisterInterfaces(encCfg.InterfaceRegistry)

	return encCfg
}

func (env *pqcTestEnv) decorator(params pqctypes.Params) PQCDualSignDecorator {
	env.pqc.params = params
	return NewPQCDualSignDecorator(nil, env.ak, env.pqc, env.encCfg.Codec)
}

func (env *pqcTestEnv) msg(addr sdk.AccAddress) sdk.Msg {
	return &pqctypes.MsgLinkAccountPQC{
		Creator: addr.String(),
		Scheme:  pqctypes.SchemeDilithium3,
		PubKey:  []byte{0x01},
	}
}

func (env *pqcTestEnv) buildBaseTx(msgs []sdk.Msg) (authsigning.Tx, []sdk.AccAddress, []byte, *txv1beta1.TxRaw) {
	txBuilder := env.encCfg.TxConfig.NewTxBuilder()
	require.NoError(env.t, txBuilder.SetMsgs(msgs...))
	txBuilder.SetGasLimit(200000)

	signers := env.uniqueSigners(msgs)
	sigs := make([]signingtypes.SignatureV2, len(signers))
	for i, addr := range signers {
		account := env.ak.accounts[addr.String()]
		sigs[i] = signingtypes.SignatureV2{
			PubKey: nil,
			Data: &signingtypes.SingleSignatureData{
				SignMode:  signingtypes.SignMode_SIGN_MODE_DIRECT,
				Signature: []byte{byte(i + 1)},
			},
			Sequence: account.GetSequence(),
		}
	}
	if len(sigs) > 0 {
		require.NoError(env.t, txBuilder.SetSignatures(sigs...))
	}

	tx := txBuilder.GetTx()
	txBytes, err := env.encCfg.TxConfig.TxEncoder()(tx)
	require.NoError(env.t, err)

	txRaw := new(txv1beta1.TxRaw)
	require.NoError(env.t, proto.Unmarshal(txBytes, txRaw))

	return tx, signers, txBytes, txRaw
}

func (env *pqcTestEnv) uniqueSigners(msgs []sdk.Msg) []sdk.AccAddress {
	seen := make(map[string]struct{})
	var out []sdk.AccAddress
	for _, msg := range msgs {
		type signerGetter interface {
			GetSigners() []sdk.AccAddress
		}
		if sg, ok := msg.(signerGetter); ok {
			for _, signer := range sg.GetSigners() {
				if _, exists := seen[signer.String()]; exists {
					continue
				}
				seen[signer.String()] = struct{}{}
				out = append(out, signer)
			}
		}
	}
	return out
}

func (env *pqcTestEnv) makeSignatures(decorator PQCDualSignDecorator, txRaw *txv1beta1.TxRaw, signers []sdk.AccAddress) map[string]*pqctypes.PQCSignatureEntry {
	out := make(map[string]*pqctypes.PQCSignatureEntry, len(signers))
	for _, addr := range signers {
		account := env.ak.accounts[addr.String()]
		signBytes, err := pqcsigning.ComputeSignBytes(env.ctx.ChainID(), account.GetAccountNumber(), txRaw)
		require.NoError(env.t, err)
		sig, err := env.scheme.Sign(env.privKeys[addr.String()], signBytes)
		require.NoError(env.t, err)
		out[addr.String()] = &pqctypes.PQCSignatureEntry{
			Addr:      addr.String(),
			Scheme:    pqctypes.SchemeDilithium3,
			Signature: sig,
		}
	}
	return out
}

func (env *pqcTestEnv) newContext(txBytes []byte) sdk.Context {
	return env.ctx.WithTxBytes(txBytes).WithEventManager(sdk.NewEventManager())
}

func defaultParams() pqctypes.Params {
	return pqctypes.Params{
		Policy:             pqctypes.PqcPolicy_PQC_POLICY_REQUIRED,
		MinScheme:          pqctypes.SchemeDilithium3,
		AllowAccountRotate: false,
	}
}

func nextAnte(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
	return ctx, nil
}
