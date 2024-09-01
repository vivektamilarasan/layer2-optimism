package deployer

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum-optimism/optimism/op-chain-ops/foundry"
	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

type GenesisCMDConfig struct {
	L1RPCURL       string
	Infile         string
	Outfile        string
	ContractsImage string
	Logger         log.Logger
}

func (g *GenesisCMDConfig) Check() error {
	if g.L1RPCURL == "" {
		return fmt.Errorf("l1-rpc-url must be specified")
	}

	if g.Infile == "" {
		return fmt.Errorf("infile must be specified")
	}

	if g.Outfile == "" {
		return fmt.Errorf("outfile must be specified")
	}

	if g.ContractsImage == "" {
		return fmt.Errorf("image must be specified")
	}

	return nil
}

func GenesisCLI() func(ctx *cli.Context) error {
	return func(cliCtx *cli.Context) error {
		logCfg := oplog.ReadCLIConfig(cliCtx)
		l := oplog.NewLogger(oplog.AppOut(cliCtx), logCfg)
		oplog.SetGlobalLogHandler(l.Handler())

		config := GenesisCMDConfig{
			L1RPCURL:       cliCtx.String(L1RPCURLFlagName),
			Infile:         cliCtx.String(InfileFlagName),
			Outfile:        cliCtx.String(OutfileFlagName),
			ContractsImage: cliCtx.String(ContractsImageFlagName),
			Logger:         l,
		}

		if err := config.Check(); err != nil {
			return err
		}

		return Genesis(context.Background(), config)
	}
}

func Genesis(ctx context.Context, config GenesisCMDConfig) error {
	lgr := config.Logger

	lgr.Info("reading deployment state", "file", config.Infile)
	state, err := ReadDeploymentState(config.Infile)
	if err != nil {
		return fmt.Errorf("failed to read infile: %w", err)
	}

	if state.Addresses == nil {
		return fmt.Errorf("no addresses found in state - contracts must be deployed first")
	}

	backend, err := NewDockerBackend(lgr, config.ContractsImage)
	if err != nil {
		return fmt.Errorf("failed to create docker backend: %w", err)
	}

	lgr.Info("generating l2 allocs")
	allocData, err := backend.GenesisAllocs(ctx, GenerateAllocsOpts{
		L2ChainID: state.DeployConfig.L2ChainID,
		State:     state,
	})
	if err != nil {
		return fmt.Errorf("failed to generate allocs: %w", err)
	}

	var l2Allocs foundry.ForgeAllocs
	if err := json.Unmarshal(allocData, &l2Allocs); err != nil {
		return fmt.Errorf("failed to unmarshal allocs: %w", err)
	}

	lgr.Info("fetching L2 start block on L1")
	ethClient, err := ethclient.Dial(config.L1RPCURL)
	if err != nil {
		return fmt.Errorf("failed to connect to L1 RPC: %w", err)
	}
	startBlock, err := ethClient.BlockByHash(ctx, *state.DeployConfig.L1StartingBlockTag.BlockHash)
	if err != nil {
		return fmt.Errorf("failed to fetch start block: %w", err)
	}

	lgr.Info("building L2 genesis")
	l2Genesis, err := genesis.BuildL2Genesis(state.DeployConfig, &l2Allocs, startBlock)
	if err != nil {
		return fmt.Errorf("failed to build L2 genesis: %w", err)
	}

	l2GenesisBlock := l2Genesis.ToBlock()
	rollupConfig, err := state.DeployConfig.RollupConfig(startBlock, l2GenesisBlock.Hash(), l2GenesisBlock.NumberU64())
	if err != nil {
		return fmt.Errorf("failed to create rollup config: %w", err)
	}
	if err := rollupConfig.Check(); err != nil {
		return fmt.Errorf("generated rollup config does not pass validation: %w", err)
	}

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	if err := json.NewEncoder(gw).Encode(l2Genesis); err != nil {
		return fmt.Errorf("failed to encode genesis block: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	if state.GenesisFiles == nil {
		state.GenesisFiles = make(map[uint64]Base64Encoded)
	}
	if state.RollupConfigs == nil {
		state.RollupConfigs = make(map[uint64]*rollup.Config)
	}
	state.GenesisFiles[state.DeployConfig.L2ChainID] = buf.Bytes()
	state.RollupConfigs[state.DeployConfig.L2ChainID] = rollupConfig

	if err := WriteDeploymentState(config.Outfile, state); err != nil {
		return fmt.Errorf("failed to write outfile: %w", err)
	}

	return nil
}
