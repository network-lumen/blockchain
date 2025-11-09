package txext

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cosmos/cosmos-sdk/client"
	clientflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/input"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
)

// GenerateOrBroadcastTxCLI constructs a tx factory from CLI flags and handles
// PQC injection before the standard signing pipeline.
func GenerateOrBroadcastTxCLI(
	cmd *cobra.Command,
	clientCtx client.Context,
	flagSet *pflag.FlagSet,
	msgs ...types.Msg,
) error {
	txf, err := clienttx.NewFactoryCLI(clientCtx, flagSet)
	if err != nil {
		return err
	}

	return GenerateOrBroadcastTxWithFactory(cmd, clientCtx, txf, msgs...)
}

// GenerateOrBroadcastTxWithFactory mirrors the upstream helper but ensures the
// PQC signatures are embedded before the standard signatures are produced.
func GenerateOrBroadcastTxWithFactory(
	cmd *cobra.Command,
	clientCtx client.Context,
	txf clienttx.Factory,
	msgs ...types.Msg,
) error {
	for _, msg := range msgs {
		if m, ok := msg.(types.HasValidateBasic); ok {
			if err := m.ValidateBasic(); err != nil {
				return err
			}
		}
	}

	if clientCtx.IsAux {
		auxSignerData, err := makeAuxSignerData(clientCtx, txf, msgs...)
		if err != nil {
			return err
		}
		return clientCtx.PrintProto(&auxSignerData)
	}

	if clientCtx.GenerateOnly {
		builder, err := txf.BuildUnsignedTx(msgs...)
		if err != nil {
			return err
		}

		encoder := clientCtx.TxConfig.TxJSONEncoder()
		if encoder == nil {
			return fmt.Errorf("cannot print unsigned tx: tx json encoder is nil")
		}

		jsonBz, err := encoder(builder.GetTx())
		if err != nil {
			return err
		}

		return clientCtx.PrintString(fmt.Sprintf("%s\n", jsonBz))
	}

	return broadcastTxWithPQC(cmd, clientCtx, txf, msgs...)
}

func broadcastTxWithPQC(
	cmd *cobra.Command,
	clientCtx client.Context,
	txf clienttx.Factory,
	msgs ...types.Msg,
) error {
	txf, err := txf.Prepare(clientCtx)
	if err != nil {
		return err
	}

	skipSimulation := shouldSkipGasEstimation(clientCtx, txf)

	if !skipSimulation && (txf.SimulateAndExecute() || clientCtx.Simulate) {
		if clientCtx.Offline {
			return fmt.Errorf("cannot estimate gas in offline mode")
		}

		_, adjusted, err := clienttx.CalculateGas(clientCtx, txf, msgs...)
		if err != nil {
			return err
		}

		txf = txf.WithGas(adjusted)
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", clienttx.GasEstimateResponse{GasEstimate: txf.Gas()})
	} else if skipSimulation && txf.Gas() == 0 {
		txf = txf.WithGas(clientflags.DefaultGasLimit)
	}

	if clientCtx.Simulate && !skipSimulation {
		return nil
	}

	txBuilder, err := txf.BuildUnsignedTx(msgs...)
	if err != nil {
		return err
	}

	if !clientCtx.SkipConfirm {
		encoder := clientCtx.TxConfig.TxJSONEncoder()
		if encoder == nil {
			return fmt.Errorf("failed to encode transaction: tx json encoder is nil")
		}

		txJSON, err := encoder(txBuilder.GetTx())
		if err != nil {
			return fmt.Errorf("failed to encode transaction: %w", err)
		}

		if err := clientCtx.PrintRaw(json.RawMessage(txJSON)); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n%s\n", err, txJSON)
		}

		reader := bufio.NewReader(os.Stdin)
		ok, err := input.GetConfirmation("confirm transaction before signing and broadcasting", reader, os.Stderr)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\ncanceled transaction\n", err)
			return err
		}
		if !ok {
			_, _ = fmt.Fprintln(os.Stderr, "canceled transaction")
			return nil
		}
	}

	if err := clienttx.Sign(cmd.Context(), txf, clientCtx.FromName, txBuilder, true); err != nil {
		return err
	}

	injected, err := InjectPQCPostSign(clientCtx, txBuilder)
	if err != nil {
		return err
	}
	if injected {
		if err := clienttx.Sign(cmd.Context(), txf, clientCtx.FromName, txBuilder, true); err != nil {
			return err
		}
	}

	txBytes, err := clientCtx.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return err
	}

	res, err := clientCtx.BroadcastTx(txBytes)
	if err != nil {
		return err
	}

	if res.Code != 0 {
		_ = clientCtx.PrintProto(res)
		return fmt.Errorf("transaction failed (code=%d): %s", res.Code, res.RawLog)
	}

	return clientCtx.PrintProto(res)
}

func makeAuxSignerData(clientCtx client.Context, f clienttx.Factory, msgs ...types.Msg) (txtypes.AuxSignerData, error) {
	builder := clienttx.NewAuxTxBuilder()
	fromAddress, name, _, err := client.GetFromFields(clientCtx, clientCtx.Keyring, clientCtx.From)
	if err != nil {
		return txtypes.AuxSignerData{}, err
	}

	builder.SetAddress(fromAddress.String())
	if clientCtx.Offline {
		builder.SetAccountNumber(f.AccountNumber())
		builder.SetSequence(f.Sequence())
	} else {
		accNum, seq, err := clientCtx.AccountRetriever.GetAccountNumberSequence(clientCtx, fromAddress)
		if err != nil {
			return txtypes.AuxSignerData{}, err
		}
		builder.SetAccountNumber(accNum)
		builder.SetSequence(seq)
	}

	if err := builder.SetMsgs(msgs...); err != nil {
		return txtypes.AuxSignerData{}, err
	}

	if err := builder.SetSignMode(f.SignMode()); err != nil {
		return txtypes.AuxSignerData{}, err
	}

	record, err := clientCtx.Keyring.Key(name)
	if err != nil {
		return txtypes.AuxSignerData{}, err
	}

	pub, err := record.GetPubKey()
	if err != nil {
		return txtypes.AuxSignerData{}, err
	}

	if err := builder.SetPubKey(pub); err != nil {
		return txtypes.AuxSignerData{}, err
	}

	builder.SetChainID(clientCtx.ChainID)
	signBz, err := builder.GetSignBytes()
	if err != nil {
		return txtypes.AuxSignerData{}, err
	}

	sig, _, err := clientCtx.Keyring.Sign(name, signBz, f.SignMode())
	if err != nil {
		return txtypes.AuxSignerData{}, err
	}
	builder.SetSignature(sig)

	return builder.GetAuxSignerData()
}

func shouldSkipGasEstimation(clientCtx client.Context, txf clienttx.Factory) bool {
	if clientCtx.Simulate {
		return false
	}
	if txf.SimulateAndExecute() {
		return false
	}
	if !txf.Fees().IsZero() {
		return false
	}
	if gp := txf.GasPrices(); gp != nil && !gp.IsZero() {
		return false
	}
	return true
}
