package cmd

import "testing"

func TestDNSPowIdentifierFromArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		raw     bool
		want    string
		wantErr bool
	}{
		{name: "domain ext", args: []string{"Example", "LMN"}, want: "example.lmn"},
		{name: "fqdn single arg", args: []string{"Example.LMN"}, want: "example.lmn"},
		{name: "raw identifier", args: []string{"Mixed.Index"}, raw: true, want: "Mixed.Index"},
		{name: "plain identifier", args: []string{"custom-index"}, want: "custom-index"},
		{name: "invalid fqdn", args: []string{"bad", "1"}, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := dnsPowIdentifierFromArgs(tc.args, tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("dnsPowIdentifierFromArgs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("dnsPowIdentifierFromArgs() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("dnsPowIdentifierFromArgs() = %q, want %q", got, tc.want)
			}
		})
	}
}
