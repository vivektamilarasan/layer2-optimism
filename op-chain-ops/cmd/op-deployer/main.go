package main

import (
	"fmt"
	"github.com/ethereum-optimism/optimism/op-chain-ops/configurator"
	"github.com/ethereum-optimism/optimism/op-service/cliapp"
	"github.com/urfave/cli/v2"
	"os"
)

func main() {
	app := cli.NewApp()
	app.Name = "op-deployer"
	app.Usage = "Tool to configure and deploy OP Chains."
	app.Flags = cliapp.ProtectFlags([]cli.Flag{})
	app.Commands = []*cli.Command{
		{
			Name:  "configure",
			Usage: "generate a deploy config",
			Flags: cliapp.ProtectFlags([]cli.Flag{
				&cli.StringFlag{
					Name:  configurator.GenDeployConfigInfileFlagName,
					Usage: "input configuration file",
					EnvVars: []string{
						"DEPLOYER_INFILE",
					},
				},
				&cli.StringFlag{
					Name:  configurator.GenDeployConfigOutfileFlagName,
					Usage: "output configuration file",
					EnvVars: []string{
						"OUTFILE",
					},
				},
				&cli.StringFlag{
					Name:  configurator.GenDeployConfigMnemonicFlagName,
					Usage: "mnemonic for account generation",
					EnvVars: []string{
						"MNEMONIC",
					},
				},
				&cli.StringFlag{
					Name:  configurator.GenDeployConfigL1RPCURLFlagName,
					Usage: "L1 RPC URL",
					EnvVars: []string{
						"L1_RPC_URL",
					},
				},
			}),
			Action: configurator.GenDeployConfigCLI(),
		},
		{
			Name:  "deploy",
			Usage: "deploys a chain",
			Flags: cliapp.ProtectFlags([]cli.Flag{}),
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
