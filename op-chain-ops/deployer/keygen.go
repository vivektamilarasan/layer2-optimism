package deployer

import (
	"crypto/ecdsa"
	"github.com/ethereum-optimism/optimism/op-chain-ops/devkeys"
	"github.com/ethereum/go-ethereum/common"
)

type KeyGenerator interface {
	Address(key devkeys.Key) (common.Address, error)
	PrivateKey(key devkeys.Key) (*ecdsa.PrivateKey, error)
}

type MnemonicKeyGenerator struct {
	dk *devkeys.MnemonicDevKeys
}

func NewMnemonicKeyGenerator(mnemonic string) (*MnemonicKeyGenerator, error) {
	dk, err := devkeys.NewMnemonicDevKeys(mnemonic)
	if err != nil {
		return nil, err
	}

	return &MnemonicKeyGenerator{
		dk: dk,
	}, nil
}

func (m *MnemonicKeyGenerator) Address(key devkeys.Key) (common.Address, error) {
	return m.dk.Address(key)
}

func (m *MnemonicKeyGenerator) PrivateKey(key devkeys.Key) (*ecdsa.PrivateKey, error) {
	return m.dk.Secret(key)
}
