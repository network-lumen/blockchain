//go:build !pqc_oqs

package dilithium

// Default returns the Circl Dilithium3 backend when building without pqc_oqs.
func Default() Scheme {
	scheme, err := newCirclScheme()
	if err != nil {
		panic("pqc: circl backend unavailable: " + err.Error())
	}
	if scheme == nil {
		panic("pqc: circl backend unavailable: nil scheme")
	}
	return scheme
}
