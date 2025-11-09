package app

import (
	"io"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func TestOnlySetPQCDebug(enabled bool) {
	pqcDebugFlag.Store(enabled)
}

func TestOnlySwapPQCDebugWriter(w io.Writer) (restore func()) {
	pqcDebugMu.Lock()
	prev := pqcDebugWriter
	pqcDebugWriter = w
	pqcDebugMu.Unlock()
	return func() {
		pqcDebugMu.Lock()
		pqcDebugWriter = prev
		pqcDebugMu.Unlock()
	}
}

func TestOnlyEmitPQCDebugLog(txHash []byte, signer sdk.AccAddress, status, reason string) {
	pqcDebugLogSigner(txHash, signer, status, reason)
}
