//go:build pqc_oqs

package dilithium

import (
	"fmt"

	dilithium3 "github.com/cloudflare/circl/sign/dilithium/mode3"
)

func newPQCleanScheme() (Scheme, error) {
	sch := dilithium3.Scheme()
	if sch == nil {
		return nil, fmt.Errorf("dilithium: pqclean scheme unavailable")
	}
	setActiveBackend(BackendDilithium3PQClean)
	return newModeScheme(sch, algoDilithium3)
}
