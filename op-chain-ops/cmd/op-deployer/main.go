package main

import (
	"fmt"
	"github.com/ethereum-optimism/optimism/op-chain-ops/deployer"
	"github.com/ethereum-optimism/optimism/op-service/cliapp"
	"github.com/urfave/cli/v2"
	"os"
)

func main() {
	app := cli.NewApp()
	app.Name = "op-deployer"
	app.Usage = "Tool to configure and deploy OP Chains."
	app.Flags = cliapp.ProtectFlags(deployer.GlobalFlags)
	app.Commands = []*cli.Command{
		{
			Name:   "configure",
			Usage:  "generate a deploy config",
			Flags:  cliapp.ProtectFlags(deployer.ConfigureFlags),
			Action: deployer.ConfigureCLI(),
		},
		{
			Name:   "deploy",
			Usage:  "deploys a chain",
			Flags:  cliapp.ProtectFlags(deployer.DeployFlags),
			Action: deployer.DeployCLI(),
		},
		{
			Name:   "genesis",
			Usage:  "generate a genesis file for an L2 chain",
			Flags:  cliapp.ProtectFlags(deployer.GenesisFlags),
			Action: deployer.GenesisCLI(),
		},
	}
	app.Writer = os.Stdout
	app.ErrWriter = os.Stderr
	err := app.Run(os.Args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Application failed: %v\n", err)
		os.Exit(1)
	}
}
