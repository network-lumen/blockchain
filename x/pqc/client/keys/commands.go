//go:build dev || test

package keys

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"lumen/crypto/pqc/dilithium"
	pqctypes "lumen/x/pqc/types"
)

// AttachCommands adds PQC key management commands under the standard `keys` tree.
func AttachCommands(keysCmd *cobra.Command) {
	keysCmd.AddCommand(
		NewImportCmd(),
		NewListCmd(),
		NewShowCmd(),
		NewLinkCmd(),
	)
}

// NewImportCmd returns the `keys pqc-import` command.
func NewImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pqc-import",
		Short: "Import a Dilithium keypair into the local PQC keystore",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)

			name, err := cmd.Flags().GetString("name")
			if err != nil {
				return err
			}
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("--name is required")
			}

			scheme, err := cmd.Flags().GetString("scheme")
			if err != nil {
				return err
			}
			if !pqctypes.IsSupportedScheme(scheme) {
				return fmt.Errorf("unsupported scheme %q", scheme)
			}

			pubInput, err := cmd.Flags().GetString("pubkey")
			if err != nil {
				return err
			}
			if strings.TrimSpace(pubInput) == "" {
				return fmt.Errorf("--pubkey is required")
			}
			pubKey, err := DecodeKey(pubInput)
			if err != nil {
				return err
			}

			privInput, err := cmd.Flags().GetString("privkey")
			if err != nil {
				return err
			}
			if strings.TrimSpace(privInput) == "" {
				return fmt.Errorf("--privkey is required")
			}
			privKey, err := DecodeKey(privInput)
			if err != nil {
				return err
			}

			backend := dilithium.Default()
			if !strings.EqualFold(backend.Name(), scheme) {
				return fmt.Errorf("active backend %q does not match requested scheme %q", backend.Name(), scheme)
			}
			if len(pubKey) != backend.PublicKeySize() {
				return fmt.Errorf("public key must be %d bytes", backend.PublicKeySize())
			}
			if len(privKey) == 0 {
				return fmt.Errorf("private key cannot be empty")
			}

			store, err := LoadStore(clientCtx.HomeDir)
			if err != nil {
				return err
			}

			record := KeyRecord{
				Name:       name,
				Scheme:     scheme,
				PublicKey:  append([]byte(nil), pubKey...),
				PrivateKey: append([]byte(nil), privKey...),
				CreatedAt:  time.Now().UTC(),
			}
			if err := store.SaveKey(record); err != nil {
				return err
			}

			cmd.Println("PQC key imported (stored insecurely on disk; not suitable for production HSM flows).")
			return nil
		},
	}

	cmd.Flags().String("name", "", "Local name for the PQC key")
	cmd.Flags().String("scheme", dilithium.Default().Name(), "PQC scheme identifier")
	cmd.Flags().String("pubkey", "", "Public key bytes (hex or base64)")
	cmd.Flags().String("privkey", "", "Private key bytes (hex or base64)")
	return cmd
}

// NewListCmd lists the locally stored PQC keys and bindings.
func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pqc-list",
		Short: "List PQC keys and local address bindings",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)

			store, err := LoadStore(clientCtx.HomeDir)
			if err != nil {
				return err
			}

			keys := store.ListKeys()
			sort.Slice(keys, func(i, j int) bool { return keys[i].Name < keys[j].Name })

			if len(keys) == 0 {
				cmd.Println("No PQC keys imported.")
			} else {
				cmd.Println("PQC Keys:")
				for _, k := range keys {
					fp := fingerprint(k.PublicKey)
					cmd.Printf("  - %s (%s) fingerprint %s\n", k.Name, k.Scheme, fp)
				}
			}

			links := store.ListLinks()
			if len(links) == 0 {
				cmd.Println("\nNo local address bindings.")
			} else {
				cmd.Println("\nBindings:")
				ordered := make([]string, 0, len(links))
				for addr := range links {
					ordered = append(ordered, addr)
				}
				sort.Strings(ordered)
				for _, addr := range ordered {
					cmd.Printf("  - %s -> %s\n", addr, links[addr])
				}
			}

			return nil
		},
	}
	return cmd
}

// NewShowCmd prints details for a single PQC key.
func NewShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pqc-show <name>",
		Short: "Show information about a stored PQC key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)

			store, err := LoadStore(clientCtx.HomeDir)
			if err != nil {
				return err
			}

			record, ok := store.GetKey(args[0])
			if !ok {
				return fmt.Errorf("no PQC key named %q", args[0])
			}

			cmd.Printf("Name:        %s\n", record.Name)
			cmd.Printf("Scheme:      %s\n", record.Scheme)
			cmd.Printf("Fingerprint: %s\n", fingerprint(record.PublicKey))
			cmd.Printf("Created At:  %s\n", record.CreatedAt.Format(time.RFC3339))
			cmd.Printf("PubKey (hex): %s\n", hex.EncodeToString(record.PublicKey))
			cmd.Printf("PrivKey (hex): %s\n", hex.EncodeToString(record.PrivateKey))
			cmd.Println("\nWARNING: PQC keys are stored in plaintext on disk; use only for development or test environments.")
			return nil
		},
	}
	return cmd
}

// NewLinkCmd links a cosmos key to a PQC key name.
func NewLinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pqc-link",
		Short: "Link a cosmos key (from the keyring) to a local PQC key",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)

			fromName, err := cmd.Flags().GetString(flags.FlagFrom)
			if err != nil {
				return err
			}
			if strings.TrimSpace(fromName) == "" {
				return fmt.Errorf("--from is required")
			}

			pqcName, err := cmd.Flags().GetString("pqc")
			if err != nil {
				return err
			}
			if strings.TrimSpace(pqcName) == "" {
				return fmt.Errorf("--pqc is required")
			}

			store, err := LoadStore(clientCtx.HomeDir)
			if err != nil {
				return err
			}

			if _, ok := store.GetKey(pqcName); !ok {
				return fmt.Errorf("no PQC key named %q - import it first", pqcName)
			}

			if clientCtx.Keyring == nil {
				return fmt.Errorf("keyring unavailable: ensure --home is set")
			}

			target, err := clientCtx.Keyring.Key(fromName)
			if err != nil {
				return err
			}

			addr, err := target.GetAddress()
			if err != nil {
				return fmt.Errorf("resolve address for %q: %w", fromName, err)
			}

			if err := validateAccountAddress(addr); err != nil {
				return err
			}

			if err := store.LinkAddress(addr.String(), pqcName); err != nil {
				return err
			}

			cmd.Printf("Linked %s -> %s\n", addr.String(), pqcName)
			return nil
		},
	}

	cmd.Flags().String(flags.FlagFrom, "", "Name of the cosmos key in the keyring")
	cmd.Flags().String("pqc", "", "Name of the local PQC key to link")
	return cmd
}

func fingerprint(pub []byte) string {
	hash := sha256.Sum256(pub)
	return hex.EncodeToString(hash[:8])
}

func validateAccountAddress(addr sdk.AccAddress) error {
	if len(addr) == 0 {
		return fmt.Errorf("empty address")
	}
	return nil
}
