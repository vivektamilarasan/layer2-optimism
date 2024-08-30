package configurator

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum-optimism/optimism/op-chain-ops/devkeys"
	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"math/big"
)

var DefaultFaultGameAbsolutePrestate = common.HexToHash("0x03c7ae758795765c6664a5d39bf63841c71ff191e9189522bad8ebff5d4eca98")

type ChainIntent struct {
	L1ChainID uint64 `json:"l1ChainID"`

	L2ChainID uint64 `json:"l2ChainID"`

	UseFaultProofs bool `json:"useFaultProofs"`

	UseAltDA bool `json:"useAltDA"`

	FundDevAccounts bool `json:"fundDevAccounts"`

	Overrides map[string]interface{} `json:"overrides"`
}

func (c *ChainIntent) L1ChainIDBig() *big.Int {
	return big.NewInt(int64(c.L1ChainID))
}

func (c *ChainIntent) L2ChainIDBig() *big.Int {
	return big.NewInt(int64(c.L2ChainID))
}

func (c ChainIntent) Check() error {
	if c.L1ChainID == 0 {
		return fmt.Errorf("L1ChainID must be set")
	}

	if c.L2ChainID == 0 {
		return fmt.Errorf("L2ChainID must be set")
	}

	if c.UseFaultProofs && c.UseAltDA {
		return fmt.Errorf("cannot use both fault proofs and alt-DA")
	}

	return nil
}

func NewDeployConfig(keygen KeyGenerator, l1RPC *ethclient.Client, intent *ChainIntent) (*genesis.DeployConfig, error) {
	var guardErr error
	addrFor := func(role devkeys.Role) common.Address {
		if guardErr != nil {
			return common.Address{}
		}

		key := role.Key(intent.L1ChainIDBig())
		addr, err := keygen.Address(key)
		if err != nil {
			guardErr = fmt.Errorf("failed to derive address for %s: %w", role, err)
			return common.Address{}
		}
		return addr
	}

	l1StartingBlock, err := l1RPC.HeaderByNumber(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get L1 starting block: %w", err)
	}

	l1StartingBlockHash := l1StartingBlock.Hash()

	l2GenesisBlockBaseFeePerGas := hexutil.Big(*(big.NewInt(1000000000)))

	cfg := &genesis.DeployConfig{
		L2InitializationConfig: genesis.L2InitializationConfig{
			DevDeployConfig: genesis.DevDeployConfig{},
			L2GenesisBlockDeployConfig: genesis.L2GenesisBlockDeployConfig{
				L2GenesisBlockGasLimit:      30_000_000,
				L2GenesisBlockBaseFeePerGas: &l2GenesisBlockBaseFeePerGas,
			},
			OwnershipDeployConfig: genesis.OwnershipDeployConfig{
				ProxyAdminOwner:  addrFor(devkeys.L2ProxyAdminOwnerRole),
				FinalSystemOwner: addrFor(devkeys.L1ProxyAdminOwnerRole),
			},
			L2VaultsDeployConfig: genesis.L2VaultsDeployConfig{
				BaseFeeVaultRecipient:              addrFor(devkeys.BaseFeeVaultRecipientRole),
				L1FeeVaultRecipient:                addrFor(devkeys.L1FeeVaultRecipientRole),
				SequencerFeeVaultRecipient:         addrFor(devkeys.SequencerFeeVaultRecipientRole),
				BaseFeeVaultWithdrawalNetwork:      genesis.WithdrawalNetwork("local"),
				L1FeeVaultWithdrawalNetwork:        genesis.WithdrawalNetwork("local"),
				SequencerFeeVaultWithdrawalNetwork: genesis.WithdrawalNetwork("local"),
			},
			GovernanceDeployConfig: genesis.GovernanceDeployConfig{
				EnableGovernance:      true,
				GovernanceTokenSymbol: "OP",
				GovernanceTokenName:   "Optimism",
				GovernanceTokenOwner:  addrFor(devkeys.L2ProxyAdminOwnerRole),
			},
			GasPriceOracleDeployConfig: genesis.GasPriceOracleDeployConfig{
				GasPriceOracleBaseFeeScalar:     0,
				GasPriceOracleBlobBaseFeeScalar: 1000000,
			},
			OperatorDeployConfig: genesis.OperatorDeployConfig{
				P2PSequencerAddress: addrFor(devkeys.SequencerP2PRole),
				BatchSenderAddress:  addrFor(devkeys.BatcherRole),
			},
			EIP1559DeployConfig: genesis.EIP1559DeployConfig{
				EIP1559Denominator:       50,
				EIP1559DenominatorCanyon: 250,
				EIP1559Elasticity:        6,
			},
			UpgradeScheduleDeployConfig: genesis.UpgradeScheduleDeployConfig{},
			L2CoreDeployConfig: genesis.L2CoreDeployConfig{
				L1ChainID:                 intent.L1ChainID,
				L2ChainID:                 intent.L2ChainID,
				L2BlockTime:               2,
				FinalizationPeriodSeconds: 12,
				MaxSequencerDrift:         600,
				SequencerWindowSize:       3600,
				ChannelTimeoutBedrock:     300,
				BatchInboxAddress:         batchInboxAddress(intent.L2ChainIDBig()),
				SystemConfigStartBlock:    0,
			},
		},
		L1StartingBlockTag: &genesis.MarshalableRPCBlockNumberOrHash{
			BlockHash: &l1StartingBlockHash,
		},
		SuperchainL1DeployConfig: genesis.SuperchainL1DeployConfig{
			RequiredProtocolVersion:    rollup.OPStackSupport,
			RecommendedProtocolVersion: rollup.OPStackSupport,
			SuperchainConfigGuardian:   addrFor(devkeys.SuperchainConfigGuardianKey),
		},
		OutputOracleDeployConfig: genesis.OutputOracleDeployConfig{
			L2OutputOracleSubmissionInterval:  10,
			L2OutputOracleStartingBlockNumber: 0,
			L2OutputOracleStartingTimestamp:   0,
			L2OutputOracleChallenger:          addrFor(devkeys.ChallengerRole),
			L2OutputOracleProposer:            addrFor(devkeys.ProposerRole),
		},
	}

	if intent.FundDevAccounts {
		cfg.DevDeployConfig = genesis.DevDeployConfig{
			FundDevAccounts: true,
		}
	}

	if intent.UseFaultProofs {
		cfg.UseFaultProofs = true
		cfg.FaultProofDeployConfig = genesis.FaultProofDeployConfig{
			UseFaultProofs:                true,
			FaultGameAbsolutePrestate:     DefaultFaultGameAbsolutePrestate,
			FaultGameMaxDepth:             44,
			FaultGameClockExtension:       0,
			FaultGameMaxClockDuration:     1200,
			FaultGameGenesisBlock:         0,
			FaultGameGenesisOutputRoot:    common.Hash{},
			FaultGameSplitDepth:           14,
			FaultGameWithdrawalDelay:      600,
			PreimageOracleMinProposalSize: 1800000,
			PreimageOracleChallengePeriod: 300,
		}
	}

	if intent.UseAltDA {
		cfg.UseAltDA = true
		cfg.AltDADeployConfig = genesis.AltDADeployConfig{
			DAChallengeWindow:          140,
			DAResolveWindow:            160,
			DABondSize:                 1000000,
			DAResolverRefundPercentage: 0,
		}
	}

	if guardErr != nil {
		return nil, guardErr
	}

	if intent.Overrides != nil {
		cfg, err = ApplyDeployConfigOverrides(cfg, intent.Overrides)
		if err != nil {
			return nil, fmt.Errorf("failed to apply overrides: %w", err)
		}
	}

	if err := cfg.Check(log.NewLogger(log.DiscardHandler())); err != nil {
		return nil, fmt.Errorf("deploy config failed validation: %w", err)
	}

	return cfg, nil
}

func ApplyDeployConfigOverrides(deployConfig *genesis.DeployConfig, overrides map[string]interface{}) (*genesis.DeployConfig, error) {
	jsonString, err := json.Marshal(deployConfig)
	if err != nil {
		return nil, err
	}
	tmpDeployConfig := map[string]interface{}{}
	if err := json.Unmarshal(jsonString, &tmpDeployConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deploy config: %w", err)
	}
	for key, value := range overrides {
		tmpDeployConfig[key] = value
	}
	jsonString, err = json.Marshal(tmpDeployConfig)
	if err != nil {
		return nil, err
	}
	result := &genesis.DeployConfig{}
	if err := json.Unmarshal(jsonString, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal overridden deploy config: %w", err)
	}
	return result, nil
}

func batchInboxAddress(chainID *big.Int) common.Address {
	addr := common.Address{0x42}
	chainIDBytes := chainID.Bytes()
	j := len(addr) - 1

	for i := len(chainIDBytes) - 1; i >= 0; i-- {
		addr[j] = chainIDBytes[i]
		j--

		if j == 0 {
			break
		}
	}
	return addr
}
