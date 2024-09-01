package deployer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"os"
)

type DeploymentState struct {
	Intent *ChainIntent `json:"intent"`

	DeployConfig *genesis.DeployConfig `json:"deployConfig,omitempty"`

	Addresses *Addresses `json:"addresses,omitempty"`

	GenesisFiles map[uint64]Base64Encoded `json:"genesisFiles,omitempty"`

	RollupConfigs map[uint64]*rollup.Config `json:"rollupConfigs,omitempty"`
}

type Base64Encoded []byte

func (b Base64Encoded) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.StdEncoding.EncodeToString(b))
}

func (b *Base64Encoded) UnmarshalJSON(data []byte) error {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}

	*b = decoded
	return nil
}

func ReadDeploymentState(path string) (*DeploymentState, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open chain state file: %w", err)
	}
	defer file.Close()

	var state DeploymentState
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to decode chain state file: %w", err)
	}

	return &state, nil
}

func WriteDeploymentState(path string, state *DeploymentState) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create chain state file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("failed to encode chain state file: %w", err)
	}

	return nil
}
