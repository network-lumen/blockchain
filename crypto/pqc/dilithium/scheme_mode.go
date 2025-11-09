package dilithium

import (
	"errors"
	"fmt"

	"github.com/cloudflare/circl/sign"
)

type modeScheme struct {
	scheme sign.Scheme
	algoID string
}

func newModeScheme(scheme sign.Scheme, algo string) (Scheme, error) {
	if scheme == nil {
		return nil, errors.New("dilithium: scheme unavailable")
	}
	return &modeScheme{scheme: scheme, algoID: algo}, nil
}

func (s *modeScheme) Name() string {
	return s.algoID
}

func (s *modeScheme) PublicKeySize() int {
	return s.scheme.PublicKeySize()
}

func (s *modeScheme) SignatureSize() int {
	return s.scheme.SignatureSize()
}

func (s *modeScheme) GenerateKey(seed []byte) (PublicKey, PrivateKey, error) {
	var (
		pk  sign.PublicKey
		sk  sign.PrivateKey
		err error
	)

	switch len(seed) {
	case 0:
		pk, sk, err = s.scheme.GenerateKey()
		if err != nil {
			return nil, nil, fmt.Errorf("dilithium: generate key: %w", err)
		}
	case s.scheme.SeedSize():
		seedCopy := make([]byte, len(seed))
		copy(seedCopy, seed)
		pk, sk = s.scheme.DeriveKey(seedCopy)
		wipe(seedCopy)
	default:
		return nil, nil, fmt.Errorf("dilithium: seed must be %d bytes", s.scheme.SeedSize())
	}

	pubBytes, err := pk.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("dilithium: marshal public key: %w", err)
	}
	privBytes, err := sk.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("dilithium: marshal private key: %w", err)
	}

	pubOut := make([]byte, len(pubBytes))
	copy(pubOut, pubBytes)
	privOut := make([]byte, len(privBytes))
	copy(privOut, privBytes)

	return PublicKey(pubOut), PrivateKey(privOut), nil
}

func (s *modeScheme) Sign(priv PrivateKey, msg []byte) (Signature, error) {
	if len(priv) != s.scheme.PrivateKeySize() {
		return nil, fmt.Errorf("dilithium: private key must be %d bytes", s.scheme.PrivateKeySize())
	}

	sk, err := s.scheme.UnmarshalBinaryPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("dilithium: invalid private key: %w", err)
	}

	sigBytes := s.scheme.Sign(sk, msg, nil)
	out := make([]byte, len(sigBytes))
	copy(out, sigBytes)
	return Signature(out), nil
}

func (s *modeScheme) Verify(pub PublicKey, msg []byte, sig Signature) bool {
	if len(pub) != s.scheme.PublicKeySize() {
		return false
	}
	if len(sig) != s.scheme.SignatureSize() {
		return false
	}

	pk, err := s.scheme.UnmarshalBinaryPublicKey(pub)
	if err != nil {
		return false
	}
	return s.scheme.Verify(pk, msg, sig, nil)
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
