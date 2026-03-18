package cmd

import "testing"

func TestRootCmdIncludesIBCCommands(t *testing.T) {
	root := NewRootCmd()

	tests := []struct {
		name string
		path []string
	}{
		{name: "ibc tx root", path: []string{"tx", "ibc"}},
		{name: "ibc client tx root", path: []string{"tx", "ibc", "client"}},
		{name: "ibc channelv2 tx root", path: []string{"tx", "ibc", "channelv2"}},
		{name: "ibc client delete client creator tx", path: []string{"tx", "ibc", "client", "delete-client-creator"}},
		{name: "ibc query root", path: []string{"query", "ibc"}},
		{name: "ibc client query root", path: []string{"query", "ibc", "client"}},
		{name: "ibc connection query root", path: []string{"query", "ibc", "connection"}},
		{name: "ibc channel query root", path: []string{"query", "ibc", "channel"}},
		{name: "ibc channelv2 query root", path: []string{"query", "ibc", "channelv2"}},
		{name: "ibc transfer query root", path: []string{"query", "ibc-transfer"}},
		{name: "ibc transfer params", path: []string{"query", "ibc-transfer", "params"}},
		{name: "ibc transfer escrow address", path: []string{"query", "ibc-transfer", "escrow-address"}},
		{name: "ibc transfer denom hash", path: []string{"query", "ibc-transfer", "denom-hash"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cmd, _, err := root.Find(tc.path)
			if err != nil {
				t.Fatalf("root.Find(%v) error = %v", tc.path, err)
			}
			if cmd == nil {
				t.Fatalf("root.Find(%v) returned nil command", tc.path)
			}
		})
	}
}
