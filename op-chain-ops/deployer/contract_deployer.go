package deployer

import (
	"archive/tar"
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
)

const containerAddressesPath = "/workspace/optimism/packages/contracts-bedrock/deployments/deployment.json"

type ContractDeployerOpts struct {
	L1RPCURL      string
	State         *DeploymentState
	PrivateKeyHex string
}
type ContractDeployer interface {
	Deploy(ctx context.Context, opts ContractDeployerOpts) (*Addresses, error)
}

type DockerContractDeployer struct {
	contractsImage string
	dkr            *client.Client
	lgr            log.Logger
}

func NewDockerContractDeployer(lgr log.Logger, contractsImage string) (*DockerContractDeployer, error) {
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

func (d *DockerContractDeployer) Deploy(ctx context.Context, opts ContractDeployerOpts) (*Addresses, error) {
	exists, err := d.imageExists(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check if image exists: %w", err)
	}

	if !exists {
		d.lgr.Info("contracts image does not exist, locally, pulling")

		if err := d.pullImage(ctx); err != nil {
			return nil, fmt.Errorf("failed to pull image: %w", err)
		}
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

	d.lgr.Info("starting contracts container", "id", createRes.ID)
	if err := d.dkr.ContainerStart(ctx, createRes.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	go d.streamLogs(ctx, createRes.ID)

	statusCh, errCh := d.dkr.ContainerWait(ctx, createRes.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if errors.Is(err, context.Canceled) {
			return nil, ctx.Err()
		}

		d.lgr.Error("error in contracts container, check logs", "err", err)
		return nil, fmt.Errorf("error in contracts container: %w", err)
	case <-ctx.Done():
		d.lgr.Info("context cancelled, stopping container", "id", createRes.ID)
		timeout := 0
		if err := d.dkr.ContainerStop(ctx, createRes.ID, container.StopOptions{
			Timeout: &timeout,
		}); err != nil {
			d.lgr.Error("error stopping container", "id", createRes.ID, "err", err)
		}

		return nil, ctx.Err()
	case <-statusCh:
		break
	}

	d.lgr.Info("contracts deployed, reading addresses file")
	return d.readAddressesFile(ctx, createRes.ID)
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
	stream, _, err := d.dkr.CopyFromContainer(ctx, id, containerAddressesPath)
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
		if header.Name == "deployment.json" {
			decoder := json.NewDecoder(tr)
			var addrs Addresses
			if err := decoder.Decode(&addrs); err != nil {
				return nil, fmt.Errorf("failed to decode addresses file: %w", err)
			}
			return &addrs, nil
		}
	}
}

func envVar(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}
