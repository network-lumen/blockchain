package cli

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
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

			query := types.NewQueryClient(clientCtx)
			paramsResp, err := query.Params(cmd.Context(), &types.QueryParamsRequest{})
			if err != nil {
				return err
			}
			if err := ensureMinBalance(cmd.Context(), clientCtx, paramsResp.Params.GetMinBalanceForLink()); err != nil {
				return err
			}
			powNonce, err := computePowNonce(pubKey, paramsResp.Params.PowDifficultyBits)
			if err != nil {
				return err
			}

			msg := types.NewMsgLinkAccountPQC(clientCtx.GetFromAddress(), schemeName, pubKey)
			msg.PowNonce = powNonce
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

func computePowNonce(pubKey []byte, bits uint32) ([]byte, error) {
	if bits == 0 {
		return []byte{0x00}, nil
	}
	nonce := make([]byte, 8)
	var counter uint64
	for {
		binary.BigEndian.PutUint64(nonce, counter)
		digest := types.ComputePowDigest(pubKey, nonce)
		if types.LeadingZeroBits(digest[:]) >= int(bits) {
			return append([]byte(nil), nonce...), nil
		}
		counter++
		if counter == 0 {
			return nil, fmt.Errorf("pow search exhausted")
		}
	}
}

func ensureMinBalance(ctx context.Context, clientCtx client.Context, coin sdk.Coin) error {
	if strings.TrimSpace(coin.Denom) == "" {
		return nil
	}
	if !coin.IsValid() || !coin.Amount.IsPositive() {
		return nil
	}

	query := banktypes.NewQueryClient(clientCtx)
	resp, err := query.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: clientCtx.GetFromAddress().String(),
		Denom:   coin.Denom,
	})
	if err != nil {
		return err
	}
	balance := resp.GetBalance()
	if balance == nil || balance.Amount.LT(coin.Amount) {
		current := "0"
		if balance != nil {
			current = balance.Amount.String()
		}
		return fmt.Errorf(
			"PQC link requires at least %s%s (current %s%s)",
			coin.Amount.String(), coin.Denom, current, coin.Denom,
		)
	}
	return nil
}
