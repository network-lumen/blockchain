package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"

	pqctxext "lumen/x/pqc/client/txext"
)

func newIBCTransferTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "ibc-transfer",
		Short:                      "IBC transfer transactions",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(newIBCTransferTransferCmd())
	return cmd
}

func newIBCTransferTransferCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer [source-channel] [receiver] [amount]",
		Short: "Send an ICS-20 transfer [PQC]",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			sourcePort, err := cmd.Flags().GetString("source-port")
			if err != nil {
				return err
			}

			timeoutHeightRaw, err := cmd.Flags().GetString("packet-timeout-height")
			if err != nil {
				return err
			}
			timeoutHeight, err := parseIBCHeight(timeoutHeightRaw)
			if err != nil {
				return err
			}

			timeoutSeconds, err := cmd.Flags().GetUint64("packet-timeout-seconds")
			if err != nil {
				return err
			}

			packetMemo, err := cmd.Flags().GetString("packet-memo")
			if err != nil {
				return err
			}

			coin, err := sdk.ParseCoinNormalized(args[2])
			if err != nil {
				return fmt.Errorf("parse amount: %w", err)
			}
			if !coin.IsPositive() {
				return fmt.Errorf("amount must be positive")
			}

			var timeoutTimestamp uint64
			if timeoutSeconds > 0 {
				timeoutTimestamp = uint64(time.Now().Add(time.Duration(timeoutSeconds) * time.Second).UnixNano())
			}

			msg := ibctransfertypes.NewMsgTransfer(
				sourcePort,
				args[0],
				coin,
				clientCtx.GetFromAddress().String(),
				args[1],
				timeoutHeight,
				timeoutTimestamp,
				packetMemo,
			)

			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	cmd.Flags().String("source-port", ibctransfertypes.PortID, "IBC source port")
	cmd.Flags().String("packet-timeout-height", "0-0", "IBC packet timeout height as revision-height (for example 1-100)")
	cmd.Flags().Uint64("packet-timeout-seconds", 600, "Relative packet timeout in seconds; set to 0 to disable timestamp timeout")
	cmd.Flags().String("packet-memo", "", "Optional ICS-20 packet memo")
	return cmd
}

func parseIBCHeight(raw string) (clienttypes.Height, error) {
	if raw == "" || raw == "0-0" {
		return clienttypes.ZeroHeight(), nil
	}

	height, err := clienttypes.ParseHeight(raw)
	if err != nil {
		return clienttypes.ZeroHeight(), fmt.Errorf("parse timeout height: %w", err)
	}

	return height, nil
}
