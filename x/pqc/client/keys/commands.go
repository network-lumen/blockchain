package keys

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"lumen/crypto/pqc/dilithium"
	pqctypes "lumen/x/pqc/types"
)

// AttachCommands adds PQC key management commands under the standard `keys` tree.
func AttachCommands(keysCmd *cobra.Command) {
	keysCmd.AddCommand(
		NewGenerateCmd(),
		NewImportCmd(),
		NewListCmd(),
		NewShowCmd(),
		NewLinkCmd(),
		NewGenesisEntryCmd(),
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

			store, passphrase, err := openStore(cmd, clientCtx)
			if err != nil {
				return err
			}
			defer wipeBytes(passphrase)

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

			if len(passphrase) == 0 {
				cmd.Println("[pqc-keystore] WARNING: key stored unencrypted. Provide --pqc-passphrase or --pqc-passphrase-file to enable encryption.")
			}
			cmd.Println("PQC key imported.")
			return nil
		},
	}

	cmd.Flags().String("name", "", "Local name for the PQC key")
	cmd.Flags().String("scheme", dilithium.Default().Name(), "PQC scheme identifier")
	cmd.Flags().String("pubkey", "", "Public key bytes (hex or base64)")
	cmd.Flags().String("privkey", "", "Private key bytes (hex or base64)")
	addPassphraseFlags(cmd)
	return cmd
}

// NewListCmd lists the locally stored PQC keys and bindings.
func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pqc-list",
		Short: "List PQC keys and local address bindings",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)

			store, passphrase, err := openStore(cmd, clientCtx)
			if err != nil {
				return err
			}
			defer wipeBytes(passphrase)

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
	addPassphraseFlags(cmd)
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

			store, passphrase, err := openStore(cmd, clientCtx)
			if err != nil {
				return err
			}
			defer wipeBytes(passphrase)

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
			if len(passphrase) == 0 {
				cmd.Println("\nWARNING: PQC keys are stored in plaintext; pass --pqc-passphrase-file to enable encryption.")
			}
			return nil
		},
	}
	addPassphraseFlags(cmd)
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

			store, passphrase, err := openStore(cmd, clientCtx)
			if err != nil {
				return err
			}
			defer wipeBytes(passphrase)

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
	addPassphraseFlags(cmd)
	return cmd
}

// NewGenerateCmd generates a Dilithium keypair and optionally stores it locally.
func NewGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pqc-generate",
		Short: "Generate a Dilithium keypair",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)

			schemeFlag, err := cmd.Flags().GetString("scheme")
			if err != nil {
				return err
			}
			backend := dilithium.Default()
			if !strings.EqualFold(backend.Name(), schemeFlag) {
				return fmt.Errorf("active backend %q does not match requested scheme %q", backend.Name(), schemeFlag)
			}

			pubKey, privKey, err := backend.GenerateKey(nil)
			if err != nil {
				return fmt.Errorf("generate pqc key: %w", err)
			}

			pubHex := hex.EncodeToString(pubKey)
			privHex := hex.EncodeToString(privKey)

			pubOut, err := cmd.Flags().GetString("pub-out")
			if err != nil {
				return err
			}
			privOut, err := cmd.Flags().GetString("priv-out")
			if err != nil {
				return err
			}
			force, err := cmd.Flags().GetBool("force")
			if err != nil {
				return err
			}

			if pubOut != "" {
				if err := writeStringToFile(pubOut, pubHex+"\n", force); err != nil {
					return err
				}
				cmd.Printf("Public key written to %s\n", pubOut)
			}
			if privOut != "" {
				if err := writeStringToFile(privOut, privHex+"\n", force); err != nil {
					return err
				}
				cmd.Printf("Private key written to %s\n", privOut)
			}

			name, err := cmd.Flags().GetString("name")
			if err != nil {
				return err
			}
			name = strings.TrimSpace(name)

			linkFrom, err := cmd.Flags().GetString("link-from")
			if err != nil {
				return err
			}
			linkFrom = strings.TrimSpace(linkFrom)
			if linkFrom != "" && name == "" {
				return fmt.Errorf("--link-from requires --name to store the key locally")
			}

			var (
				store      *Store
				passphrase []byte
			)
			if name != "" || linkFrom != "" {
				store, passphrase, err = openStore(cmd, clientCtx)
				if err != nil {
					return err
				}
				defer wipeBytes(passphrase)
			}

			if name != "" {
				record := KeyRecord{
					Name:       name,
					Scheme:     backend.Name(),
					PublicKey:  append([]byte(nil), pubKey...),
					PrivateKey: append([]byte(nil), privKey...),
					CreatedAt:  time.Now().UTC(),
				}
				if err := store.SaveKey(record); err != nil {
					return err
				}
				cmd.Printf("Stored PQC key as %q (fingerprint %s)\n", name, fingerprint(pubKey))
				if len(passphrase) == 0 {
					cmd.Println("[pqc-keystore] WARNING: stored in plaintext. Supply --pqc-passphrase or --pqc-passphrase-file to encrypt the keystore.")
				}
			}

			if linkFrom != "" {
				if clientCtx.Keyring == nil {
					return fmt.Errorf("keyring unavailable: ensure --home is set")
				}
				target, err := clientCtx.Keyring.Key(linkFrom)
				if err != nil {
					return err
				}
				addr, err := target.GetAddress()
				if err != nil {
					return fmt.Errorf("resolve address for %q: %w", linkFrom, err)
				}
				if err := validateAccountAddress(addr); err != nil {
					return err
				}
				if err := store.LinkAddress(addr.String(), name); err != nil {
					return err
				}
				cmd.Printf("Linked %s -> %s\n", addr.String(), name)
			}

			if pubOut == "" && privOut == "" {
				cmd.Printf("Public (hex): %s\nPrivate (hex): %s\n", pubHex, privHex)
			}
			return nil
		},
	}

	cmd.Flags().String("scheme", dilithium.Default().Name(), "PQC scheme identifier")
	cmd.Flags().String("name", "", "Optionally store the generated key under this name")
	cmd.Flags().String("link-from", "", "Link the stored key to the given cosmos key (requires --name)")
	cmd.Flags().String("pub-out", "", "Write the public key (hex) to this file")
	cmd.Flags().String("priv-out", "", "Write the private key (hex) to this file")
	cmd.Flags().Bool("force", false, "Allow overwriting existing output files")
	addPassphraseFlags(cmd)
	return cmd
}

// NewGenesisEntryCmd prints a JSON snippet for inclusion in app_state.pqc.accounts.
func NewGenesisEntryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pqc-genesis-entry",
		Short: "Produce a genesis entry for app_state.pqc.accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			if clientCtx.Keyring == nil {
				return fmt.Errorf("keyring unavailable: ensure --home is set")
			}

			fromName, err := cmd.Flags().GetString(flags.FlagFrom)
			if err != nil {
				return err
			}
			if strings.TrimSpace(fromName) == "" {
				return fmt.Errorf("--from is required")
			}

			store, passphrase, err := openStore(cmd, clientCtx)
			if err != nil {
				return err
			}
			defer wipeBytes(passphrase)

			target, err := clientCtx.Keyring.Key(fromName)
			if err != nil {
				return err
			}
			addr, err := target.GetAddress()
			if err != nil {
				return fmt.Errorf("resolve address for %q: %w", fromName, err)
			}

			pqcName, err := cmd.Flags().GetString("pqc")
			if err != nil {
				return err
			}
			pqcName = strings.TrimSpace(pqcName)
			if pqcName == "" {
				if linked, ok := store.GetLink(addr.String()); ok {
					pqcName = linked
				} else {
					return fmt.Errorf("no PQC key linked to %s; specify --pqc", addr.String())
				}
			}

			record, ok := store.GetKey(pqcName)
			if !ok {
				return fmt.Errorf("no PQC key named %q", pqcName)
			}

			hash := sha256.Sum256(record.PublicKey)
			addedAt, err := cmd.Flags().GetInt64("added-at")
			if err != nil {
				return err
			}

			entry := pqctypes.AccountPQC{
				Addr:       addr.String(),
				Scheme:     record.Scheme,
				PubKeyHash: append([]byte(nil), hash[:]...),
				AddedAt:    addedAt,
			}

			data, err := marshalJSON(entry, cmd.Flags())
			if err != nil {
				return err
			}

			outPath, err := cmd.Flags().GetString("output")
			if err != nil {
				return err
			}
			force, err := cmd.Flags().GetBool("force")
			if err != nil {
				return err
			}

			if outPath != "" {
				if err := writeBytesToFile(outPath, data, force); err != nil {
					return err
				}
				cmd.Printf("Genesis entry written to %s\n", outPath)
			} else {
				cmd.Println(string(data))
			}
			genesisPath, err := cmd.Flags().GetString("write-genesis")
			if err != nil {
				return err
			}
			if strings.TrimSpace(genesisPath) != "" {
				pretty, _ := cmd.Flags().GetBool("pretty")
				if err := injectEntryIntoGenesis(genesisPath, entry, pretty, force); err != nil {
					return err
				}
				cmd.Printf("Updated %s with PQC entry for %s\n", genesisPath, entry.Addr)
			}
			return nil
		},
	}

	cmd.Flags().String(flags.FlagFrom, "", "Cosmos key (from the keyring) to describe in the entry")
	cmd.Flags().String("pqc", "", "Local PQC key name (defaults to the linked key for --from)")
	cmd.Flags().Int64("added-at", 0, "Value for the added_at field (defaults to 0)")
	cmd.Flags().String("output", "", "Write the JSON entry to this file instead of stdout")
	cmd.Flags().Bool("pretty", true, "Pretty-print the generated JSON")
	cmd.Flags().Bool("force", false, "Allow overwriting the output file")
	cmd.Flags().String("write-genesis", "", "Inject the entry directly into this genesis.json (a .bak backup will be created)")
	addPassphraseFlags(cmd)
	return cmd
}

func marshalJSON(v any, flagSet *pflag.FlagSet) ([]byte, error) {
	pretty, err := flagSet.GetBool("pretty")
	if err != nil {
		return nil, err
	}
	var (
		data []byte
	)
	if pretty {
		data, err = json.MarshalIndent(v, "", "  ")
	} else {
		data, err = json.Marshal(v)
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return data, nil
}

func writeStringToFile(path, data string, force bool) error {
	return writeBytesToFile(path, []byte(data), force)
}

func writeBytesToFile(path string, data []byte, force bool) error {
	if path == "" {
		return errors.New("output path is empty")
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", path, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func injectEntryIntoGenesis(path string, entry pqctypes.AccountPQC, pretty bool, force bool) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	original, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}

	backupPath := absPath + ".bak"
	if err := os.WriteFile(backupPath, original, 0o600); err != nil && !force {
		return fmt.Errorf("failed to write backup %s: %w", backupPath, err)
	}

	type rawMap map[string]json.RawMessage

	var genesis rawMap
	if err := json.Unmarshal(original, &genesis); err != nil {
		return fmt.Errorf("parse genesis: %w", err)
	}

	var appState rawMap
	if raw, ok := genesis["app_state"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &appState); err != nil {
			return fmt.Errorf("parse app_state: %w", err)
		}
	} else {
		appState = make(rawMap)
	}

	var pqcState rawMap
	if raw, ok := appState["pqc"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &pqcState); err != nil {
			return fmt.Errorf("parse app_state.pqc: %w", err)
		}
	} else {
		pqcState = make(rawMap)
	}

	var accounts []pqctypes.AccountPQC
	if raw, ok := pqcState["accounts"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &accounts); err != nil {
			return fmt.Errorf("parse pqc accounts: %w", err)
		}
	}

	replaced := false
	for i, acc := range accounts {
		if strings.EqualFold(acc.Addr, entry.Addr) {
			accounts[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		accounts = append(accounts, entry)
	}

	accBytes, err := json.Marshal(accounts)
	if err != nil {
		return err
	}
	pqcState["accounts"] = accBytes

	pqcBytes, err := json.Marshal(pqcState)
	if err != nil {
		return err
	}
	appState["pqc"] = pqcBytes

	appBytes, err := json.Marshal(appState)
	if err != nil {
		return err
	}
	genesis["app_state"] = appBytes

	var updated []byte
	if pretty {
		updated, err = json.MarshalIndent(genesis, "", "  ")
	} else {
		updated, err = json.Marshal(genesis)
	}
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	return os.WriteFile(absPath, updated, 0o600)
}

const (
	flagPassphrase     = "pqc-passphrase"
	flagPassphraseFile = "pqc-passphrase-file"
	envPassphrase      = "LUMEN_PQC_PASSPHRASE"
	envPassphraseFile  = "LUMEN_PQC_PASSPHRASE_FILE"
)

func addPassphraseFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagPassphrase, "", "Passphrase protecting the PQC keystore (prefer --pqc-passphrase-file or environment variables)")
	cmd.Flags().String(flagPassphraseFile, "", "Path to a file containing the PQC keystore passphrase")
}

func openStore(cmd *cobra.Command, clientCtx client.Context) (*Store, []byte, error) {
	passphrase, err := readPassphrase(cmd.Flags())
	if err != nil {
		return nil, nil, err
	}
	store, err := LoadStore(clientCtx.HomeDir, WithPassphrase(passphrase))
	if err != nil {
		wipeBytes(passphrase)
		return nil, nil, err
	}
	return store, passphrase, nil
}

func readPassphrase(flagSet *pflag.FlagSet) ([]byte, error) {
	filePath, err := flagSet.GetString(flagPassphraseFile)
	if err != nil {
		return nil, err
	}
	if filePath == "" {
		filePath = os.Getenv(envPassphraseFile)
	}
	if strings.TrimSpace(filePath) != "" {
		data, err := os.ReadFile(strings.TrimSpace(filePath))
		if err != nil {
			return nil, err
		}
		pass := strings.TrimSpace(string(data))
		if pass != "" {
			return []byte(pass), nil
		}
	}

	pass, err := flagSet.GetString(flagPassphrase)
	if err != nil {
		return nil, err
	}
	if pass == "" {
		pass = os.Getenv(envPassphrase)
	}
	pass = strings.TrimSpace(pass)
	if pass == "" {
		return nil, nil
	}
	return []byte(pass), nil
}

func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
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
