package types

import "fmt"

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:      func() *Params { p := DefaultParams(); return &p }(),
		Releases:    []*Release{},
		BundleCount: 0,
	}
}

func (gs GenesisState) Validate() error {
	idMap := make(map[uint64]bool)
	for _, r := range gs.Releases {
		if _, ok := idMap[r.Id]; ok {
			return fmt.Errorf("duplicated id for release")
		}
		idMap[r.Id] = true
	}
	if gs.Params != nil {
		return gs.Params.Validate()
	}
	return nil
}
