package deployer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	"github.com/ethereum/go-ethereum/log"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/urfave/cli/v2"
	"os"
	"os/signal"
)

type DeployCMDConfig struct {
	L1RPCURL       string
	Infile         string
	Outfile        string
	ContractsImage string
	PrivateKeyHex  string
	Logger         log.Logger
}

func (d *DeployCMDConfig) Check() error {
	if d.L1RPCURL == "" {
		return fmt.Errorf("l1-rpc-url must be specified")
	}

	if d.Infile == "" {
		return fmt.Errorf("infile must be specified")
	}

	if d.Outfile == "" {
		return fmt.Errorf("outfile must be specified")
	}

	if d.ContractsImage == "" {
		return fmt.Errorf("image must be specified")
	}

	if d.PrivateKeyHex == "" {
		return fmt.Errorf("private key must be specified")
	}

	if err := checkPriv(d.PrivateKeyHex); err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	return nil
}

func DeployCLI() func(ctx *cli.Context) error {
	return func(cliCtx *cli.Context) error {
		logCfg := oplog.ReadCLIConfig(cliCtx)
		l := oplog.NewLogger(oplog.AppOut(cliCtx), logCfg)
		oplog.SetGlobalLogHandler(l.Handler())

		config := DeployCMDConfig{
			L1RPCURL:       cliCtx.String(L1RPCURLFlagName),
			Infile:         cliCtx.String(InfileFlagName),
			Outfile:        cliCtx.String(OutfileFlagName),
			ContractsImage: cliCtx.String(ContractsImageFlagName),
			PrivateKeyHex:  cliCtx.String(DeployPrivateKeyFlagName),
			Logger:         l,
		}

		if err := config.Check(); err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(cliCtx.Context)
		defer cancel()

		errCh := make(chan error, 1)
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, os.Interrupt)

		go func() {
			err := Deploy(ctx, config)
			errCh <- err
		}()

		select {
		case err := <-errCh:
			return err
		case <-sigs:
			cancel()
			<-errCh
			return nil
		}
	}
}

func Deploy(ctx context.Context, config DeployCMDConfig) error {
	lgr := config.Logger

	lgr.Info("reading deployment state", "file", config.Infile)
	state, err := ReadDeploymentState(config.Infile)
	if err != nil {
		return fmt.Errorf("failed to read infile: %w", err)
	}

	lgr.Info("performing deployment")
	deployer, err := NewDockerBackend(config.Logger, config.ContractsImage)
	if err != nil {
		return err
	}
	addresses, err := deployer.Deploy(ctx, DeployContractsOpts{
		L1RPCURL:      config.L1RPCURL,
		State:         state,
		PrivateKeyHex: config.PrivateKeyHex,
	})
	if err != nil {
		return err
	}

	state.Addresses = addresses

	lgr.Info("writing deployment state", "file", config.Outfile)
	if err := WriteDeploymentState(config.Outfile, state); err != nil {
		return fmt.Errorf("failed to write outfile: %w", err)
	}

	return nil
}

func checkPriv(data string) error {
	if len(data) > 2 && data[:2] == "0x" {
		data = data[2:]
	}
	b, err := hex.DecodeString(data)
	if err != nil {
		return errors.New("private key is not formatted in hex chars")
	}
	if _, err := crypto.UnmarshalSecp256k1PrivateKey(b); err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	return nil
}
