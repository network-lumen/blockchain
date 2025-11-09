//go:build !pqc_oqs

package dilithium

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCirclSchemeRoundTrip(t *testing.T) {
	scheme, err := newCirclScheme()
	if err != nil {
		t.Skipf("circl backend unavailable: %v", err)
	}

	ms, ok := scheme.(*modeScheme)
	if !ok {
		t.Fatalf("unexpected scheme type %T", scheme)
	}

	seed := bytes.Repeat([]byte{0x42}, ms.scheme.SeedSize())
	pub, priv, err := scheme.GenerateKey(seed)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	if len(pub) != scheme.PublicKeySize() {
		t.Fatalf("unexpected public key size: got %d want %d", len(pub), scheme.PublicKeySize())
	}
	if len(priv) != ms.scheme.PrivateKeySize() {
		t.Fatalf("unexpected private key size: got %d want %d", len(priv), ms.scheme.PrivateKeySize())
	}

	msg := []byte("circl backend sign/verify test")
	sig, err := scheme.Sign(priv, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if len(sig) != scheme.SignatureSize() {
		t.Fatalf("unexpected signature size: got %d want %d", len(sig), scheme.SignatureSize())
	}
	if ok := scheme.Verify(pub, msg, sig); !ok {
		t.Fatal("Verify rejected valid signature")
	}

	badSig := append([]byte{}, sig...)
	badSig[0] ^= 0xFF
	if ok := scheme.Verify(pub, msg, badSig); ok {
		t.Fatal("Verify accepted tampered signature")
	}

	if _, err := scheme.Sign(PrivateKey(priv[:len(priv)-1]), msg); err == nil {
		t.Fatal("expected error for truncated private key")
	}
	if ok := scheme.Verify(PublicKey(pub[:len(pub)-1]), msg, sig); ok {
		t.Fatal("expected failure for truncated public key")
	}
	if ok := scheme.Verify(pub, msg, Signature(sig[:len(sig)-1])); ok {
		t.Fatal("expected failure for truncated signature")
	}
}

func TestDefaultPrefersCircl(t *testing.T) {
	scheme := Default()
	ms, ok := scheme.(*modeScheme)
	if !ok {
		t.Fatalf("expected circl modeScheme, got %T", scheme)
	}
	require.Equal(t, algoDilithium3, ms.algoID)
}
