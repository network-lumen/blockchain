package signing

import (
	"fmt"

	txv1beta1 "cosmossdk.io/api/cosmos/tx/v1beta1"
	errorsmod "cosmossdk.io/errors"
	"google.golang.org/protobuf/proto"

	pqctypes "lumen/x/pqc/types"
)

// ComputeSignBytes returns the prefixed SignDoc bytes used for PQC signatures.
func ComputeSignBytes(chainID string, accountNumber uint64, txRaw *txv1beta1.TxRaw) ([]byte, error) {
	if txRaw == nil {
		return nil, fmt.Errorf("tx raw cannot be nil")
	}
	if len(txRaw.BodyBytes) == 0 || len(txRaw.AuthInfoBytes) == 0 {
		return nil, fmt.Errorf("tx raw missing body/auth info")
	}

	bodyBytes, err := SanitizeBodyBytes(txRaw.BodyBytes)
	if err != nil {
		return nil, err
	}

	signDoc := txv1beta1.SignDoc{
		BodyBytes:     bodyBytes,
		AuthInfoBytes: txRaw.AuthInfoBytes,
		ChainId:       chainID,
		AccountNumber: accountNumber,
	}
	docBytes, err := proto.Marshal(&signDoc)
	if err != nil {
		return nil, errorsmod.Wrap(err, "pqc: failed to marshal sign doc")
	}

	payload := make([]byte, len(pqctypes.PQCSignDocPrefix)+len(docBytes))
	copy(payload, pqctypes.PQCSignDocPrefix)
	copy(payload[len(pqctypes.PQCSignDocPrefix):], docBytes)
	return payload, nil
}

// SanitizeBodyBytes removes PQC signature extension options before signing.
func SanitizeBodyBytes(bodyBytes []byte) ([]byte, error) {
	if len(bodyBytes) == 0 {
		return nil, fmt.Errorf("tx body bytes empty")
	}

	var body txv1beta1.TxBody
	if err := proto.Unmarshal(bodyBytes, &body); err != nil {
		return nil, errorsmod.Wrap(err, "decode tx body")
	}

	filtered := body.ExtensionOptions[:0]
	for _, option := range body.ExtensionOptions {
		if option.GetTypeUrl() == pqctypes.PQCSignaturesTypeURL {
			continue
		}
		filtered = append(filtered, option)
	}
	body.ExtensionOptions = filtered

	filteredNC := body.NonCriticalExtensionOptions[:0]
	for _, option := range body.NonCriticalExtensionOptions {
		if option.GetTypeUrl() == pqctypes.PQCSignaturesTypeURL {
			continue
		}
		filteredNC = append(filteredNC, option)
	}
	body.NonCriticalExtensionOptions = filteredNC

	sanitized, err := proto.Marshal(&body)
	if err != nil {
		return nil, errorsmod.Wrap(err, "marshal sanitized body")
	}
	return sanitized, nil
}
