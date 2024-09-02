package deployer

import (
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/urfave/cli/v2"
)

type ConfigureCMDConfig struct {
	L1RPCURL string
	Infile   string
	Outfile  string
	Mnemonic string
}

func (c *ConfigureCMDConfig) Check() error {
	if c.L1RPCURL == "" {
		return fmt.Errorf("l1-rpc-url must be specified")
	}

	if c.Infile == "" {
		return fmt.Errorf("infile must be specified")
	}

	if c.Outfile == "" {
		return fmt.Errorf("outfile must be specified")
	}

	if c.Mnemonic == "" {
		return fmt.Errorf("mnemonic must be specified")
	}

	return nil
}

func ConfigureCLI() func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		infile := ctx.String(InfileFlagName)
		outfile := ctx.String(OutfileFlagName)
		mnemonic := ctx.String(ConfigureMnemonicFlagName)
		l1RPCURL := ctx.String(L1RPCURLFlagName)

		if outfile == "" {
			outfile = infile
		}

		return Configure(ConfigureCMDConfig{
			L1RPCURL: l1RPCURL,
			Infile:   infile,
			Outfile:  outfile,
			Mnemonic: mnemonic,
		})
	}
}

func Configure(config ConfigureCMDConfig) error {
	l1Client, err := ethclient.Dial(config.L1RPCURL)
	if err != nil {
		return fmt.Errorf("failed to connect to L1 RPC: %w", err)
	}

	state, err := ReadDeploymentState(config.Infile)
	if err != nil {
		return fmt.Errorf("failed to read chain intent: %w", err)
	}

	if state.Intent == nil {
		return fmt.Errorf("chain intent is nil")
	}

	keygen, err := NewMnemonicKeyGenerator(config.Mnemonic)
	if err != nil {
		return fmt.Errorf("failed to create key generator: %w", err)
	}

	deployConfig, err := NewDeployConfig(keygen, l1Client, state.Intent)
	if err != nil {
		return fmt.Errorf("failed to create deploy config: %w", err)
	}

	return WriteDeploymentState(
		config.Outfile,
		&DeploymentState{
			Intent:       state.Intent,
			DeployConfig: deployConfig,
		},
	)
}
