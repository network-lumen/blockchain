package simulation

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"

	"lumen/x/dns/keeper"
	"lumen/x/dns/types"
)

func pickRandomDomain(ctx sdk.Context, k keeper.Keeper, ak types.AuthKeeper, accs []simtypes.Account) (types.Domain, simtypes.Account, bool, error) {
	var (
		selected types.Domain
		account  simtypes.Account
	)
	err := k.Domain.Walk(ctx, nil, func(_ string, dom types.Domain) (bool, error) {
		addrBz, err := ak.AddressCodec().StringToBytes(dom.Owner)
		if err != nil {
			return false, err
		}
		if acc, found := simtypes.FindAccount(accs, sdk.AccAddress(addrBz)); found {
			selected = dom
			account = acc
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return types.Domain{}, simtypes.Account{}, false, err
	}
	if account.Address.Empty() {
		return types.Domain{}, simtypes.Account{}, false, nil
	}
	return selected, account, true, nil
}

func splitFQDN(name string) (string, string, bool) {
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return "", "", false
	}
	ext := parts[len(parts)-1]
	domain := strings.Join(parts[:len(parts)-1], ".")
	return domain, ext, true
}

func mineNonce(identifier, creator string, difficulty uint32) uint64 {
	if difficulty == 0 {
		return 0
	}
	if difficulty > 20 {
		difficulty = 20
	}
	var nonce uint64
	for {
		payload := fmt.Sprintf("%s|%s|%d", identifier, creator, nonce)
		sum := sha256.Sum256([]byte(payload))
		if leadingZeroBits(sum[:]) >= difficulty {
			return nonce
		}
		nonce++
	}
}

func normalizeFQDN(domain, ext string) string {
	d, e := types.NormalizeDomainParts(domain, ext)
	return d + "." + e
}

func randomTXTRecords(r *rand.Rand) []*types.Record {
	return []*types.Record{
		{Key: "txt", Value: fmt.Sprintf("sim-%d", r.Int63()), Ttl: 60},
	}
}

func leadingZeroBits(b []byte) uint32 {
	var count uint32
	for _, v := range b {
		if v == 0 {
			count += 8
			continue
		}
		for i := 7; i >= 0; i-- {
			if (v>>uint(i))&1 == 0 {
				count++
			} else {
				return count
			}
		}
	}
	return count
}
