package deployer

import (
	"archive/tar"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/urfave/cli/v2"
	"io"
	"os"
	"path/filepath"
)

const containerAddressesPath = "/workspace/optimism/packages/contracts-bedrock/deployments/deployment.json"

type DeployCMDConfig struct {
	L1RPCURL       string
	Infile         string
	Outfile        string
	ContractsImage string
	PrivateKeyHex  string

	privateKey *crypto.Secp256k1PrivateKey
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
		config := DeployCMDConfig{
			L1RPCURL:       cliCtx.String(L1RPCURLFlagName),
			Infile:         cliCtx.String(InfileFlagName),
			Outfile:        cliCtx.String(OutfileFlagName),
			ContractsImage: cliCtx.String(DeployImageFlagName),
			PrivateKeyHex:  cliCtx.String(DeployPrivateKeyFlagName),
		}

		if err := config.Check(); err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(cliCtx.Context)
		defer cancel()

		return Deploy(ctx, config)
	}
}

func Deploy(ctx context.Context, config DeployCMDConfig) error {
	dkr, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}

	state, err := ReadDeploymentState(config.Infile)
	if err != nil {
		return fmt.Errorf("failed to read infile: %w", err)
	}

	exists, err := imageExists(dkr, config.ContractsImage)
	if err != nil {
		return fmt.Errorf("failed to check image existence: %w", err)
	}
	if !exists {
		reader, err := dkr.ImagePull(ctx, config.ContractsImage, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull image: %w", err)
		}
		if _, err := io.Copy(os.Stderr, reader); err != nil {
			_ = reader.Close()
			return fmt.Errorf("failed to copy image pull output: %w", err)
		}
		_ = reader.Close()
	}

	absInfile, err := filepath.Abs(config.Infile)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for infile: %w", err)
	}

	createRes, err := dkr.ContainerCreate(ctx, &container.Config{
		Image: config.ContractsImage,
		Env: []string{
			envVar("DEPLOY_ETH_RPC_URL", config.L1RPCURL),
			envVar("DEPLOY_PRIVATE_KEY", config.PrivateKeyHex),
			envVar("DEPLOY_STATE_PATH", "/infile.json"),
		},
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: absInfile,
				Target: "/infile.json",
			},
		},
	}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := dkr.ContainerStart(ctx, createRes.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	go func() {
		out, err := dkr.ContainerLogs(ctx, createRes.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			// Output error log
			fmt.Printf("failed to get logs: %v\n", err)
			return
		}

		_, _ = stdcopy.StdCopy(os.Stdout, os.Stderr, out)
	}()

	statusCh, errCh := dkr.ContainerWait(ctx, createRes.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return fmt.Errorf("container failed: %w", err)
	case <-ctx.Done():
		timeout := 0
		if err := dkr.ContainerStop(ctx, createRes.ID, container.StopOptions{
			Timeout: &timeout,
		}); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}

		return ctx.Err()
	case <-statusCh:
		break
	}

	addressesTarStream, _, err := dkr.CopyFromContainer(ctx, createRes.ID, containerAddressesPath)
	if err != nil {
		return fmt.Errorf("failed to copy addresses file: %w", err)
	}

	addresses, err := extractAddressesFile(addressesTarStream)
	if err != nil {
		return fmt.Errorf("failed to extract addresses file: %w", err)
	}

	state.Addresses = addresses

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

func envVar(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

func extractAddressesFile(r io.ReadCloser) (*Addresses, error) {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("addresses file not found in tar")
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}
		fmt.Println("header.Name", header.Name)
		if header.Name == containerAddressesPath[1:] {
			decoder := json.NewDecoder(tr)
			var addrs Addresses
			if err := decoder.Decode(&addrs); err != nil {
				return nil, fmt.Errorf("failed to decode addresses file: %w", err)
			}
			break
		}
	}

	return nil, errors.New("addresses file not found in tar")
}

func imageExists(dkr *client.Client, imgStr string) (bool, error) {
	images, err := dkr.ImageList(context.Background(), image.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list images: %w", err)
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imgStr {
				return true, nil
			}
		}
	}

	return false, nil
}
