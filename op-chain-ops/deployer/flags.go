package deployer

import (
	op_service "github.com/ethereum-optimism/optimism/op-service"
	"github.com/urfave/cli/v2"
)

const (
	EnvVarPrefix              = "DEPLOYER"
	L1RPCURLFlagName          = "l1-rpc-url"
	InfileFlagName            = "infile"
	OutfileFlagName           = "outfile"
	ConfigureMnemonicFlagName = "mnemonic"
	ContractsImageFlagName    = "image"
	DeployPrivateKeyFlagName  = "private-key"
)

var (
	L1RPCURLFlag = &cli.StringFlag{
		Name:  L1RPCURLFlagName,
		Usage: "L1 RPC URL",
		EnvVars: []string{
			"L1_RPC_URL",
		},
	}
	InfileFlag = &cli.StringFlag{
		Name:    InfileFlagName,
		Usage:   "input configuration file",
		EnvVars: prefixEnvVar("INFILE"),
	}
	OutfileFlag = &cli.StringFlag{
		Name:    OutfileFlagName,
		Usage:   "output configuration file",
		EnvVars: prefixEnvVar("OUTFILE"),
	}
	ConfigureMnemonicFlag = &cli.StringFlag{
		Name:    ConfigureMnemonicFlagName,
		Usage:   "mnemonic for account generation",
		EnvVars: prefixEnvVar("MNEMONIC"),
	}
	ContractsImageFlag = &cli.StringFlag{
		Name:    ContractsImageFlagName,
		Usage:   "Docker image for deploying contracts",
		EnvVars: prefixEnvVar("IMAGE"),
		Value:   "ethereumoptimism/contracts-bedrock:latest",
	}
	DeployPrivateKeyFlag = &cli.StringFlag{
		Name:    DeployPrivateKeyFlagName,
		Usage:   "private key for deployment",
		EnvVars: prefixEnvVar("PRIVATE_KEY"),
	}
)

var ConfigureFlags = []cli.Flag{
	L1RPCURLFlag,
	InfileFlag,
	OutfileFlag,
	ConfigureMnemonicFlag,
}

var DeployFlags = []cli.Flag{
	L1RPCURLFlag,
	InfileFlag,
	OutfileFlag,
	ContractsImageFlag,
	DeployPrivateKeyFlag,
}

var GenesisFlags = []cli.Flag{
	InfileFlag,
	OutfileFlag,
	ContractsImageFlag,
}

func prefixEnvVar(name string) []string {
	return op_service.PrefixEnvVar(EnvVarPrefix, name)
}
