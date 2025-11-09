package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "pqc"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

const (
	SchemeDilithium3     = "dilithium3"
	PQCSignaturesTypeURL = "/lumen.pqc.v1.PQCSignatures"
	PQCSignDocPrefix     = "PQCv1:"
)

var (
	ParamsKey        = collections.NewPrefix("pqc_params")
	AccountKeyPrefix = collections.NewPrefix("pqc_account")
)
