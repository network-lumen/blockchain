package cli

import (
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	"lumen/crypto/pqc/dilithium"
	pqckeys "lumen/x/pqc/client/keys"
	pqctxext "lumen/x/pqc/client/txext"
	"lumen/x/pqc/types"
)

func NewTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "PQC transactions",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(LinkAccountPQCCmd())
	return cmd
}

func LinkAccountPQCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link-account",
		Short: "Link a PQC public key to the --from account",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			schemeName, err := cmd.Flags().GetString("scheme")
			if err != nil {
				return err
			}
			if !types.IsSupportedScheme(schemeName) {
				return fmt.Errorf("unsupported scheme %q", schemeName)
			}

			pubInput, err := cmd.Flags().GetString("pubkey")
			if err != nil {
				return err
			}
			if strings.TrimSpace(pubInput) == "" {
				return fmt.Errorf("--pubkey is required")
			}
			pubKey, err := pqckeys.DecodeKey(pubInput)
			if err != nil {
				return err
			}

			backend := dilithium.Default()
			if !strings.EqualFold(backend.Name(), schemeName) {
				return fmt.Errorf("active backend %q does not match requested scheme %q", backend.Name(), schemeName)
			}
			if len(pubKey) != backend.PublicKeySize() {
				return fmt.Errorf("public key must be %d bytes", backend.PublicKeySize())
			}

			msg := types.NewMsgLinkAccountPQC(clientCtx.GetFromAddress(), schemeName, pubKey)
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String("scheme", types.SchemeDilithium3, "PQC scheme identifier")
	cmd.Flags().String("pubkey", "", "PQC public key in hex or base64")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
