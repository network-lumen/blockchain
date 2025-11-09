package types

const (
	EventTypeLinkAccount  = "pqc.link_account"
	EventTypeVerified     = "pqc.verified"
	EventTypeMissing      = "pqc.missing"
	EventTypeVerifyFailed = "pqc.verify_failed"

	AttributeKeyAddress    = "address"
	AttributeKeyScheme     = "scheme"
	AttributeKeyPubKeyHash = "pubkey_hash"
	AttributeKeyReason     = "reason"
)
