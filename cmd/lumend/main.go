package main

import (
	"fmt"
	"os"

	clienthelpers "cosmossdk.io/client/v2/helpers"
	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"

	"lumen/app"
	"lumen/cmd/lumend/cmd"
	"lumen/crypto/pqc/dilithium"
)

func init() {
	_ = dilithium.Default()
	name := dilithium.ActiveBackend()
	switch name {
	case dilithium.BackendDilithium3Circl, dilithium.BackendDilithium3PQClean:
		// allowed backends
	default:
		panic("security: invalid PQC backend linked: " + name)
	}
}

func main() {
	rootCmd := cmd.NewRootCmd()
	if err := svrcmd.Execute(rootCmd, clienthelpers.EnvPrefix, app.DefaultNodeHome); err != nil {
		fmt.Fprintln(rootCmd.OutOrStderr(), err)
		os.Exit(1)
	}
}
