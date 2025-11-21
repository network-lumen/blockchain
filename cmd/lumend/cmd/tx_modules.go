package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	gogotypes "github.com/cosmos/gogoproto/types"

	dnstypes "lumen/x/dns/types"
	gatewaytypes "lumen/x/gateways/types"
	pqctxext "lumen/x/pqc/client/txext"
	releasetypes "lumen/x/release/types"
)

// DNS ------------------------------------------------------------------------

func newDNSTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "dns",
		Short:                      "DNS transactions",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		newDNSRegisterCmd(),
		newDNSRenewCmd(),
		newDNSTransferCmd(),
		newDNSBidCmd(),
		newDNSSettleCmd(),
		newDNSUpdateCmd(),
		newDNSUpdateDomainCmd(),
	)
	return cmd
}

func parseDNSRecordsJSON(raw string) ([]*dnstypes.Record, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil, nil
	}

	var list []*dnstypes.Record
	if err := json.Unmarshal([]byte(trimmed), &list); err == nil {
		return list, nil
	}

	var single dnstypes.Record
	if err := json.Unmarshal([]byte(trimmed), &single); err == nil {
		if single.Key == "" && single.Value == "" {
			return nil, nil
		}
		return []*dnstypes.Record{{Key: single.Key, Value: single.Value, Ttl: single.Ttl}}, nil
	}
	return nil, fmt.Errorf("invalid records JSON")
}

func newDNSRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register [domain] [ext]",
		Short: "Register or renew a domain",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			recordsJSON, err := cmd.Flags().GetString("records")
			if err != nil {
				return err
			}
			records, err := parseDNSRecordsJSON(recordsJSON)
			if err != nil {
				return fmt.Errorf("parse --records: %w", err)
			}

			duration, err := cmd.Flags().GetUint64("duration-days")
			if err != nil {
				return err
			}

			owner, err := cmd.Flags().GetString("owner")
			if err != nil {
				return err
			}
			if owner == "" {
				owner = clientCtx.GetFromAddress().String()
			}

			msg := &dnstypes.MsgRegister{
				Creator:      clientCtx.GetFromAddress().String(),
				Domain:       args[0],
				Ext:          args[1],
				Records:      records,
				DurationDays: duration,
				Owner:        owner,
			}

			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("records", "[]", "JSON array of DNS record objects")
	cmd.Flags().Uint64("duration-days", 365, "Registration duration in days")
	cmd.Flags().String("owner", "", "Explicit owner address (defaults to --from)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newDNSRenewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "renew [domain] [ext] [duration-days]",
		Short: "Renew an existing domain for N days",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			duration, err := strconv.ParseUint(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("parse duration-days: %w", err)
			}

			msg := &dnstypes.MsgRenew{
				Creator:      clientCtx.GetFromAddress().String(),
				Domain:       args[0],
				Ext:          args[1],
				DurationDays: duration,
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newDNSBidCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bid [domain] [ext] [amount]",
		Short: "Submit an auction bid",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &dnstypes.MsgBid{
				Creator: clientCtx.GetFromAddress().String(),
				Domain:  args[0],
				Ext:     args[1],
				Amount:  args[2],
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newDNSTransferCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer [domain] [ext] [new-owner]",
		Short: "Transfer domain ownership",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &dnstypes.MsgTransfer{
				Creator:  clientCtx.GetFromAddress().String(),
				Domain:   args[0],
				Ext:      args[1],
				NewOwner: args[2],
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newDNSSettleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settle [domain] [ext]",
		Short: "Settle a completed auction",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &dnstypes.MsgSettle{
				Creator: clientCtx.GetFromAddress().String(),
				Domain:  args[0],
				Ext:     args[1],
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newDNSUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [domain] [ext] [records-json]",
		Short: "Update an existing domain's records (requires PoW)",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			recordsJSON, err := cmd.Flags().GetString("records")
			if err != nil {
				return err
			}
			if len(args) >= 3 {
				recordsJSON = args[2]
			}
			records, err := parseDNSRecordsJSON(recordsJSON)
			if err != nil {
				return fmt.Errorf("parse --records: %w", err)
			}

			powNonce, err := cmd.Flags().GetUint64("pow-nonce")
			if err != nil {
				return err
			}

			msg := &dnstypes.MsgUpdate{
				Creator:  clientCtx.GetFromAddress().String(),
				Domain:   args[0],
				Ext:      args[1],
				Records:  records,
				PowNonce: powNonce,
			}

			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("records", "[]", "JSON array of DNS record objects")
	cmd.Flags().Uint64("pow-nonce", 0, "nonce satisfying the update PoW requirement")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newDNSUpdateDomainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-domain [index] [name] [owner] [records-json] [expire-at]",
		Short: "Update a domain record (testing helper)",
		Args:  cobra.ExactArgs(5),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			expireAt, err := strconv.ParseUint(args[4], 10, 64)
			if err != nil {
				return fmt.Errorf("parse expire-at: %w", err)
			}

			records, err := parseDNSRecordsJSON(args[3])
			if err != nil {
				return fmt.Errorf("parse records-json: %w", err)
			}

			powNonce, err := cmd.Flags().GetUint64("pow-nonce")
			if err != nil {
				return err
			}

			msg := &dnstypes.MsgUpdateDomain{
				Creator:  clientCtx.GetFromAddress().String(),
				Index:    args[0],
				Name:     args[1],
				Owner:    args[2],
				Records:  records,
				ExpireAt: expireAt,
				PowNonce: powNonce,
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().Uint64("pow-nonce", 0, "nonce satisfying the update PoW requirement")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// Release --------------------------------------------------------------------

func newReleaseTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "release",
		Short:                      "Release module transactions",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		newReleasePublishCmd(),
		newReleaseMirrorCmd(),
		newReleaseYankCmd(),
	)
	return cmd
}

func newReleasePublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish a new release (expects MsgPublishRelease JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readJSONPayload(cmd)
			if err != nil {
				return err
			}

			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			var msg releasetypes.MsgPublishRelease
			if err := json.Unmarshal(payload, &msg); err != nil {
				return fmt.Errorf("parse --msg payload: %w", err)
			}
			if msg.Creator == "" {
				msg.Creator = clientCtx.GetFromAddress().String()
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), &msg)
		},
	}
	addJSONPayloadFlags(cmd)
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newReleaseMirrorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Mirror release artifacts (expects MsgMirrorRelease JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readJSONPayload(cmd)
			if err != nil {
				return err
			}

			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			var msg releasetypes.MsgMirrorRelease
			if err := json.Unmarshal(payload, &msg); err != nil {
				return fmt.Errorf("parse --msg payload: %w", err)
			}
			if msg.Creator == "" {
				msg.Creator = clientCtx.GetFromAddress().String()
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), &msg)
		},
	}
	addJSONPayloadFlags(cmd)
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newReleaseYankCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "yank",
		Short: "Yank a release (expects MsgYankRelease JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readJSONPayload(cmd)
			if err != nil {
				return err
			}

			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			var msg releasetypes.MsgYankRelease
			if err := json.Unmarshal(payload, &msg); err != nil {
				return fmt.Errorf("parse --msg payload: %w", err)
			}
			if msg.Creator == "" {
				msg.Creator = clientCtx.GetFromAddress().String()
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), &msg)
		},
	}
	addJSONPayloadFlags(cmd)
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func addJSONPayloadFlags(cmd *cobra.Command) {
	cmd.Flags().String("msg", "", "Inline JSON representing the message")
	cmd.Flags().String("msg-file", "", "Path to JSON file representing the message")
}

func readJSONPayload(cmd *cobra.Command) ([]byte, error) {
	inline, err := cmd.Flags().GetString("msg")
	if err != nil {
		return nil, err
	}
	filePath, err := cmd.Flags().GetString("msg-file")
	if err != nil {
		return nil, err
	}
	if inline == "" && filePath == "" {
		return nil, fmt.Errorf("either --msg or --msg-file must be provided")
	}
	if inline != "" && filePath != "" {
		return nil, fmt.Errorf("only one of --msg or --msg-file can be provided")
	}
	if filePath != "" {
		return os.ReadFile(filePath)
	}
	return []byte(inline), nil
}

// Gateways -------------------------------------------------------------------

func newGatewaysTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "gateways",
		Short:                      "Gateway contract transactions",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		newGatewayRegisterCmd(),
		newGatewayUpdateCmd(),
		newGatewayCreateContractCmd(),
		newGatewayClaimPaymentCmd(),
		newGatewayCancelContractCmd(),
		newGatewayFinalizeContractCmd(),
	)
	return cmd
}

func newGatewayRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-gateway [payout-addr]",
		Short: "Register a gateway operator (pays register_gateway_fee_ulmn)",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			payout := clientCtx.GetFromAddress().String()
			if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
				payout = strings.TrimSpace(args[0])
			}

			metadata, err := cmd.Flags().GetString("metadata")
			if err != nil {
				return err
			}

			msg := &gatewaytypes.MsgRegisterGateway{
				Operator: clientCtx.GetFromAddress().String(),
				Payout:   payout,
				Metadata: metadata,
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("metadata", "", "Optional metadata string (<=512 chars)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGatewayUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-gateway [gateway-id]",
		Short: "Update gateway metadata/payout/active flag (pays action_fee_ulmn)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			gatewayID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("parse gateway-id: %w", err)
			}

			payout, err := cmd.Flags().GetString("payout")
			if err != nil {
				return err
			}
			metadata, err := cmd.Flags().GetString("metadata")
			if err != nil {
				return err
			}
			active, err := cmd.Flags().GetBool("active")
			if err != nil {
				return err
			}

			msg := &gatewaytypes.MsgUpdateGateway{
				Operator:  clientCtx.GetFromAddress().String(),
				GatewayId: gatewayID,
			}
			if cmd.Flags().Changed("payout") {
				msg.Payout = &gogotypes.StringValue{Value: strings.TrimSpace(payout)}
			}
			if cmd.Flags().Changed("metadata") {
				msg.Metadata = &gogotypes.StringValue{Value: metadata}
			}
			if cmd.Flags().Changed("active") {
				msg.Active = &gogotypes.BoolValue{Value: active}
			}

			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("payout", "", "Optional payout address override")
	cmd.Flags().String("metadata", "", "Optional metadata update")
	cmd.Flags().Bool("active", false, "Set active flag (requires explicit flag)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGatewayCreateContractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-contract [gateway-id] [price-ulmn] [storage-gb] [network-gb] [months-total]",
		Short: "Create a client contract with a gateway",
		Args:  cobra.ExactArgs(5),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			gatewayID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("parse gateway-id: %w", err)
			}
			price, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("parse price-ulmn: %w", err)
			}
			storage, err := strconv.ParseUint(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("parse storage-gb: %w", err)
			}
			network, err := strconv.ParseUint(args[3], 10, 64)
			if err != nil {
				return fmt.Errorf("parse network-gb: %w", err)
			}
			months, err := strconv.ParseUint(args[4], 10, 32)
			if err != nil {
				return fmt.Errorf("parse months-total: %w", err)
			}
			metadata, err := cmd.Flags().GetString("metadata")
			if err != nil {
				return err
			}

			msg := &gatewaytypes.MsgCreateContract{
				Client:            clientCtx.GetFromAddress().String(),
				GatewayId:         gatewayID,
				PriceUlmn:         price,
				StorageGbPerMonth: storage,
				NetworkGbPerMonth: network,
				MonthsTotal:       uint32(months),
				Metadata:          metadata,
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("metadata", "", "Optional metadata string")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGatewayClaimPaymentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claim-payment [contract-id]",
		Short: "Claim a contract payout (gateway operator)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			contractID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("parse contract-id: %w", err)
			}

			msg := &gatewaytypes.MsgClaimPayment{
				Operator:   clientCtx.GetFromAddress().String(),
				ContractId: contractID,
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGatewayCancelContractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel-contract [contract-id]",
		Short: "Cancel an active contract (client)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			contractID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("parse contract-id: %w", err)
			}

			msg := &gatewaytypes.MsgCancelContract{
				Client:     clientCtx.GetFromAddress().String(),
				ContractId: contractID,
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGatewayFinalizeContractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "finalize-contract [contract-id]",
		Short: "Finalize a completed contract (finalizer)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			contractID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("parse contract-id: %w", err)
			}

			msg := &gatewaytypes.MsgFinalizeContract{
				Finalizer:  clientCtx.GetFromAddress().String(),
				ContractId: contractID,
			}
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
