package txext

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	txv1beta1 "cosmossdk.io/api/cosmos/tx/v1beta1"
	txsigning "cosmossdk.io/x/tx/signing"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/testutil"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"lumen/crypto/pqc/dilithium"
	pqckeys "lumen/x/pqc/client/keys"
	pqcmodule "lumen/x/pqc/module"
	pqcsigning "lumen/x/pqc/signing"
	pqctypes "lumen/x/pqc/types"
)

func TestInjectPQCPostSignAddsExtension(t *testing.T) {
	encCfg := makeEncodingConfig(t)

	addr := authtypes.NewModuleAddress("signer")
	home := t.TempDir()

	store, err := pqckeys.LoadStore(home)
	require.NoError(t, err)

	seed := bytesOf(32, 0x01)
	pub, priv, err := dilithium.Default().GenerateKey(seed)
	require.NoError(t, err)

	require.NoError(t, store.SaveKey(pqckeys.KeyRecord{
		Name:       "local",
		Scheme:     dilithium.Default().Name(),
		PublicKey:  pub,
		PrivateKey: priv,
		CreatedAt:  time.Now().UTC(),
	}))
	require.NoError(t, store.LinkAddress(addr.String(), "local"))

	clientCtx := client.Context{}.
		WithCodec(encCfg.Codec).
		WithInterfaceRegistry(encCfg.InterfaceRegistry).
		WithTxConfig(encCfg.TxConfig).
		WithAccountRetriever(client.MockAccountRetriever{ReturnAccNum: 7, ReturnAccSeq: 3}).
		WithHomeDir(home).
		WithChainID("chain-test")
	clientCtx.FromAddress = addr
	clientCtx.Viper = viper.New()
	clientCtx.Viper.Set(FlagEnable, true)
	clientCtx.Viper.Set(FlagScheme, dilithium.Default().Name())

	builder := encCfg.TxConfig.NewTxBuilder()
	msg := pqctypes.NewMsgLinkAccountPQC(addr, dilithium.Default().Name(), pub)
	msg.PowNonce = []byte{0x01}
	require.NoError(t, builder.SetMsgs(msg))

	secp := secp256k1.GenPrivKey()
	require.NoError(t, builder.SetSignatures(signingtypes.SignatureV2{
		PubKey: secp.PubKey(),
		Data: &signingtypes.SingleSignatureData{
			SignMode:  signingtypes.SignMode_SIGN_MODE_DIRECT,
			Signature: []byte("stub"),
		},
	}))

	injected, err := InjectPQCPostSign(clientCtx, builder)
	require.NoError(t, err)
	require.True(t, injected)

	exts := getNonCriticalExtensions(t, encCfg, builder)
	require.Len(t, exts, 1)

	ext := exts[0]
	require.Equal(t, pqctypes.PQCSignaturesTypeURL, ext.TypeUrl)

	var payload pqctypes.PQCSignatures
	require.NoError(t, encCfg.Codec.Unmarshal(ext.Value, &payload))
	require.Len(t, payload.Signatures, 1)
	require.Equal(t, addr.String(), payload.Signatures[0].Addr)
	require.Equal(t, dilithium.Default().Name(), payload.Signatures[0].Scheme)
	require.Equal(t, []byte(pub), payload.Signatures[0].PubKey)

	txBytes, err := encCfg.TxConfig.TxEncoder()(builder.GetTx())
	require.NoError(t, err)
	txRaw := new(txv1beta1.TxRaw)
	require.NoError(t, proto.Unmarshal(txBytes, txRaw))

	signBytes, err := pqcsigning.ComputeSignBytes("chain-test", 7, txRaw)
	require.NoError(t, err)

	require.True(t, dilithium.Default().Verify(pub, signBytes, payload.Signatures[0].Signature))
}

func TestInjectPQCPostSignDisabled(t *testing.T) {
	encCfg := makeEncodingConfig(t)
	builder := encCfg.TxConfig.NewTxBuilder()
	addr := authtypes.NewModuleAddress("skip")

	clientCtx := client.Context{}.
		WithCodec(encCfg.Codec).
		WithInterfaceRegistry(encCfg.InterfaceRegistry).
		WithTxConfig(encCfg.TxConfig).
		WithAccountRetriever(client.MockAccountRetriever{ReturnAccNum: 1, ReturnAccSeq: 0})
	clientCtx.FromAddress = addr
	clientCtx.Viper = viper.New()
	clientCtx.Viper.Set(FlagEnable, false)

	injected, err := InjectPQCPostSign(clientCtx, builder)
	require.NoError(t, err)
	require.False(t, injected)
	require.Len(t, getNonCriticalExtensions(t, encCfg, builder), 0)
}

func TestInjectPQCPostSignOptionalMissingKey(t *testing.T) {
	encCfg := makeEncodingConfig(t)
	addr := authtypes.NewModuleAddress("optional")
	home := t.TempDir()

	_, err := pqckeys.LoadStore(home)
	require.NoError(t, err)

	conn := startMockGRPCServer(t, pqctypes.Params{
		Policy:    pqctypes.PqcPolicy_PQC_POLICY_OPTIONAL,
		MinScheme: dilithium.Default().Name(),
	})
	defer conn.Close()

	clientCtx := client.Context{}.
		WithCodec(encCfg.Codec).
		WithInterfaceRegistry(encCfg.InterfaceRegistry).
		WithTxConfig(encCfg.TxConfig).
		WithAccountRetriever(client.MockAccountRetriever{ReturnAccNum: 5, ReturnAccSeq: 0}).
		WithHomeDir(home).
		WithGRPCClient(conn).
		WithChainID("chain-test")
	clientCtx.FromAddress = addr
	clientCtx.Viper = viper.New()
	clientCtx.Viper.Set(FlagEnable, true)

	builder := encCfg.TxConfig.NewTxBuilder()
	secp := secp256k1.GenPrivKey()
	require.NoError(t, builder.SetSignatures(signingtypes.SignatureV2{
		PubKey: secp.PubKey(),
		Data: &signingtypes.SingleSignatureData{
			SignMode:  signingtypes.SignMode_SIGN_MODE_DIRECT,
			Signature: []byte("stub"),
		},
	}))

	injected, err := InjectPQCPostSign(clientCtx, builder)
	require.NoError(t, err)
	require.False(t, injected)
	require.Len(t, getNonCriticalExtensions(t, encCfg, builder), 0)
}

func makeEncodingConfig(t *testing.T) moduletestutil.TestEncodingConfig {
	t.Helper()

	codecOptions := testutil.CodecOptions{
		AccAddressPrefix: sdk.GetConfig().GetBech32AccountAddrPrefix(),
		ValAddressPrefix: sdk.GetConfig().GetBech32ValidatorAddrPrefix(),
	}
	interfaceRegistry := codecOptions.NewInterfaceRegistry()
	protoCodec := codec.NewProtoCodec(interfaceRegistry)

	signOpts := txsigning.Options{
		AddressCodec:          authcodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		ValidatorAddressCodec: authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ValidatorAddrPrefix()),
		FileResolver:          interfaceRegistry,
	}
	txConfig, err := authtx.NewTxConfigWithOptions(protoCodec, authtx.ConfigOptions{
		SigningOptions: &signOpts,
	})
	require.NoError(t, err)

	encCfg := moduletestutil.TestEncodingConfig{
		InterfaceRegistry: interfaceRegistry,
		Codec:             protoCodec,
		TxConfig:          txConfig,
		Amino:             codec.NewLegacyAmino(),
	}

	moduleBasics := module.NewBasicManager(pqcmodule.AppModule{})
	std.RegisterLegacyAminoCodec(encCfg.Amino)
	std.RegisterInterfaces(encCfg.InterfaceRegistry)
	moduleBasics.RegisterLegacyAminoCodec(encCfg.Amino)
	moduleBasics.RegisterInterfaces(encCfg.InterfaceRegistry)

	return encCfg
}

func bytesOf(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func getNonCriticalExtensions(t *testing.T, encCfg moduletestutil.TestEncodingConfig, builder client.TxBuilder) []*anypb.Any {
	t.Helper()

	txBytes, err := encCfg.TxConfig.TxEncoder()(builder.GetTx())
	require.NoError(t, err)

	txRaw := new(txv1beta1.TxRaw)
	require.NoError(t, proto.Unmarshal(txBytes, txRaw))

	body := new(txv1beta1.TxBody)
	require.NoError(t, proto.Unmarshal(txRaw.BodyBytes, body))
	return body.NonCriticalExtensionOptions
}

type mockQueryServer struct {
	pqctypes.UnimplementedQueryServer
	params pqctypes.Params
}

func (s *mockQueryServer) Params(ctx context.Context, _ *pqctypes.QueryParamsRequest) (*pqctypes.QueryParamsResponse, error) {
	return &pqctypes.QueryParamsResponse{Params: s.params}, nil
}

func (s *mockQueryServer) AccountPQC(ctx context.Context, _ *pqctypes.QueryAccountPQCRequest) (*pqctypes.QueryAccountPQCResponse, error) {
	return &pqctypes.QueryAccountPQCResponse{Account: pqctypes.AccountPQC{}}, nil
}

func startMockGRPCServer(t *testing.T, params pqctypes.Params) *grpc.ClientConn {
	t.Helper()

	const bufSize = 1 << 20
	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer()
	pqctypes.RegisterQueryServer(server, &mockQueryServer{params: params})

	go func() {
		if err := server.Serve(lis); err != nil {
			panic(err)
		}
	}()

	conn, err := grpc.NewClient(
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		server.Stop()
	})

	return conn
}
