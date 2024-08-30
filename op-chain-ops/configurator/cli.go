package configurator

import (
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/urfave/cli/v2"
)

const (
	GenDeployConfigInfileFlagName   = "infile"
	GenDeployConfigOutfileFlagName  = "outfile"
	GenDeployConfigMnemonicFlagName = "mnemonic"
	GenDeployConfigL1RPCURLFlagName = "l1-rpc-url"
)

func GenDeployConfigCLI() func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		infile := ctx.String(GenDeployConfigInfileFlagName)
		outfile := ctx.String(GenDeployConfigOutfileFlagName)
		mnemonic := ctx.String(GenDeployConfigMnemonicFlagName)
		l1RPCURL := ctx.String(GenDeployConfigL1RPCURLFlagName)

		if infile == "" {
			return fmt.Errorf("infile must be specified")
		}

		if outfile == "" {
			outfile = infile
		}

		if mnemonic == "" {
			return fmt.Errorf("mnemonic must be specified")
		}

		if l1RPCURL == "" {
			return fmt.Errorf("l1-rpc-url must be specified")
		}

		l1Client, err := ethclient.Dial(l1RPCURL)
		if err != nil {
			return fmt.Errorf("failed to connect to L1 RPC: %w", err)
		}

		state, err := ReadDeploymentState(infile)
		if err != nil {
			return fmt.Errorf("failed to read chain intent: %w", err)
		}

		if state.Intent == nil {
			return fmt.Errorf("chain intent is nil")
		}

		keygen, err := NewMnemonicKeyGenerator(mnemonic)
		if err != nil {
			return fmt.Errorf("failed to create key generator: %w", err)
		}

		deployConfig, err := NewDeployConfig(keygen, l1Client, state.Intent)
		if err != nil {
			return fmt.Errorf("failed to create deploy config: %w", err)
		}

		return WriteDeploymentState(
			outfile,
			&DeploymentState{
				Intent:       state.Intent,
				DeployConfig: deployConfig,
			},
		)
	}
}
