package types

import (
	"crypto/sha256"
	"fmt"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:   DefaultParams(),
		Accounts: []AccountPQC{},
	}
}

func ValidateGenesis(genState *GenesisState) error {
	if genState == nil {
		return fmt.Errorf("genesis state cannot be nil")
	}
	return genState.Validate()
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return errorsmod.Wrap(err, "params")
	}

	seen := make(map[string]struct{})
	for i, account := range gs.Accounts {
		if strings.TrimSpace(account.Addr) == "" {
			return fmt.Errorf("accounts[%d]: addr cannot be empty", i)
		}
		if _, err := sdk.AccAddressFromBech32(account.Addr); err != nil {
			return errorsmod.Wrapf(err, "accounts[%d]: invalid addr", i)
		}
		if _, ok := seen[account.Addr]; ok {
			return fmt.Errorf("duplicate pqc account for address %s", account.Addr)
		}
		seen[account.Addr] = struct{}{}

		if !IsSupportedScheme(account.Scheme) {
			return fmt.Errorf("accounts[%d]: unsupported scheme %q", i, account.Scheme)
		}
		if len(account.PubKeyHash) != sha256.Size {
			return fmt.Errorf("accounts[%d]: pub_key_hash must be %d bytes", i, sha256.Size)
		}
	}

	return nil
}

func IsSupportedScheme(s string) bool {
	for _, supported := range SupportedSchemes() {
		if strings.EqualFold(supported, s) {
			return true
		}
	}
	return false
}
