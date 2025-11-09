package dilithium

// PublicKey represents a Dilithium public key.
type PublicKey []byte

// PrivateKey represents a Dilithium private key (seed material in the test backend).
type PrivateKey []byte

// Signature represents a Dilithium signature.
type Signature []byte

const (
	algoDilithium3           = "dilithium3"
	BackendDilithium3Circl   = "dilithium3-circl"
	BackendDilithium3PQClean = "dilithium3-pqclean"
)

var activeBackend = "unknown"

func ActiveBackend() string { return activeBackend }

func setActiveBackend(name string) {
	activeBackend = name
}

// Scheme defines the minimal interface implemented by Dilithium backends.
type Scheme interface {
	// Name returns the scheme identifier (e.g. "dilithium3").
	Name() string
	// PublicKeySize returns the expected public key length in bytes.
	PublicKeySize() int
	// SignatureSize returns the expected signature length in bytes.
	SignatureSize() int

	// Optional helpers used in tests.
	GenerateKey(seed []byte) (PublicKey, PrivateKey, error)
	Sign(priv PrivateKey, msg []byte) (Signature, error)
	Verify(pub PublicKey, msg []byte, sig Signature) bool
}
