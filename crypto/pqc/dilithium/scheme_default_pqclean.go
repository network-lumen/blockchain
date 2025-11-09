//go:build pqc_oqs

package dilithium

// Default returns the PQClean Dilithium3 backend when pqc_oqs is enabled.
func Default() Scheme {
	scheme, err := newPQCleanScheme()
	if err != nil {
		panic("pqc: pqclean backend unavailable: " + err.Error())
	}
	if scheme == nil {
		panic("pqc: pqclean backend unavailable: nil scheme")
	}
	return scheme
}
