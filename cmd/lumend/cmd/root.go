package cmd

import (
	"fmt"
	"os"
	"strconv"

	"cosmossdk.io/client/v2/autocli"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/config"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/server"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtxconfig "github.com/cosmos/cosmos-sdk/x/auth/tx/config"
	"github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distcli "github.com/cosmos/cosmos-sdk/x/distribution/client/cli"
	disttypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"

	"lumen/app"
	pqctxext "lumen/x/pqc/client/txext"
)

func NewRootCmd() *cobra.Command {
	var (
		autoCliOpts        autocli.AppOptions
		moduleBasicManager module.BasicManager
		clientCtx          client.Context
	)

	if err := depinject.Inject(
		depinject.Configs(app.AppConfig(),
			depinject.Supply(log.NewNopLogger()),
			depinject.Provide(
				ProvideClientContext,
			),
		),
		&autoCliOpts,
		&moduleBasicManager,
		&clientCtx,
	); err != nil {
		panic(err)
	}

	rootCmd := &cobra.Command{
		Use:           app.Name + "d",
		Short:         "lumen node",
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SetOut(cmd.OutOrStdout())
			cmd.SetErr(cmd.ErrOrStderr())

			clientCtx = clientCtx.WithCmdContext(cmd.Context()).WithViper(app.Name)
			clientCtx, err := client.ReadPersistentCommandFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			clientCtx, err = config.ReadFromClientConfig(clientCtx)
			if err != nil {
				return err
			}

			if err := client.SetCmdClientContextHandler(clientCtx, cmd); err != nil {
				return err
			}

			customAppTemplate, customAppConfig := initAppConfig()
			customCMTConfig := initCometBFTConfig()

			return server.InterceptConfigsPreRunHandler(cmd, customAppTemplate, customAppConfig, customCMTConfig)
		},
	}

	initRootCmd(rootCmd, clientCtx.TxConfig, moduleBasicManager)

	if err := autoCliOpts.EnhanceRootCommand(rootCmd); err != nil {
		panic(err)
	}
	patchBankSendCommand(rootCmd)
	patchGovTxCommand(rootCmd)
	patchDistributionWithdrawCommand(rootCmd)
	patchStakingTxCommands(rootCmd)

	return rootCmd
}

func ProvideClientContext(
	appCodec codec.Codec,
	interfaceRegistry codectypes.InterfaceRegistry,
	txConfigOpts tx.ConfigOptions,
	legacyAmino *codec.LegacyAmino,
) client.Context {
	clientCtx := client.Context{}.
		WithCodec(appCodec).
		WithInterfaceRegistry(interfaceRegistry).
		WithLegacyAmino(legacyAmino).
		WithInput(os.Stdin).
		WithAccountRetriever(types.AccountRetriever{}).
		WithHomeDir(app.DefaultNodeHome).
		WithViper(app.Name) // env variable prefix

	clientCtx, _ = config.ReadFromClientConfig(clientCtx)

	txConfigOpts.TextualCoinMetadataQueryFn = authtxconfig.NewGRPCCoinMetadataQueryFn(clientCtx)
	txConfig, err := tx.NewTxConfigWithOptions(clientCtx.Codec, txConfigOpts)
	if err != nil {
		panic(err)
	}
	clientCtx = clientCtx.WithTxConfig(txConfig)

	return clientCtx
}

func patchBankSendCommand(root *cobra.Command) {
	path := []string{"tx", "bank", "send"}
	sendCmd, _, err := root.Find(path)
	if err != nil || sendCmd == nil {
		return
	}

	sendCmd.Args = cobra.ExactArgs(3)
	sendCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := cmd.Flags().Set(flags.FlagFrom, args[0]); err != nil {
			return err
		}

		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		toAddrBz, err := signingCtx.AddressCodec().StringToBytes(args[1])
		if err != nil {
			return err
		}

		coins, err := sdk.ParseCoinsNormalized(args[2])
		if err != nil {
			return err
		}
		if coins.IsZero() {
			return fmt.Errorf("amount must be positive")
		}

		msg := banktypes.NewMsgSend(clientCtx.GetFromAddress(), sdk.AccAddress(toAddrBz), coins)
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

func patchDistributionWithdrawCommand(root *cobra.Command) {
	path := []string{"tx", "distribution", "withdraw-rewards"}
	withdrawCmd, _, err := root.Find(path)
	if err != nil || withdrawCmd == nil {
		return
	}

	withdrawCmd.Args = cobra.ExactArgs(1)
	withdrawCmd.RunE = func(cmd *cobra.Command, args []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}
		if _, err := signingCtx.ValidatorAddressCodec().StringToBytes(args[0]); err != nil {
			return err
		}

		delAddr := clientCtx.GetFromAddress()
		if delAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		msgs := []sdk.Msg{
			disttypes.NewMsgWithdrawDelegatorReward(delAddr.String(), args[0]),
		}
		if commission, _ := cmd.Flags().GetBool(distcli.FlagCommission); commission {
			msgs = append(msgs, disttypes.NewMsgWithdrawValidatorCommission(args[0]))
		}

		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msgs...)
	}
}

func patchStakingTxCommands(root *cobra.Command) {
	path := []string{"tx", "staking"}
	stakingCmd, _, err := root.Find(path)
	if err != nil || stakingCmd == nil {
		return
	}

	patchStakingDelegateCommand(stakingCmd)
	patchStakingRedelegateCommand(stakingCmd)
	patchStakingUnbondCommand(stakingCmd)
	patchStakingCancelUnbondCommand(stakingCmd)
}

func patchStakingDelegateCommand(stakingCmd *cobra.Command) {
	cmd, _, err := stakingCmd.Find([]string{"delegate"})
	if err != nil || cmd == nil {
		return
	}

	cmd.Args = cobra.ExactArgs(2)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		if _, err := signingCtx.ValidatorAddressCodec().StringToBytes(args[0]); err != nil {
			return err
		}

		amount, err := sdk.ParseCoinNormalized(args[1])
		if err != nil {
			return err
		}

		delAddr := clientCtx.GetFromAddress()
		if delAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		msg := stakingtypes.NewMsgDelegate(delAddr.String(), args[0], amount)
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

func patchStakingRedelegateCommand(stakingCmd *cobra.Command) {
	cmd, _, err := stakingCmd.Find([]string{"redelegate"})
	if err != nil || cmd == nil {
		return
	}

	cmd.Args = cobra.ExactArgs(3)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		if _, err := signingCtx.ValidatorAddressCodec().StringToBytes(args[0]); err != nil {
			return err
		}
		if _, err := signingCtx.ValidatorAddressCodec().StringToBytes(args[1]); err != nil {
			return err
		}

		amount, err := sdk.ParseCoinNormalized(args[2])
		if err != nil {
			return err
		}

		delAddr := clientCtx.GetFromAddress()
		if delAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		msg := stakingtypes.NewMsgBeginRedelegate(delAddr.String(), args[0], args[1], amount)
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

func patchStakingUnbondCommand(stakingCmd *cobra.Command) {
	cmd, _, err := stakingCmd.Find([]string{"unbond"})
	if err != nil || cmd == nil {
		return
	}

	cmd.Args = cobra.ExactArgs(2)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		if _, err := signingCtx.ValidatorAddressCodec().StringToBytes(args[0]); err != nil {
			return err
		}

		amount, err := sdk.ParseCoinNormalized(args[1])
		if err != nil {
			return err
		}

		delAddr := clientCtx.GetFromAddress()
		if delAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		msg := stakingtypes.NewMsgUndelegate(delAddr.String(), args[0], amount)
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

func patchStakingCancelUnbondCommand(stakingCmd *cobra.Command) {
	cmd, _, err := stakingCmd.Find([]string{"cancel-unbond"})
	if err != nil || cmd == nil {
		return
	}

	cmd.Args = cobra.ExactArgs(3)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		if _, err := signingCtx.ValidatorAddressCodec().StringToBytes(args[0]); err != nil {
			return err
		}

		amount, err := sdk.ParseCoinNormalized(args[1])
		if err != nil {
			return err
		}

		creationHeight, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid height: %s", args[2])
		}

		delAddr := clientCtx.GetFromAddress()
		if delAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		msg := stakingtypes.NewMsgCancelUnbondingDelegation(delAddr.String(), args[0], creationHeight, amount)
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}
