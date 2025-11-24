package preflight_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"lumen/app"
	"lumen/crypto/pqc/dilithium"
	pqctypes "lumen/x/pqc/types"
	tokenomicstypes "lumen/x/tokenomics/types"

	addresscodec "cosmossdk.io/core/address"
	sdklog "cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/server"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	jsonpb "github.com/cosmos/gogoproto/jsonpb"
	gogoproto "github.com/cosmos/gogoproto/proto"
	protov2 "google.golang.org/protobuf/proto"
)

var longHexSequence = regexp.MustCompile(`[0-9a-fA-F]{64,}`)

func readFileIfExists(t *testing.T, path string) (string, bool) {
	t.Helper()
	bz, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(bz), true
}

func grepRepo(root string, needle string, skipDir func(string) bool) ([]string, error) {
	var hits []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if skipDir != nil && skipDir(base) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
			bz, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(bz, []byte(needle)) {
				hits = append(hits, path)
			}
		}
		return nil
	})
	return hits, err
}

func TestPQCBackendApproved(t *testing.T) {
	dilithium.Default()
	name := dilithium.ActiveBackend()
	switch name {
	case "dilithium3-circl", "dilithium3-pqclean":
	default:
		t.Fatalf("unapproved PQC backend linked: %s", name)
	}
}

func TestGaslessMsgTypesResolve(t *testing.T) {
	reg := codectypes.NewInterfaceRegistry()
	app.ModuleBasics.RegisterInterfaces(reg)

	types := app.GaslessMsgTypes()
	require.NotEmpty(t, types, "GaslessMsgTypes must not be empty")

	resolver, ok := any(reg).(interface {
		Resolve(string) (gogoproto.Message, error)
	})
	require.True(t, ok, "interface registry missing Resolve implementation")

	for _, url := range types {
		msg, err := resolver.Resolve(url)
		require.NoError(t, err, "unknown MsgTypeURL: %s", url)
		if _, ok := any(msg).(sdk.Msg); !ok {
			t.Fatalf("TypeURL %s resolved to %T, not sdk.Msg", url, msg)
		}
	}
}

func TestNoDeprecatedPluralGatewayTypeURL(t *testing.T) {
	repoRoot := findRepoRoot(t)
	deprecatedTypeURL := "lumen.gateway" + "s.v1"
	hits, err := grepRepo(repoRoot, deprecatedTypeURL, func(base string) bool {
		switch base {
		case ".git", "vendor", "artifacts", "dist", "build", "proto":
			return true
		}
		return false
	})
	require.NoError(t, err)
	if len(hits) > 0 {
		t.Fatalf("deprecated TypeURL %q found in: %v", deprecatedTypeURL, hits)
	}
}

func TestRateLimitClampsConfigured(t *testing.T) {
	repoRoot := findRepoRoot(t)
	readme, ok := readFileIfExists(t, filepath.Join(repoRoot, "README.md"))
	require.True(t, ok, "README.md should exist")
	require.Contains(t, strings.ToLower(readme), "clamp", "README should mention clamps for rate limits")

	rlFile := findFile(t, repoRoot, "app", "ante_rate_limit.go")
	src, ok := readFileIfExists(t, rlFile)
	require.True(t, ok, "ante_rate_limit.go should exist at %s", rlFile)
	require.Contains(t, src, "LUMEN_RL_PER_BLOCK")
	require.Contains(t, src, "LUMEN_RL_PER_WINDOW")
	require.Contains(t, src, "LUMEN_RL_WINDOW_SEC")
	require.Contains(t, src, "LUMEN_RL_GLOBAL_MAX")
	require.Contains(t, src, "clampInt(")
}

func TestLicenseIsMIT(t *testing.T) {
	repoRoot := findRepoRoot(t)
	lic, ok := readFileIfExists(t, filepath.Join(repoRoot, "LICENSE"))
	require.True(t, ok, "LICENSE must exist")
	sc := bufio.NewScanner(strings.NewReader(lic))
	foundMIT := false
	for sc.Scan() {
		if strings.Contains(strings.ToUpper(sc.Text()), "MIT LICENSE") {
			foundMIT = true
			break
		}
	}
	require.True(t, foundMIT, "LICENSE must contain 'MIT License'")
}

func TestDocs(t *testing.T) {
	repoRoot := findRepoRoot(t)
	mustRead := func(rel string) string {
		t.Helper()
		path := filepath.Join(repoRoot, rel)
		bz, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(bz)
	}

	readme := mustRead("README.md")
	if !strings.Contains(readme, "Simulation (Docker)") {
		t.Fatalf("README: missing 'Simulation (Docker)' section")
	}
	if !strings.Contains(readme, "simulate_network.sh") && !strings.Contains(readme, "simulate-network") {
		t.Fatalf("README: missing simulator references")
	}
	if !strings.Contains(readme, "devtools/README.md") {
		t.Fatalf("README: missing devtools/README.md reference")
	}

	sec := strings.ToLower(mustRead("docs/security.md"))
	if !(strings.Contains(sec, "pqc") && strings.Contains(sec, "require")) {
		t.Fatalf("docs/security.md: missing PQC REQUIRED mention")
	}

	devtoolsReadme := mustRead("devtools/README.md")
	if !strings.Contains(devtoolsReadme, "simulate_network.sh") {
		t.Fatalf("devtools/README.md: missing simulate_network.sh mention")
	}

	if _, err := os.Stat(filepath.Join(repoRoot, "devtools", "docker", "builder", "README.md")); err == nil {
		t.Fatalf("devtools/docker/builder/README.md should not exist (content moved to devtools/README.md)")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat builder README: %v", err)
	}

	lic := strings.ToUpper(mustRead("LICENSE"))
	if !strings.Contains(lic, "MIT") {
		t.Fatalf("LICENSE: MIT string not found")
	}
}

func TestGenesisGovParamsHardened(t *testing.T) {
	repoRoot := findRepoRoot(t)
	path := filepath.Join(repoRoot, "tests", "preflight", "genesis_mainnet_template.json")

	raw, ok := readFileIfExists(t, path)
	if !ok {
		t.Skipf("genesis template %s not found; create it before mainnet launch", path)
	}

	type deposit struct {
		Denom  string `json:"denom"`
		Amount string `json:"amount"`
	}
	type govParams struct {
		Quorum        string    `json:"quorum"`
		Threshold     string    `json:"threshold"`
		VetoThreshold string    `json:"veto_threshold"`
		MinDeposit    []deposit `json:"min_deposit"`
	}
	var genesis struct {
		AppState struct {
			Gov struct {
				Params govParams `json:"params"`
			} `json:"gov"`
		} `json:"app_state"`
	}

	if err := json.Unmarshal([]byte(raw), &genesis); err != nil {
		t.Fatalf("failed to unmarshal genesis template %s: %v", path, err)
	}

	params := genesis.AppState.Gov.Params

	quorum, err := sdkmath.LegacyNewDecFromStr(params.Quorum)
	require.NoError(t, err, "invalid gov quorum")
	threshold, err := sdkmath.LegacyNewDecFromStr(params.Threshold)
	require.NoError(t, err, "invalid gov threshold")
	veto, err := sdkmath.LegacyNewDecFromStr(params.VetoThreshold)
	require.NoError(t, err, "invalid gov veto_threshold")

	minQuorum := sdkmath.LegacyMustNewDecFromStr("0.67")
	minThreshold := sdkmath.LegacyMustNewDecFromStr("0.75")
	maxVeto := sdkmath.LegacyMustNewDecFromStr("0.334")

	require.True(t, quorum.GTE(minQuorum), "quorum must be >= %s (got %s)", minQuorum, quorum)
	require.True(t, threshold.GTE(minThreshold), "threshold must be >= %s (got %s)", minThreshold, threshold)
	require.True(t, veto.LTE(maxVeto), "veto_threshold must be <= %s (got %s)", maxVeto, veto)

	require.NotEmpty(t, params.MinDeposit, "min_deposit must not be empty")
	require.Equal(t, "ulmn", strings.TrimSpace(params.MinDeposit[0].Denom), "min_deposit denom must be ulmn")
	require.NotEmpty(t, strings.TrimSpace(params.MinDeposit[0].Amount), "min_deposit amount must be set")
}

func TestFailOnNonZeroMinGasPrices(t *testing.T) {
	opts := testAppOptions{
		server.FlagMinGasPrices: "0.1ulmn",
	}
	err := app.EnsureZeroMinGasPrices(opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gasless chain")
	require.Contains(t, err.Error(), "0ulmn")

	opts = testAppOptions{
		server.FlagMinGasPrices: "0ulmn",
	}
	require.NoError(t, app.EnsureZeroMinGasPrices(opts))
}

func TestGaslessOnly(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{}, false, sdklog.NewNopLogger())
	decorator := app.NewZeroFeeDecorator()

	fee := sdk.NewCoins(sdk.NewInt64Coin(tokenomicstypes.DefaultDenom, 5))
	_, err := decorator.AnteHandle(ctx, preflightMinSendTx{fee: fee}, false, preflightNoopAnte)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
	require.Contains(t, err.Error(), "gasless tx must have zero fee")

	_, err = decorator.AnteHandle(ctx, preflightMinSendTx{fee: sdk.NewCoins()}, false, preflightNoopAnte)
	require.NoError(t, err)
}

func TestTaxAppliesOnBankSend(t *testing.T) {
	params := tokenomicstypes.DefaultParams()
	rate := tokenomicstypes.GetTxTaxRateDec(params)
	require.True(t, rate.IsPositive())

	_, _, sender := testdata.KeyTestPubAddr()
	_, _, recipient := testdata.KeyTestPubAddr()

	amount := sdkmath.NewInt(1000)
	msg := banktypes.NewMsgSend(sender, recipient, sdk.NewCoins(sdk.NewCoin(tokenomicstypes.DefaultDenom, amount)))
	tx := preflightMinSendTx{msgs: []sdk.Msg{msg}}

	perPayer, totalTax, err := app.TestOnlyComputeSendTaxes(tx, rate, bech32Codec{})
	require.NoError(t, err)
	require.NotNil(t, perPayer)

	taxCoins, ok := perPayer[recipient.String()]
	require.True(t, ok, "expected recipient tax entry")

	expectedTax := rate.MulInt(amount).TruncateInt()
	require.Equal(t, expectedTax.Int64(), taxCoins.AmountOf(tokenomicstypes.DefaultDenom).Int64())
	require.Equal(t, expectedTax.Int64(), totalTax.AmountOf(tokenomicstypes.DefaultDenom).Int64())

	net := amount.Sub(expectedTax)
	require.EqualValues(t, net.Int64(), amount.Sub(expectedTax).Int64())
}

func TestPQCNoSensitiveLogs(t *testing.T) {
	app.TestOnlySetPQCDebug(false)
	buf := &bytes.Buffer{}
	restore := app.TestOnlySwapPQCDebugWriter(buf)
	defer restore()

	app.TestOnlyEmitPQCDebugLog([]byte{0xAA}, sdk.AccAddress("signer1"), "verifying", "")
	require.Equal(t, "", buf.String(), "debug logs should be silent when disabled")

	app.TestOnlySetPQCDebug(true)
	buf.Reset()
	app.TestOnlyEmitPQCDebugLog(bytes.Repeat([]byte{0xBB}, 4), sdk.AccAddress("signer2"), "verified", "ok")

	out := strings.ToLower(buf.String())
	require.NotContains(t, out, "priv")
	require.NotContains(t, out, "seed")
	if longHexSequence.MatchString(out) {
		t.Fatalf("debug log contains long hex payload: %s", out)
	}
}

func TestMsgCapsPresent(t *testing.T) {
	repoRoot := findRepoRoot(t)
	checks := map[string][]string{
		"x/gateways/types/limits.go":   {"GatewayMetadataMaxLen", "GatewayEndpointMaxLen", "ContractMetadataMaxLen"},
		"x/dns/types/limits.go":        {"DNSLabelMaxLen", "DNSFQDNMaxLen", "DNSRecordsMax", "DNSRecordsPayloadMaxBytes"},
		"x/release/types/limits.go":    {"ReleaseVersionMaxLen", "ReleaseChannelMaxLen", "ReleaseNotesMaxLen"},
		"x/pqc/types/limits.go":        {"PQCSchemeMaxLen", "PQCPubKeyMaxLen"},
		"x/tokenomics/types/params.go": {"DefaultMinSendUlmn"},
	}
	for file, tokens := range checks {
		contents, ok := readFileIfExists(t, filepath.Join(repoRoot, file))
		require.Truef(t, ok, "%s must exist", file)
		for _, token := range tokens {
			if !strings.Contains(contents, token) {
				t.Fatalf("%s missing constant %s", file, token)
			}
		}
	}
}

func TestNoPQCExemptions(t *testing.T) {
	params := pqctypes.DefaultParams()
	var buf bytes.Buffer
	marshaler := &jsonpb.Marshaler{EmitDefaults: true}
	err := marshaler.Marshal(&buf, &params)
	require.NoError(t, err)
	bz := buf.Bytes()
	key := strings.Join([]string{"exempt", "msg", "types"}, "_")
	if strings.Contains(buf.String(), key) {
		t.Fatalf("pqc params JSON unexpectedly contains %s: %s", key, string(bz))
	}
}

func TestMinSendDustPreflight(t *testing.T) {
	ctx, decorator, addrs := setupPreflightMinSend(t)

	small := banktypes.NewMsgSend(addrs.userA, addrs.userB, sdk.NewCoins(sdk.NewInt64Coin(tokenomicstypes.DefaultDenom, int64(tokenomicstypes.DefaultMinSendUlmn-1))))
	_, err := decorator.AnteHandle(ctx, preflightMinSendTx{msgs: []sdk.Msg{small}}, false, preflightNoopAnte)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)

	exact := banktypes.NewMsgSend(addrs.userA, addrs.userB, sdk.NewCoins(sdk.NewInt64Coin(tokenomicstypes.DefaultDenom, int64(tokenomicstypes.DefaultMinSendUlmn))))
	_, err = decorator.AnteHandle(ctx, preflightMinSendTx{msgs: []sdk.Msg{exact}}, false, preflightNoopAnte)
	require.NoError(t, err)
}

func TestNoLegacyAminoInBusinessLogic(t *testing.T) {
	repoRoot := findRepoRoot(t)
	banned := [][]byte{
		[]byte("LegacyAmino"),
		[]byte("LegacyMustNewDecFromStr"),
	}

	walkGoFiles(t, repoRoot, func(rel string, data []byte) {
		if strings.HasSuffix(rel, "_test.go") {
			return
		}
		for _, needle := range banned {
			if !bytes.Contains(data, needle) {
				continue
			}
			if strings.Contains(rel, "app/ante_") ||
				strings.Contains(rel, "app/post_") ||
				(strings.HasPrefix(rel, "x/") && strings.Contains(rel, "/keeper/")) {
				t.Fatalf("legacy codec usage found in forbidden path %s", rel)
			}
		}
	})
}

func TestNoHiddenMinGasPrices(t *testing.T) {
	repoRoot := findRepoRoot(t)
	allowed := map[string]struct{}{
		"app/min_gas_guard.go":                 {},
		"app/ante_ignore_min_gas.go":           {},
		"app/app.go":                           {},
		"cmd/lumend/cmd/testnet_multi_node.go": {},
	}
	keywords := [][]byte{
		[]byte("mingasprices"),
		[]byte("minimum-gas-prices"),
		[]byte("mingasprice"),
	}

	walkGoFiles(t, repoRoot, func(rel string, data []byte) {
		if strings.HasSuffix(rel, "_test.go") {
			return
		}

		lower := bytes.ToLower(data)
		for _, needle := range keywords {
			if !bytes.Contains(lower, needle) {
				continue
			}
			if _, ok := allowed[rel]; ok {
				continue
			}
			t.Fatalf("unexpected minimum-gas-prices reference found in %s", rel)
		}
	})
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisfile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisfile)
	return filepath.Clean(filepath.Join(dir, "../.."))
}

func findFile(t *testing.T, root string, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{root}, parts...)...)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	t.Fatalf("file not found: %s", path)
	return ""
}

type testAppOptions map[string]interface{}

func (o testAppOptions) Get(key string) interface{} { return o[key] }

type bech32Codec struct{}

func (bech32Codec) StringToBytes(text string) ([]byte, error) {
	return sdk.AccAddressFromBech32(text)
}

func (bech32Codec) BytesToString(bz []byte) (string, error) {
	return sdk.AccAddress(bz).String(), nil
}

func walkGoFiles(t *testing.T, root string, fn func(rel string, data []byte)) {
	t.Helper()
	skip := map[string]bool{
		".git":      true,
		"artifacts": true,
		"dist":      true,
		"build":     true,
		"vendor":    true,
		"private":   true,
		"devtools":  true,
		"docs":      true,
		"proto":     true,
	}

	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		bz, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fn(filepath.ToSlash(rel), bz)
		return nil
	}); err != nil {
		t.Fatalf("walk go files: %v", err)
	}
}

// --- min-send smoke helpers ---

type preflightMinSendAccounts struct {
	userA sdk.AccAddress
	userB sdk.AccAddress
}

func setupPreflightMinSend(t *testing.T) (sdk.Context, app.MinSendDecorator, preflightMinSendAccounts) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey("preflight_min_send")
	memKey := storetypes.NewTransientStoreKey("preflight_min_send_transient")
	ctx := sdktestutil.DefaultContextWithDB(t, storeKey, memKey).Ctx.WithLogger(sdklog.NewNopLogger())

	codec := authcodec.NewBech32Codec(app.AccountAddressPrefix)
	ak := newPreflightAccountKeeper(codec)

	userA := sdk.AccAddress(bytes.Repeat([]byte{0x01}, 20))
	userB := sdk.AccAddress(bytes.Repeat([]byte{0x02}, 20))
	ak.addAccount(authtypes.NewBaseAccountWithAddress(userA))
	ak.addAccount(authtypes.NewBaseAccountWithAddress(userB))
	ak.addAccount(authtypes.NewEmptyModuleAccount(authtypes.FeeCollectorName))

	params := tokenomicstypes.Params{
		TxTaxRate:                  tokenomicstypes.DefaultTxTaxRate,
		InitialRewardPerBlockLumn:  tokenomicstypes.DefaultInitialRewardPerBlockLumn,
		HalvingIntervalBlocks:      tokenomicstypes.DefaultHalvingIntervalBlocks,
		SupplyCapLumn:              tokenomicstypes.DefaultSupplyCapLumn,
		Decimals:                   tokenomicstypes.DefaultDecimals,
		MinSendUlmn:                tokenomicstypes.DefaultMinSendUlmn,
		Denom:                      tokenomicstypes.DefaultDenom,
		DistributionIntervalBlocks: tokenomicstypes.DefaultDistributionIntervalBlocks,
	}

	decorator := app.NewMinSendDecorator(ak, preflightTokenomicsKeeper{params: params})

	return ctx, decorator, preflightMinSendAccounts{
		userA: userA,
		userB: userB,
	}
}

type preflightAccountKeeper struct {
	codec    addresscodec.Codec
	accounts map[string]sdk.AccountI
}

func newPreflightAccountKeeper(codec addresscodec.Codec) *preflightAccountKeeper {
	return &preflightAccountKeeper{
		codec:    codec,
		accounts: make(map[string]sdk.AccountI),
	}
}

func (m *preflightAccountKeeper) AddressCodec() addresscodec.Codec {
	return m.codec
}

func (m *preflightAccountKeeper) GetAccount(_ context.Context, addr sdk.AccAddress) sdk.AccountI {
	return m.accounts[addr.String()]
}

func (m *preflightAccountKeeper) addAccount(acc sdk.AccountI) {
	m.accounts[acc.GetAddress().String()] = acc
}

type preflightTokenomicsKeeper struct {
	params tokenomicstypes.Params
}

func (p preflightTokenomicsKeeper) GetParams(context.Context) tokenomicstypes.Params {
	return p.params
}

var preflightNoopAnte sdk.AnteHandler = func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
	return ctx, nil
}

type preflightMinSendTx struct {
	msgs []sdk.Msg
	fee  sdk.Coins
}

var _ sdk.Tx = preflightMinSendTx{}
var _ authsigning.SigVerifiableTx = preflightMinSendTx{}

func (m preflightMinSendTx) GetMsgs() []sdk.Msg                                 { return m.msgs }
func (preflightMinSendTx) GetMsgsV2() ([]protov2.Message, error)                { return nil, nil }
func (preflightMinSendTx) ValidateBasic() error                                 { return nil }
func (preflightMinSendTx) GetMemo() string                                      { return "" }
func (preflightMinSendTx) GetSignaturesV2() ([]signingtypes.SignatureV2, error) { return nil, nil }
func (preflightMinSendTx) GetPubKeys() ([]cryptotypes.PubKey, error)            { return nil, nil }
func (preflightMinSendTx) GetGas() uint64                                       { return 0 }
func (m preflightMinSendTx) GetFee() sdk.Coins {
	if m.fee == nil {
		return sdk.NewCoins()
	}
	return m.fee
}
func (preflightMinSendTx) FeePayer() []byte               { return nil }
func (preflightMinSendTx) FeeGranter() []byte             { return nil }
func (preflightMinSendTx) GetTimeoutHeight() uint64       { return 0 }
func (preflightMinSendTx) GetTimeoutTimeStamp() time.Time { return time.Time{} }
func (preflightMinSendTx) GetUnordered() bool             { return false }
func (preflightMinSendTx) GetSigners() ([][]byte, error)  { return nil, nil }
