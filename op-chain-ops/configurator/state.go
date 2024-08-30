package configurator

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"os"
)

type DeploymentState struct {
	Intent *ChainIntent `json:"intent"`

	DeployConfig *genesis.DeployConfig `json:"deployConfig,omitempty"`

	Addresses *Addresses `json:"addresses,omitempty"`
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
