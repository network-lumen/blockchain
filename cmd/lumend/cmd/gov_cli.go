package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/version"
	govcli "github.com/cosmos/cosmos-sdk/x/gov/client/cli"
	govutils "github.com/cosmos/cosmos-sdk/x/gov/client/utils"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"

	pqctxext "lumen/x/pqc/client/txext"
)

const (
	govFlagTitle    = "title"
	govFlagDeposit  = "deposit"
	govFlagMetadata = "metadata"
	govFlagSummary  = "summary"

	govFlagProposal     = "proposal"
	govFlagDescription  = "description"
	govFlagProposalType = "type"
)

var govProposalFlags = []string{
	govFlagTitle,
	govFlagDescription,
	govFlagProposalType,
	govFlagDeposit,
}

func newGovTxCmd() *cobra.Command {
	govTxCmd := &cobra.Command{
		Use:                        govtypes.ModuleName,
		Short:                      "Governance transactions subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmdSubmitLegacy := newGovSubmitLegacyProposalCmd()
	govTxCmd.AddCommand(
		newGovDepositCmd(),
		newGovVoteCmd(),
		newGovWeightedVoteCmd(),
		newGovSubmitProposalCmd(),
		govcli.NewCmdDraftProposal(),
		newGovCancelProposalCmd(),
		cmdSubmitLegacy,
	)

	return govTxCmd
}

func newGovSubmitProposalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-proposal [path/to/proposal.json]",
		Short: "Submit a proposal along with some messages, metadata and deposit",
		Args:  cobra.ExactArgs(1),
		Long: strings.TrimSpace(
			"Submit a proposal along with some messages, metadata and deposit.\n" +
				"They should be defined in a JSON file.\n\n" +
				"Example:\n" +
				"$ " + version.AppName + " tx gov submit-proposal path/to/proposal.json\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			proposal, msgs, deposit, err := parseGovSubmitProposal(clientCtx.Codec, args[0])
			if err != nil {
				return err
			}

			msg, err := govv1.NewMsgSubmitProposal(
				msgs,
				deposit,
				clientCtx.GetFromAddress().String(),
				proposal.Metadata,
				proposal.Title,
				proposal.Summary,
				proposal.Expedited,
			)
			if err != nil {
				return fmt.Errorf("invalid message: %w", err)
			}

			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGovCancelProposalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cancel-proposal [proposal-id]",
		Short:   "Cancel governance proposal before the voting period ends. Must be signed by the proposal creator.",
		Args:    cobra.ExactArgs(1),
		Example: "$ " + version.AppName + " tx gov cancel-proposal 1 --from mykey",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			proposalID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("proposal-id %s not a valid uint, please input a valid proposal-id", args[0])
			}

			msg := govv1.NewMsgCancelProposal(proposalID, clientCtx.GetFromAddress().String())
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGovSubmitLegacyProposalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-legacy-proposal",
		Short: "Submit a legacy proposal along with an initial deposit",
		Long: strings.TrimSpace(`Submit a legacy proposal along with an initial deposit.
	Proposal title, description, type and deposit can be given directly or through a proposal JSON file.
	`),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			proposal, err := parseGovSubmitLegacyProposal(cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to parse proposal: %w", err)
			}

			amount, err := sdk.ParseCoinsNormalized(proposal.Deposit)
			if err != nil {
				return err
			}

			content, ok := govv1beta1.ContentFromProposalType(proposal.Title, proposal.Description, proposal.Type)
			if !ok {
				return fmt.Errorf("failed to create proposal content: unknown proposal type %s", proposal.Type)
			}

			msg, err := govv1beta1.NewMsgSubmitProposal(content, amount, clientCtx.GetFromAddress())
			if err != nil {
				return fmt.Errorf("invalid message: %w", err)
			}

			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String(govFlagTitle, "", "The proposal title")
	cmd.Flags().String(govFlagDescription, "", "The proposal description")
	cmd.Flags().String(govFlagProposalType, "", "The proposal Type")
	cmd.Flags().String(govFlagDeposit, "", "The proposal deposit")
	cmd.Flags().String(govFlagProposal, "", "Proposal file path (if provided other flags are ignored)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGovDepositCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deposit [proposal-id] [deposit]",
		Args:  cobra.ExactArgs(2),
		Short: "Deposit tokens for an active proposal",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			proposalID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("proposal-id %s not a valid uint, please input a valid proposal-id", args[0])
			}

			amount, err := sdk.ParseCoinsNormalized(args[1])
			if err != nil {
				return err
			}

			msg := govv1.NewMsgDeposit(clientCtx.GetFromAddress(), proposalID, amount)
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGovVoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vote [proposal-id] [option]",
		Args:  cobra.ExactArgs(2),
		Short: "Vote for an active proposal, options: yes/no/no_with_veto/abstain",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			proposalID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("proposal-id %s not a valid int, please input a valid proposal-id", args[0])
			}

			byteVoteOption, err := govv1.VoteOptionFromString(govutils.NormalizeVoteOption(args[1]))
			if err != nil {
				return err
			}

			metadata, err := cmd.Flags().GetString(govFlagMetadata)
			if err != nil {
				return err
			}

			msg := govv1.NewMsgVote(clientCtx.GetFromAddress(), proposalID, byteVoteOption, metadata)
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String(govFlagMetadata, "", "Specify metadata of the vote")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGovWeightedVoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "weighted-vote [proposal-id] [weighted-options]",
		Args:  cobra.ExactArgs(2),
		Short: "Vote for an active proposal with weights, e.g. yes=0.6,no=0.4",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			proposalID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("proposal-id %s not a valid int, please input a valid proposal-id", args[0])
			}

			options, err := govv1.WeightedVoteOptionsFromString(govutils.NormalizeWeightedVoteOptions(args[1]))
			if err != nil {
				return err
			}

			metadata, err := cmd.Flags().GetString(govFlagMetadata)
			if err != nil {
				return err
			}

			msg := govv1.NewMsgVoteWeighted(clientCtx.GetFromAddress(), proposalID, options, metadata)
			return pqctxext.GenerateOrBroadcastTxCLI(cmd, clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String(govFlagMetadata, "", "Specify metadata of the weighted vote")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// --- Helpers ----------------------------------------------------------------

type govLegacyProposal struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Deposit     string `json:"deposit"`
}

func (p govLegacyProposal) validate() error {
	if strings.TrimSpace(p.Type) == "" {
		return fmt.Errorf("proposal type is required")
	}
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("proposal title is required")
	}
	if strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("proposal description is required")
	}
	return nil
}

func parseGovSubmitLegacyProposal(fs *pflag.FlagSet) (*govLegacyProposal, error) {
	proposal := &govLegacyProposal{}
	proposalFile, _ := fs.GetString(govFlagProposal)

	if proposalFile == "" {
		proposalType, _ := fs.GetString(govFlagProposalType)
		proposal.Title, _ = fs.GetString(govFlagTitle)
		proposal.Description, _ = fs.GetString(govFlagDescription)
		proposal.Type = govutils.NormalizeProposalType(proposalType)
		proposal.Deposit, _ = fs.GetString(govFlagDeposit)
		if err := proposal.validate(); err != nil {
			return nil, err
		}
		return proposal, nil
	}

	for _, flag := range govProposalFlags {
		if v, _ := fs.GetString(flag); v != "" {
			return nil, fmt.Errorf("--%s flag provided alongside --proposal, which is a noop", flag)
		}
	}

	contents, err := os.ReadFile(proposalFile)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(contents, proposal); err != nil {
		return nil, err
	}

	if err := proposal.validate(); err != nil {
		return nil, err
	}
	return proposal, nil
}

type govProposal struct {
	Messages  []json.RawMessage `json:"messages,omitempty"`
	Metadata  string            `json:"metadata"`
	Deposit   string            `json:"deposit"`
	Title     string            `json:"title"`
	Summary   string            `json:"summary"`
	Expedited bool              `json:"expedited"`
}

func parseGovSubmitProposal(cdc codec.Codec, path string) (govProposal, []sdk.Msg, sdk.Coins, error) {
	var proposal govProposal

	contents, err := os.ReadFile(path)
	if err != nil {
		return proposal, nil, nil, err
	}

	if err := json.Unmarshal(contents, &proposal); err != nil {
		return proposal, nil, nil, err
	}

	msgs := make([]sdk.Msg, len(proposal.Messages))
	for i, anyJSON := range proposal.Messages {
		var msg sdk.Msg
		if err := cdc.UnmarshalInterfaceJSON(anyJSON, &msg); err != nil {
			return proposal, nil, nil, err
		}
		msgs[i] = msg
	}

	deposit, err := sdk.ParseCoinsNormalized(proposal.Deposit)
	if err != nil {
		return proposal, nil, nil, err
	}

	return proposal, msgs, deposit, nil
}

func patchGovTxCommand(root *cobra.Command) {
	txCmd, _, err := root.Find([]string{"tx"})
	if err != nil || txCmd == nil {
		return
	}
	for _, c := range append([]*cobra.Command(nil), txCmd.Commands()...) {
		if c.Name() == "gov" {
			txCmd.RemoveCommand(c)
		}
	}
	txCmd.AddCommand(newGovTxCmd())
}
