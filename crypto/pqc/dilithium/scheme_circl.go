//go:build !pqc_oqs

package dilithium

import (
	"fmt"

	dilithium3 "github.com/cloudflare/circl/sign/dilithium/mode3"
)

func newCirclScheme() (Scheme, error) {
	sch := dilithium3.Scheme()
	if sch == nil {
		return nil, fmt.Errorf("dilithium: circl scheme unavailable")
	}
	setActiveBackend(BackendDilithium3Circl)
	return newModeScheme(sch, algoDilithium3)
}
