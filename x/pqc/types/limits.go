package types

const (
	// PQCSchemeMaxLen keeps scheme identifiers short to avoid memspam.
	PQCSchemeMaxLen = 32
	// PQCPubKeyMaxLen caps Dilithium public key blobs accepted via CLI.
	PQCPubKeyMaxLen = 4096
)
