package deployer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/urfave/cli/v2"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

type DeployCMDConfig struct {
	L1RPCURL       string
	Infile         string
	Outfile        string
	ContractsImage string
	PrivateKeyHex  string
	Local          bool
	MonorepoDir    string
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

		absMonorepoDir, err := filepath.Abs(cliCtx.String(MonorepoDirFlagName))
		if err != nil {
			return fmt.Errorf("failed to get absolute path for monorepo-dir: %w", err)
		}

		config := DeployCMDConfig{
			L1RPCURL:       cliCtx.String(L1RPCURLFlagName),
			Infile:         cliCtx.String(InfileFlagName),
			Outfile:        cliCtx.String(OutfileFlagName),
			ContractsImage: cliCtx.String(ContractsImageFlagName),
			PrivateKeyHex:  cliCtx.String(DeployPrivateKeyFlagName),
			Local:          cliCtx.Bool(LocalFlagName),
			MonorepoDir:    absMonorepoDir,
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

	if err := checkCreate2(ctx, config); err != nil {
		return fmt.Errorf("failed to check/create CREATE2 deployer: %w", err)
	}

	var backend ContractDeployerBackend
	if config.Local {
		lgr.Info("using local backend")
		backend = NewLocalBackend(config.Logger, config.MonorepoDir)
	} else {
		lgr.Info("using docker backend", "image", config.ContractsImage)
		backend, err = NewDockerBackend(config.Logger, config.ContractsImage)
		if err != nil {
			return err
		}
	}
	addresses, err := backend.Deploy(ctx, DeployContractsOpts{
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

// var Create2Address = common.HexToAddress("0x4e59b44847b379578588920cA78FbF26c0B4956C")
// break it to test it
var Create2Address = common.HexToAddress("0x4e59b44847b379578588920cA78FbF26c0B4956d")

var Create2Bytecode = "0xf8a58085174876e800830186a08080b853604580600e600039806000f350fe7f" +
	"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0" +
	"3601600081602082378035828234f58015156039578182fd5b80825250505060" +
	"14600cf31ba02222222222222222222222222222222222222222222222222222" +
	"222222222222a022222222222222222222222222222222222222222222222222" +
	"22222222222222"

func checkCreate2(ctx context.Context, config DeployCMDConfig) error {
	lgr := config.Logger

	rc, err := rpc.Dial(config.L1RPCURL)
	if err != nil {
		return fmt.Errorf("failed to connect to L1 RPC: %w", err)
	}
	defer rc.Close()

	ec := ethclient.NewClient(rc)
	code, err := ec.CodeAt(context.Background(), Create2Address, nil)
	if err != nil {
		return fmt.Errorf("failed to get code at address: %w", err)
	}

	if len(code) > 0 {
		lgr.Info("CREATE2 deployer already exists")
		return nil
	}

	lgr.Info("no CREATE2 deployer found, deploying")
	var txHash common.Hash
	if err := rc.CallContext(ctx, &txHash, "eth_sendRawTransaction", Create2Bytecode); err != nil {
		return fmt.Errorf("failed to send CREATE2 deploy transaction: %w", err)
	}

	lgr.Info("sent CREATE2 deploy transaction, awaiting receipt", "txHash", txHash)
	tick := time.NewTicker(time.Second)
	for {
		select {
		case <-tick.C:
			receipt, err := ec.TransactionReceipt(ctx, txHash)
			if errors.Is(err, ethereum.NotFound) {
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to get transaction receipt: %w", err)
			}
			if receipt.Status == 0 {
				return errors.New("deployment transaction failed")
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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
