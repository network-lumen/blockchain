package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"cosmossdk.io/client/v2/autocli"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	math "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/config"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/server"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtxconfig "github.com/cosmos/cosmos-sdk/x/auth/tx/config"
	"github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distcli "github.com/cosmos/cosmos-sdk/x/distribution/client/cli"
	disttypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

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
	patchBankMultiSendCommand(rootCmd)
	patchGovTxCommand(rootCmd)
	patchDistributionTxCommands(rootCmd)
	patchStakingTxCommands(rootCmd)
	patchSlashingTxCommands(rootCmd)

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

	if !strings.Contains(sendCmd.Short, "[PQC]") {
		sendCmd.Short = strings.TrimSpace(sendCmd.Short + " [PQC]")
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

func patchBankMultiSendCommand(root *cobra.Command) {
	path := []string{"tx", "bank", "multi-send"}
	cmd, _, err := root.Find(path)
	if err != nil || cmd == nil {
		return
	}

	if !strings.Contains(cmd.Short, "[PQC]") {
		cmd.Short = strings.TrimSpace(cmd.Short + " [PQC]")
	}

	cmd.Args = cobra.MinimumNArgs(4)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// args: [from_key_or_address] [to1 ... toN] [amount]
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

		amountStr := args[len(args)-1]
		coins, err := sdk.ParseCoinsNormalized(amountStr)
		if err != nil {
			return err
		}
		if coins.IsZero() {
			return fmt.Errorf("must send positive amount")
		}

		split, err := cmd.Flags().GetBool("split")
		if err != nil {
			return err
		}

		totalAddrs := math.NewInt(int64(len(args) - 2))
		sendCoins := coins
		if split {
			sendCoins = coins.QuoInt(totalAddrs)
		}

		var outputs []banktypes.Output
		for _, arg := range args[1 : len(args)-1] {
			toAddrBz, err := signingCtx.AddressCodec().StringToBytes(arg)
			if err != nil {
				return err
			}
			outputs = append(outputs, banktypes.NewOutput(sdk.AccAddress(toAddrBz), sendCoins))
		}

		var totalAmount sdk.Coins
		if split {
			totalAmount = sendCoins.MulInt(totalAddrs)
		} else {
			totalAmount = coins.MulInt(totalAddrs)
		}

		input := banktypes.NewInput(clientCtx.GetFromAddress(), totalAmount)
		msg := banktypes.NewMsgMultiSend(input, outputs)

		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

func patchDistributionTxCommands(root *cobra.Command) {
	path := []string{"tx", "distribution"}
	distCmd, _, err := root.Find(path)
	if err != nil || distCmd == nil {
		return
	}

	patchDistributionWithdrawCommand(distCmd)
	patchDistributionWithdrawAllRewardsCommand(distCmd)
	patchDistributionFundCommunityPoolCommand(distCmd)
	patchDistributionSetWithdrawAddrCommand(distCmd)
	patchDistributionWithdrawValidatorCommissionCommand(distCmd)
}

func patchDistributionWithdrawCommand(distCmd *cobra.Command) {
	withdrawCmd, _, err := distCmd.Find([]string{"withdraw-rewards"})
	if err != nil || withdrawCmd == nil {
		return
	}

	if !strings.Contains(withdrawCmd.Short, "[PQC]") {
		withdrawCmd.Short = strings.TrimSpace(withdrawCmd.Short + " [PQC]")
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

func patchDistributionWithdrawAllRewardsCommand(distCmd *cobra.Command) {
	cmd, _, err := distCmd.Find([]string{"withdraw-all-rewards"})
	if err != nil || cmd == nil {
		return
	}

	if !strings.Contains(cmd.Short, "[PQC]") {
		cmd.Short = strings.TrimSpace(cmd.Short + " [PQC]")
	}

	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		delAddr := clientCtx.GetFromAddress()
		if delAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		delAddrStr, err := signingCtx.AddressCodec().BytesToString(delAddr)
		if err != nil {
			return err
		}

		if clientCtx.Offline {
			return fmt.Errorf("cannot generate tx in offline mode")
		}

		queryClient := disttypes.NewQueryClient(clientCtx)
		delValsRes, err := queryClient.DelegatorValidators(
			cmd.Context(),
			&disttypes.QueryDelegatorValidatorsRequest{DelegatorAddress: delAddrStr},
		)
		if err != nil {
			return err
		}

		validators := delValsRes.Validators
		msgs := make([]sdk.Msg, 0, len(validators))
		for _, valAddr := range validators {
			if _, err := signingCtx.ValidatorAddressCodec().StringToBytes(valAddr); err != nil {
				return err
			}

			msgs = append(msgs, disttypes.NewMsgWithdrawDelegatorReward(delAddrStr, valAddr))
		}

		chunkSize, _ := cmd.Flags().GetInt(distcli.FlagMaxMessagesPerTx)

		return splitAndApplyTx(pqctxext.GenerateOrBroadcastTxCLI, cmd, clientCtx, cmd.Flags(), msgs, chunkSize)
	}
}

func patchDistributionFundCommunityPoolCommand(distCmd *cobra.Command) {
	cmd, _, err := distCmd.Find([]string{"fund-community-pool"})
	if err != nil || cmd == nil {
		return
	}

	if !strings.Contains(cmd.Short, "[PQC]") {
		cmd.Short = strings.TrimSpace(cmd.Short + " [PQC]")
	}

	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		fromAddr := clientCtx.GetFromAddress()
		if fromAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		depositorAddr, err := signingCtx.AddressCodec().BytesToString(fromAddr)
		if err != nil {
			return err
		}

		amount, err := sdk.ParseCoinsNormalized(args[0])
		if err != nil {
			return err
		}

		msg := disttypes.NewMsgFundCommunityPool(amount, depositorAddr)
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

func patchDistributionSetWithdrawAddrCommand(distCmd *cobra.Command) {
	cmd, _, err := distCmd.Find([]string{"set-withdraw-addr"})
	if err != nil || cmd == nil {
		return
	}

	if !strings.Contains(cmd.Short, "[PQC]") {
		cmd.Short = strings.TrimSpace(cmd.Short + " [PQC]")
	}

	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		delAddr := clientCtx.GetFromAddress()
		if delAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		withdrawBz, err := signingCtx.AddressCodec().StringToBytes(args[0])
		if err != nil {
			return err
		}

		msg := disttypes.NewMsgSetWithdrawAddress(delAddr, sdk.AccAddress(withdrawBz))
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

func patchDistributionWithdrawValidatorCommissionCommand(distCmd *cobra.Command) {
	cmd, _, err := distCmd.Find([]string{"withdraw-validator-commission"})
	if err != nil || cmd == nil {
		return
	}

	if !strings.Contains(cmd.Short, "[PQC]") {
		cmd.Short = strings.TrimSpace(cmd.Short + " [PQC]")
	}

	cmd.Args = cobra.ExactArgs(1)
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

		msg := disttypes.NewMsgWithdrawValidatorCommission(args[0])
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

func patchStakingTxCommands(root *cobra.Command) {
	path := []string{"tx", "staking"}
	stakingCmd, _, err := root.Find(path)
	if err != nil || stakingCmd == nil {
		return
	}

	patchStakingEditValidatorCommand(stakingCmd)
	patchStakingCreateValidatorCommand(stakingCmd)
	patchStakingDelegateCommand(stakingCmd)
	patchStakingRedelegateCommand(stakingCmd)
	patchStakingUnbondCommand(stakingCmd)
	patchStakingCancelUnbondCommand(stakingCmd)
}

func patchStakingEditValidatorCommand(stakingCmd *cobra.Command) {
	cmd, _, err := stakingCmd.Find([]string{"edit-validator"})
	if err != nil || cmd == nil {
		return
	}

	if !strings.Contains(cmd.Short, "[PQC]") {
		if strings.TrimSpace(cmd.Short) == "" {
			cmd.Short = "Edit validator [PQC]"
		} else {
			cmd.Short = cmd.Short + " [PQC]"
		}
	}

	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		fromAddr := clientCtx.GetFromAddress()
		if fromAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		moniker, _ := cmd.Flags().GetString("moniker")
		identity, _ := cmd.Flags().GetString("identity")
		website, _ := cmd.Flags().GetString("website")
		security, _ := cmd.Flags().GetString("security-contact")
		details, _ := cmd.Flags().GetString("details")
		description := stakingtypes.NewDescription(moniker, identity, website, security, details)

		var newRate *math.LegacyDec
		if commissionRate, _ := cmd.Flags().GetString("commission-rate"); commissionRate != "" {
			rate, err := math.LegacyNewDecFromStr(commissionRate)
			if err != nil {
				return fmt.Errorf("invalid new commission rate: %w", err)
			}
			newRate = &rate
		}

		var newMinSelfDelegation *math.Int
		if minSelfDelegationStr, _ := cmd.Flags().GetString("min-self-delegation"); minSelfDelegationStr != "" {
			msd, ok := math.NewIntFromString(minSelfDelegationStr)
			if !ok {
				return fmt.Errorf("minimum self delegation must be a positive integer")
			}
			newMinSelfDelegation = &msd
		}

		valStr, err := signingCtx.ValidatorAddressCodec().BytesToString(sdk.ValAddress(fromAddr))
		if err != nil {
			return err
		}

		msg := stakingtypes.NewMsgEditValidator(valStr, description, newRate, newMinSelfDelegation)
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

func patchStakingCreateValidatorCommand(stakingCmd *cobra.Command) {
	if os.Getenv("LUMEN_PQC_DEBUG") == "1" {
		fmt.Fprintln(os.Stderr, "[pqc-cli] patchStakingCreateValidatorCommand: scanning staking subcommands")
		for _, c := range stakingCmd.Commands() {
			fmt.Fprintf(os.Stderr, "[pqc-cli] staking subcmd: name=%q use=%q short=%q\n", c.Name(), c.Use, c.Short)
		}
	}

	var targets []*cobra.Command
	for _, c := range stakingCmd.Commands() {
		if c.Name() == "create-validator" {
			targets = append(targets, c)
		}
	}
	if len(targets) == 0 {
		if os.Getenv("LUMEN_PQC_DEBUG") == "1" {
			fmt.Fprintln(os.Stderr, "[pqc-cli] patchStakingCreateValidatorCommand: no create-validator commands found")
		}
		return
	}

	for _, cmd := range targets {
		if os.Getenv("LUMEN_PQC_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "[pqc-cli] patchStakingCreateValidatorCommand: patching cmd name=%q use=%q ptr=%p\n", cmd.Name(), cmd.Use, cmd)
		}

		if !strings.Contains(cmd.Short, "[PQC]") {
			if strings.TrimSpace(cmd.Short) == "" {
				cmd.Short = "Create new validator [PQC]"
			} else {
				cmd.Short = cmd.Short + " [PQC]"
			}
		}

		cmd.Args = cobra.ExactArgs(1)
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if os.Getenv("LUMEN_PQC_DEBUG") == "1" {
				fmt.Fprintln(os.Stderr, "[pqc-cli] staking create-validator patched RunE")
			}

			err := func() error {
				clientCtx, err := client.GetClientTxContext(cmd)
				if err != nil {
					return err
				}

				signingCtx := clientCtx.TxConfig.SigningContext()
				if signingCtx == nil {
					return fmt.Errorf("pqc: signing context unavailable")
				}

				fromAddr := clientCtx.GetFromAddress()
				if fromAddr == nil {
					return fmt.Errorf("pqc: missing from address")
				}

				filePath := strings.TrimSpace(args[0])
				if filePath == "" {
					return fmt.Errorf("path/to/validator.json is required")
				}

				contents, err := os.ReadFile(filePath)
				if err != nil {
					return err
				}

				type validatorJSON struct {
					Amount              string          `json:"amount"`
					PubKey              json.RawMessage `json:"pubkey"`
					Moniker             string          `json:"moniker"`
					Identity            string          `json:"identity,omitempty"`
					Website             string          `json:"website,omitempty"`
					Security            string          `json:"security,omitempty"`
					Details             string          `json:"details,omitempty"`
					CommissionRate      string          `json:"commission-rate"`
					CommissionMaxRate   string          `json:"commission-max-rate"`
					CommissionMaxChange string          `json:"commission-max-change-rate"`
					MinSelfDelegation   string          `json:"min-self-delegation"`
				}

				var v validatorJSON
				if err := json.Unmarshal(contents, &v); err != nil {
					return err
				}

				if strings.TrimSpace(v.Amount) == "" {
					return fmt.Errorf("must specify amount of coins to bond")
				}
				amount, err := sdk.ParseCoinNormalized(v.Amount)
				if err != nil {
					return err
				}

				if v.PubKey == nil {
					return fmt.Errorf("must specify the JSON encoded pubkey")
				}

				var pubKey cryptotypes.PubKey
				if err := clientCtx.Codec.UnmarshalInterfaceJSON(v.PubKey, &pubKey); err != nil {
					return err
				}

				if strings.TrimSpace(v.Moniker) == "" {
					return fmt.Errorf("must specify the moniker name")
				}

				rate, err := math.LegacyNewDecFromStr(v.CommissionRate)
				if err != nil {
					return err
				}
				maxRate, err := math.LegacyNewDecFromStr(v.CommissionMaxRate)
				if err != nil {
					return err
				}
				maxChange, err := math.LegacyNewDecFromStr(v.CommissionMaxChange)
				if err != nil {
					return err
				}
				commission := stakingtypes.NewCommissionRates(rate, maxRate, maxChange)

				if strings.TrimSpace(v.MinSelfDelegation) == "" {
					return fmt.Errorf("must specify minimum self delegation")
				}
				minSelfDelegation, ok := math.NewIntFromString(v.MinSelfDelegation)
				if !ok {
					return fmt.Errorf("minimum self delegation must be a positive integer")
				}

				description := stakingtypes.NewDescription(
					v.Moniker,
					v.Identity,
					v.Website,
					v.Security,
					v.Details,
				)

				valStr, err := signingCtx.ValidatorAddressCodec().BytesToString(sdk.ValAddress(fromAddr))
				if err != nil {
					return err
				}

				msg, err := stakingtypes.NewMsgCreateValidator(
					valStr,
					pubKey,
					amount,
					description,
					commission,
					minSelfDelegation,
				)
				if err != nil {
					return err
				}

				return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
			}()

			if err != nil && os.Getenv("LUMEN_PQC_DEBUG") == "1" {
				fmt.Fprintf(os.Stderr, "[pqc-cli] staking create-validator RunE error: %v\n", err)
			}

			return err
		}
	}
}

func patchStakingDelegateCommand(stakingCmd *cobra.Command) {
	cmd, _, err := stakingCmd.Find([]string{"delegate"})
	if err != nil || cmd == nil {
		return
	}

	// Mark command as PQC-patched for easier debugging.
	if !strings.Contains(cmd.Short, "[PQC]") {
		if strings.TrimSpace(cmd.Short) == "" {
			cmd.Short = "Delegate tokens to a validator [PQC]"
		} else {
			cmd.Short = cmd.Short + " [PQC]"
		}
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

func patchSlashingTxCommands(root *cobra.Command) {
	path := []string{"tx", "slashing"}
	slashingCmd, _, err := root.Find(path)
	if err != nil || slashingCmd == nil {
		return
	}

	cmd, _, err := slashingCmd.Find([]string{"unjail"})
	if err != nil || cmd == nil {
		return
	}

	if !strings.Contains(cmd.Short, "[PQC]") {
		cmd.Short = strings.TrimSpace(cmd.Short + " [PQC]")
	}

	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		clientCtx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}

		signingCtx := clientCtx.TxConfig.SigningContext()
		if signingCtx == nil {
			return fmt.Errorf("pqc: signing context unavailable")
		}

		fromAddr := clientCtx.GetFromAddress()
		if fromAddr == nil {
			return fmt.Errorf("pqc: missing from address")
		}

		valStr, err := signingCtx.ValidatorAddressCodec().BytesToString(sdk.ValAddress(fromAddr))
		if err != nil {
			return err
		}

		msg := slashingtypes.NewMsgUnjail(valStr)
		return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
	}
}

// --- shared helpers ---------------------------------------------------------

type txGenFunc func(*cobra.Command, client.Context, *pflag.FlagSet, ...sdk.Msg) error

func splitAndApplyTx(
	gen txGenFunc,
	cmd *cobra.Command,
	clientCtx client.Context,
	fs *pflag.FlagSet,
	msgs []sdk.Msg,
	chunkSize int,
) error {
	if chunkSize == 0 {
		return gen(cmd, clientCtx, fs, msgs...)
	}

	totalMessages := len(msgs)
	for i := 0; i < totalMessages; i += chunkSize {
		end := i + chunkSize
		if end > totalMessages {
			end = totalMessages
		}
		if err := gen(cmd, clientCtx, fs, msgs[i:end]...); err != nil {
			return err
		}
	}

	return nil
}

// --- shared helpers ---------------------------------------------------------
