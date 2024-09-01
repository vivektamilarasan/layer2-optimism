package deployer

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/ethereum/go-ethereum/log"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	containerAddressesPath = "/workspace/optimism/packages/contracts-bedrock/deployments/deployment.json"
	containerAllocsPath    = "/workspace/optimism/packages/contracts-bedrock/state-dump-%d-%s.json"
)

type DeployContractsOpts struct {
	L1RPCURL      string
	State         *DeploymentState
	PrivateKeyHex string
}

type GenerateAllocsOpts struct {
	L2ChainID uint64
	State     *DeploymentState
}
type ContractDeployerBackend interface {
	Deploy(ctx context.Context, opts DeployContractsOpts) (*Addresses, error)
}

type AllocsBackend interface {
	GenerateAllocs(ctx context.Context, opts GenerateAllocsOpts) ([]byte, error)
}

type DockerContractDeployer struct {
	contractsImage string
	dkr            *client.Client
	lgr            log.Logger
}

func NewDockerBackend(lgr log.Logger, contractsImage string) (*DockerContractDeployer, error) {
	dkr, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerContractDeployer{
		contractsImage: contractsImage,
		dkr:            dkr,
		lgr:            lgr,
	}, nil
}

func (d *DockerContractDeployer) Deploy(ctx context.Context, opts DeployContractsOpts) (*Addresses, error) {
	if err := d.ensureImage(ctx); err != nil {
		return nil, err
	}

	d.lgr.Info("writing deployment state to temporary file")
	stateFile, err := os.CreateTemp("", "infile-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp state file: %w", err)
	}
	defer os.Remove(stateFile.Name())

	if err := WriteDeploymentState(stateFile.Name(), opts.State); err != nil {
		return nil, fmt.Errorf("failed to write temp state file: %w", err)
	}

	d.lgr.Info("creating contracts container")
	createRes, err := d.dkr.ContainerCreate(ctx, &container.Config{
		Image: d.contractsImage,
		Env: []string{
			envVar("DEPLOY_ETH_RPC_URL", opts.L1RPCURL),
			envVar("DEPLOY_PRIVATE_KEY", opts.PrivateKeyHex),
			envVar("DEPLOY_STATE_PATH", "/infile.json"),
		},
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: stateFile.Name(),
				Target: "/infile.json",
			},
		},
	}, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := d.runContainer(ctx, createRes.ID); err != nil {
		return nil, fmt.Errorf("failed to run container: %w", err)
	}

	d.lgr.Info("reading addresses file")
	return d.readAddressesFile(ctx, createRes.ID)
}

func (d *DockerContractDeployer) GenesisAllocs(ctx context.Context, opts GenerateAllocsOpts) ([]byte, error) {
	if err := d.ensureImage(ctx); err != nil {
		return nil, err
	}

	if opts.State.Addresses == nil {
		return nil, errors.New("addresses not found in state")
	}

	d.lgr.Info("writing addresses to temporary file")
	addressesFile, err := os.CreateTemp("", "addresses-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp addresses file: %w", err)
	}
	defer os.Remove(addressesFile.Name())

	encoder := json.NewEncoder(addressesFile)
	if err := encoder.Encode(opts.State.Addresses); err != nil {
		return nil, fmt.Errorf("failed to encode addresses file: %w", err)
	}

	d.lgr.Info("creating contracts container")
	createRes, err := d.dkr.ContainerCreate(ctx, &container.Config{
		Image: d.contractsImage,
		Env: []string{
			envVar("CONTRACT_ADDRESSES_PATH", "/addresses.json"),
		},
		Cmd: []string{
			"forge",
			"script",
			"scripts/L2Genesis.s.sol:L2Genesis",
			"--sig",
			"runWithStateDump()",
			"--chain-id",
			fmt.Sprintf("%d", opts.L2ChainID),
		},
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: addressesFile.Name(),
				Target: "/addresses.json",
			},
		},
	}, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := d.runContainer(ctx, createRes.ID); err != nil {
		return nil, fmt.Errorf("failed to run container: %w", err)
	}

	return d.readFileFromContainer(
		ctx,
		createRes.ID,
		fmt.Sprintf(containerAllocsPath, opts.L2ChainID, "granite"),
	)
}

func (d *DockerContractDeployer) ensureImage(ctx context.Context) error {
	exists, err := d.imageExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if image exists: %w", err)
	}

	if !exists {
		d.lgr.Info("contracts image does not exist, locally, pulling")

		if err := d.pullImage(ctx); err != nil {
			return fmt.Errorf("failed to pull image: %w", err)
		}
	}
	return nil
}

func (d *DockerContractDeployer) runContainer(ctx context.Context, id string) error {
	d.lgr.Info("starting container", "id", id)
	if err := d.dkr.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	go d.streamLogs(ctx, id)

	start := time.Now()
	if err := d.awaitContainerExit(ctx, id); err != nil {
		return fmt.Errorf("error awaiting container: %w", err)
	}
	d.lgr.Info("container complete", "duration", time.Since(start))

	return nil
}

func (d *DockerContractDeployer) imageExists(ctx context.Context) (bool, error) {
	images, err := d.dkr.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list images: %w", err)
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == d.contractsImage {
				return true, nil
			}
		}
	}

	return false, nil
}

func (d *DockerContractDeployer) pullImage(ctx context.Context) error {
	reader, err := d.dkr.ImagePull(ctx, d.contractsImage, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()
	if _, err := io.Copy(os.Stderr, reader); err != nil {
		_ = reader.Close()
		return fmt.Errorf("failed to copy image pull output: %w", err)
	}
	return nil
}

func (d *DockerContractDeployer) streamLogs(ctx context.Context, id string) {
	stream, err := d.dkr.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		d.lgr.Error("error streaming logs", "containerID", id, "err", err)
	}
	defer stream.Close()

	_, _ = stdcopy.StdCopy(os.Stdout, os.Stderr, stream)
}

func (d *DockerContractDeployer) readAddressesFile(ctx context.Context, id string) (*Addresses, error) {
	data, err := d.readFileFromContainer(ctx, id, containerAddressesPath)
	if err != nil {
		return nil, err
	}

	var addrs Addresses
	if err := json.Unmarshal(data, &addrs); err != nil {
		return nil, fmt.Errorf("failed to decode addresses file: %w", err)
	}
	return &addrs, nil
}

func (d *DockerContractDeployer) readFileFromContainer(ctx context.Context, id, path string) ([]byte, error) {
	stream, _, err := d.dkr.CopyFromContainer(ctx, id, path)
	if err != nil {
		return nil, fmt.Errorf("failed to copy addresses file: %w", err)
	}
	defer stream.Close()

	tr := tar.NewReader(stream)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("addresses file not found in tar")
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}
		if header.Name == filepath.Base(path) {
			b := new(bytes.Buffer)
			if _, err := io.Copy(b, tr); err != nil {
				return nil, fmt.Errorf("failed to read file from tar: %w", err)
			}
			return b.Bytes(), nil
		}
	}
}

func (d *DockerContractDeployer) awaitContainerExit(ctx context.Context, id string) error {
	statusCh, errCh := d.dkr.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if errors.Is(err, context.Canceled) {
			return err
		}

		return fmt.Errorf("error in container: %w", err)
	case <-ctx.Done():
		d.lgr.Info("context cancelled, stopping container", "id", id)
		timeout := 0
		if err := d.dkr.ContainerStop(ctx, id, container.StopOptions{
			Timeout: &timeout,
		}); err != nil {
			d.lgr.Error("error stopping container", "id", id, "err", err)
		}

		return ctx.Err()
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return fmt.Errorf("container exited with status %d", status.StatusCode)
		}

		return nil
	}
}

func envVar(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}
