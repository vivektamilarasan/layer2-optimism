package vm

import (
	"github.com/ethereum-optimism/optimism/op-challenger/game/fault/trace/utils"
)

type KonaArgs struct {
	cfg Config
}

var _ ServerArgs = (*KonaArgs)(nil)

func NewKonaArgs(cfg Config) *KonaArgs {
	return &KonaArgs{
		cfg: cfg,
	}
}

func (s *KonaArgs) HostCommand(dataDir string, inputs utils.LocalGameInputs) ([]string, error) {
	args := []string{
		s.cfg.Server, "--server",
		"--l1-node-address", s.cfg.L1,
		"--l1-beacon-address", s.cfg.L1Beacon,
		"--l2-node-address", s.cfg.L2,
		"--data-dir", dataDir,
		"--l1-head", inputs.L1Head.Hex(),
		"--l2-head", inputs.L2Head.Hex(),
		"--l2-output-root", inputs.L2OutputRoot.Hex(),
		"--l2-claim", inputs.L2Claim.Hex(),
		"--l2-block-number", inputs.L2BlockNumber.Text(10),
	}
	if s.cfg.Network != "" {
		args = append(args, "--network", s.cfg.Network)
	}
	if s.cfg.RollupConfigPath != "" {
		args = append(args, "--rollup.config", s.cfg.RollupConfigPath)
	}
	if s.cfg.L2GenesisPath != "" {
		args = append(args, "--l2.genesis", s.cfg.L2GenesisPath)
	}
	return args, nil
}
