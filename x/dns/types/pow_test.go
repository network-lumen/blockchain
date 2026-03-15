package types

import "testing"

func TestMineUpdatePowNonce(t *testing.T) {
	nonce, err := MineUpdatePowNonce("example.lmn", "lmn1creator", 12)
	if err != nil {
		t.Fatalf("MineUpdatePowNonce() error = %v", err)
	}

	digest := ComputeUpdatePowDigest("example.lmn", "lmn1creator", nonce)
	if got := LeadingZeroBits(digest[:]); got < 12 {
		t.Fatalf("LeadingZeroBits() = %d, want >= 12", got)
	}
}

func TestMineUpdatePowNonceZeroDifficulty(t *testing.T) {
	nonce, err := MineUpdatePowNonce("example.lmn", "lmn1creator", 0)
	if err != nil {
		t.Fatalf("MineUpdatePowNonce() error = %v", err)
	}
	if nonce != 0 {
		t.Fatalf("MineUpdatePowNonce() = %d, want 0", nonce)
	}
}
