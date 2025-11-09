//go:build !dev && !test

package keys

import (
	"errors"

	"github.com/spf13/cobra"
)

const disabledMsg = "pqc: plaintext key commands are disabled in production builds"

func AttachCommands(keysCmd *cobra.Command) {
	keysCmd.AddCommand(
		newDisabledCmd("pqc-import"),
		newDisabledCmd("pqc-list"),
		newDisabledCmd("pqc-show"),
		newDisabledCmd("pqc-link"),
	)
}

func NewImportCmd() *cobra.Command { return newDisabledCmd("pqc-import") }
func NewListCmd() *cobra.Command   { return newDisabledCmd("pqc-list") }
func NewShowCmd() *cobra.Command   { return newDisabledCmd("pqc-show") }
func NewLinkCmd() *cobra.Command   { return newDisabledCmd("pqc-link") }

func newDisabledCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use:    use,
		Hidden: true,
		RunE: func(*cobra.Command, []string) error {
			return errors.New(disabledMsg)
		},
	}
}
