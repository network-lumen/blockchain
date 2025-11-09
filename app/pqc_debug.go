package app

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	pqcDebugFlag   atomic.Bool
	pqcDebugWriter io.Writer = os.Stdout
	pqcDebugMu     sync.RWMutex
)

func init() {
	if os.Getenv("LUMEN_PQC_DEBUG") == "1" {
		pqcDebugFlag.Store(true)
	}
}

func pqcDebugEnabled() bool {
	return pqcDebugFlag.Load()
}

func pqcDebugLogSigner(txHash []byte, signer sdk.AccAddress, status, reason string) {
	if !pqcDebugEnabled() || signer == nil {
		return
	}

	line := fmt.Sprintf("[pqc-ante] signer=%s tx_hash=%s status=%s", signer.String(), shortHex(txHash), status)
	if reason != "" {
		line += " reason=" + reason
	}

	pqcDebugMu.RLock()
	writer := pqcDebugWriter
	pqcDebugMu.RUnlock()
	fmt.Fprintln(writer, line)
}

func shortHex(bz []byte) string {
	if len(bz) == 0 {
		return "n/a"
	}
	hexStr := strings.ToUpper(hex.EncodeToString(bz))
	if len(hexStr) > 16 {
		return hexStr[:16]
	}
	return hexStr
}
