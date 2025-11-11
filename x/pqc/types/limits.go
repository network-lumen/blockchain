package types

const (
	// PQCSchemeMaxLen keeps scheme identifiers short to avoid memspam.
	PQCSchemeMaxLen = 32
	// PQCPubKeyMaxLen caps Dilithium public key blobs accepted via CLI.
	PQCPubKeyMaxLen = 4096
	// PQCPowNonceMaxLen limits client-provided PoW nonces to a sane size.
	PQCPowNonceMaxLen = 128
)
