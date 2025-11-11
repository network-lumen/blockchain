package types

import "crypto/sha256"

// ComputePowDigest returns sha256(pubKey || nonce).
func ComputePowDigest(pubKey, nonce []byte) [sha256.Size]byte {
	payload := make([]byte, 0, len(pubKey)+len(nonce))
	payload = append(payload, pubKey...)
	payload = append(payload, nonce...)
	return sha256.Sum256(payload)
}

// LeadingZeroBits counts the number of most-significant zero bits in the provided hash.
func LeadingZeroBits(bz []byte) int {
	count := 0
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
