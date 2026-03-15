package types

import (
	"crypto/sha256"
	"fmt"
)

// ComputeUpdatePowDigest returns sha256(identifier || "|" || creator || "|" || nonce).
func ComputeUpdatePowDigest(identifier, creator string, nonce uint64) [sha256.Size]byte {
	payload := fmt.Sprintf("%s|%s|%d", identifier, creator, nonce)
	return sha256.Sum256([]byte(payload))
}

// LeadingZeroBits counts the number of most-significant zero bits in the provided hash.
func LeadingZeroBits(bz []byte) uint32 {
	var count uint32
	for _, b := range bz {
		if b == 0 {
			count += 8
			continue
		}
		for i := 7; i >= 0; i-- {
			if (b>>uint(i))&0x1 == 0 {
				count++
			} else {
				return count
			}
		}
		return count
	}
	return count
}

// MineUpdatePowNonce returns the first nonce satisfying the DNS update PoW target.
func MineUpdatePowNonce(identifier, creator string, difficulty uint32) (uint64, error) {
	if difficulty == 0 {
		return 0, nil
	}

	var nonce uint64
	for {
		digest := ComputeUpdatePowDigest(identifier, creator, nonce)
		if LeadingZeroBits(digest[:]) >= difficulty {
			return nonce, nil
		}
		nonce++
		if nonce == 0 {
			return 0, fmt.Errorf("pow search exhausted")
		}
	}
}
