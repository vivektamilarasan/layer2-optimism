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
	"os/exec"
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

type DockerBackend struct {
	contractsImage string
	dkr            *client.Client
	lgr            log.Logger
}

func NewDockerBackend(lgr log.Logger, contractsImage string) (*DockerBackend, error) {
	dkr, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerBackend{
		contractsImage: contractsImage,
		dkr:            dkr,
		lgr:            lgr,
	}, nil
}

func (d *DockerBackend) Deploy(ctx context.Context, opts DeployContractsOpts) (*Addresses, error) {
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

func (d *DockerBackend) GenerateAllocs(ctx context.Context, opts GenerateAllocsOpts) ([]byte, error) {
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

	if err := json.NewEncoder(addressesFile).Encode(opts.State.Addresses); err != nil {
		return nil, fmt.Errorf("failed to encode addresses file: %w", err)
	}

	deployConfigFile, err := os.CreateTemp("", "deploy-config-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp deploy config file: %w", err)
	}
	defer os.Remove(deployConfigFile.Name())

	if err := json.NewEncoder(deployConfigFile).Encode(opts.State.DeployConfig); err != nil {
		return nil, fmt.Errorf("failed to encode deploy config file: %w", err)
	}

	d.lgr.Info("creating contracts container")
	createRes, err := d.dkr.ContainerCreate(ctx, &container.Config{
		Image: d.contractsImage,
		Env: []string{
			envVar("CONTRACT_ADDRESSES_PATH", "/addresses.json"),
			envVar("DEPLOY_CONFIG_PATH", "/workspace/optimism/packages/contracts-bedrock/deploy-config/deploy-config.json"),
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
			{
				Type:   mount.TypeBind,
				Source: deployConfigFile.Name(),
				Target: "/workspace/optimism/packages/contracts-bedrock/deploy-config/deploy-config.json",
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

func (d *DockerBackend) ensureImage(ctx context.Context) error {
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

func (d *DockerBackend) runContainer(ctx context.Context, id string) error {
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

func (d *DockerBackend) imageExists(ctx context.Context) (bool, error) {
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

func (d *DockerBackend) pullImage(ctx context.Context) error {
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

func (d *DockerBackend) streamLogs(ctx context.Context, id string) {
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

func (d *DockerBackend) readAddressesFile(ctx context.Context, id string) (*Addresses, error) {
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

func (d *DockerBackend) readFileFromContainer(ctx context.Context, id, path string) ([]byte, error) {
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

func (d *DockerBackend) awaitContainerExit(ctx context.Context, id string) error {
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

type LocalBackend struct {
	monorepoDir string
	lgr         log.Logger
}

func NewLocalBackend(lgr log.Logger, monorepoDir string) *LocalBackend {
	return &LocalBackend{
		monorepoDir: monorepoDir,
		lgr:         lgr,
	}
}

func (l *LocalBackend) Deploy(ctx context.Context, opts DeployContractsOpts) (*Addresses, error) {
	stateFile, err := os.CreateTemp("", "infile-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp state file: %w", err)
	}
	defer os.Remove(stateFile.Name())

	cmd := exec.Command("bash", "deploy.sh")
	cmd.Dir = filepath.Join(l.monorepoDir, "packages", "contracts-bedrock")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		envVar("DEPLOY_ETH_RPC_URL", opts.L1RPCURL),
		envVar("DEPLOY_PRIVATE_KEY", opts.PrivateKeyHex),
		envVar("DEPLOY_STATE_PATH", stateFile.Name()),
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start deploy script: %w", err)
	}
	if err := l.awaitCommand(ctx, cmd); err != nil {
		return nil, err
	}

	addrFilePath := filepath.Join(l.monorepoDir, "packages", "contracts-bedrock", "deployments", "deployment.json")
	addrFile, err := os.Open(addrFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open addresses file: %w", err)
	}
	defer addrFile.Close()

	var addrs Addresses
	if err := json.NewDecoder(addrFile).Decode(&addrs); err != nil {
		return nil, fmt.Errorf("failed to decode addresses file: %w", err)
	}
	return &addrs, nil
}

func (l *LocalBackend) GenerateAllocs(ctx context.Context, opts GenerateAllocsOpts) ([]byte, error) {
	if opts.State.Addresses == nil {
		return nil, errors.New("addresses not found in state")
	}

	l.lgr.Info("writing addresses to temporary file")
	addressesFile, err := os.CreateTemp("", "addresses-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp addresses file: %w", err)
	}
	defer os.Remove(addressesFile.Name())

	if err := json.NewEncoder(addressesFile).Encode(opts.State.Addresses); err != nil {
		return nil, fmt.Errorf("failed to encode addresses file: %w", err)
	}

	deployConfigFile, err := os.CreateTemp("", "deploy-config-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp deploy config file: %w", err)
	}
	defer os.Remove(deployConfigFile.Name())

	if err := json.NewEncoder(deployConfigFile).Encode(opts.State.DeployConfig); err != nil {
		return nil, fmt.Errorf("failed to encode deploy config file: %w", err)
	}

	cmd := exec.Command(
		"forge",
		"script",
		"scripts/L2Genesis.s.sol:L2Genesis",
		"--sig",
		"runWithStateDump()",
		"--chain-id",
		fmt.Sprintf("%d", opts.L2ChainID),
	)
	cmd.Dir = filepath.Join(l.monorepoDir, "packages", "contracts-bedrock")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		envVar("CONTRACT_ADDRESSES_PATH", addressesFile.Name()),
		envVar("DEPLOY_CONFIG_PATH", filepath.Join(cmd.Dir, "deploy-config", "deploy-config.json")),
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start forge script: %w", err)
	}
	if err := l.awaitCommand(ctx, cmd); err != nil {
		return nil, err
	}

	allocFilePath := filepath.Join(l.monorepoDir, "packages", "contracts-bedrock", fmt.Sprintf("state-dump-%d-granite.json", opts.L2ChainID))
	allocFile, err := os.Open(allocFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open allocs file: %w", err)
	}
	defer allocFile.Close()

	allocData, err := io.ReadAll(allocFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read allocs file: %w", err)
	}
	return allocData, nil
}

func (l *LocalBackend) awaitCommand(ctx context.Context, cmd *exec.Cmd) error {
	cmdErrCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		cmdErrCh <- err
	}()

	select {
	case <-ctx.Done():
		if err := cmd.Process.Kill(); err != nil {
			l.lgr.Error("failed to kill script", "err", err)
		}

		<-cmdErrCh

		return ctx.Err()
	case err := <-cmdErrCh:
		if err != nil {
			return fmt.Errorf("script failed: %w", err)
		}

		return nil
	}
}
