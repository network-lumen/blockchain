package types

const (
	// DNSLabelMaxLen follows RFC 1035 (63 octets per label).
	DNSLabelMaxLen = 63
	// DNSFQDNMaxLen caps name.ext (plus dot) to 255 octets.
	DNSFQDNMaxLen = 255
	// DNSRecordsMax caps the number of resource records per tx.
	DNSRecordsMax = 64
	// DNSRecordsPayloadMaxBytes caps combined key/value payload for a tx.
	DNSRecordsPayloadMaxBytes = 16 * 1024
	// MaxRegistrationDurationDays caps register/renew duration to 1 year.
	MaxRegistrationDurationDays uint64 = 365
)
