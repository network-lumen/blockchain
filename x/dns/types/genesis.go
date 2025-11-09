package types

import "fmt"

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:     DefaultParams(),
		DomainMap:  []Domain{},
		AuctionMap: []Auction{},
	}
}

func (gs GenesisState) Validate() error {
	domainIndexMap := make(map[string]struct{})
	for _, elem := range gs.DomainMap {
		index := fmt.Sprint(elem.Index)
		if _, ok := domainIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for domain")
		}
		domainIndexMap[index] = struct{}{}
	}

	auctionIndexMap := make(map[string]struct{})
	for _, elem := range gs.AuctionMap {
		index := fmt.Sprint(elem.Index)
		if _, ok := auctionIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for auction")
		}
		auctionIndexMap[index] = struct{}{}
	}

	return gs.Params.Validate()
}
