package main

import (
	"os"

	"github.com/patiparnphot/simple-utxos-blockchain/cli"
)

func main() {
	defer os.Exit(0)
	cmd := cli.CommandLine{}
	cmd.Run()

}
