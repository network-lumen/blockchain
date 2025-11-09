package types

import "fmt"

func DefaultGenesis() *GenesisState {
	def := DefaultParams()
	return &GenesisState{
		Params:        &def,
		Gateways:      []*Gateway{},
		Contracts:     []*Contract{},
		GatewayCount:  0,
		ContractCount: 0,
	}
}

func ValidateGenesis(gs *GenesisState) error {
	if gs == nil {
		return fmt.Errorf("genesis state cannot be nil")
	}
	seenGw := make(map[uint64]struct{})
	for _, g := range gs.Gateways {
		if _, ok := seenGw[g.Id]; ok {
			return fmt.Errorf("duplicate gateway id %d", g.Id)
		}
		seenGw[g.Id] = struct{}{}
	}

	seenCt := make(map[uint64]struct{})
	for _, c := range gs.Contracts {
		if _, ok := seenCt[c.Id]; ok {
			return fmt.Errorf("duplicate contract id %d", c.Id)
		}
		seenCt[c.Id] = struct{}{}
	}

	if gs.Params != nil {
		if err := ValidateParams(*gs.Params); err != nil {
			return err
		}
	}

	return nil
}
