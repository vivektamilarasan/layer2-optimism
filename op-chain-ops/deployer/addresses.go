package deployer

import "github.com/ethereum/go-ethereum/common"

type Addresses struct {
	AddressManager                    common.Address `json:"AddressManager"`
	AnchorStateRegistry               common.Address `json:"AnchorStateRegistry"`
	AnchorStateRegistryProxy          common.Address `json:"AnchorStateRegistryProxy"`
	DelayedWETH                       common.Address `json:"DelayedWETH"`
	DelayedWETHProxy                  common.Address `json:"DelayedWETHProxy"`
	DisputeGameFactory                common.Address `json:"DisputeGameFactory"`
	DisputeGameFactoryProxy           common.Address `json:"DisputeGameFactoryProxy"`
	L1CrossDomainMessenger            common.Address `json:"L1CrossDomainMessenger"`
	L1CrossDomainMessengerProxy       common.Address `json:"L1CrossDomainMessengerProxy"`
	L1ERC721Bridge                    common.Address `json:"L1ERC721Bridge"`
	L1ERC721BridgeProxy               common.Address `json:"L1ERC721BridgeProxy"`
	L1StandardBridge                  common.Address `json:"L1StandardBridge"`
	L1StandardBridgeProxy             common.Address `json:"L1StandardBridgeProxy"`
	L2OutputOracle                    common.Address `json:"L2OutputOracle"`
	L2OutputOracleProxy               common.Address `json:"L2OutputOracleProxy"`
	Mips                              common.Address `json:"Mips"`
	OptimismMintableERC20Factory      common.Address `json:"OptimismMintableERC20Factory"`
	OptimismMintableERC20FactoryProxy common.Address `json:"OptimismMintableERC20FactoryProxy"`
	OptimismPortal                    common.Address `json:"OptimismPortal"`
	OptimismPortal2                   common.Address `json:"OptimismPortal2"`
	OptimismPortalProxy               common.Address `json:"OptimismPortalProxy"`
	PreimageOracle                    common.Address `json:"PreimageOracle"`
	ProtocolVersions                  common.Address `json:"ProtocolVersions"`
	ProtocolVersionsProxy             common.Address `json:"ProtocolVersionsProxy"`
	ProxyAdmin                        common.Address `json:"ProxyAdmin"`
	SafeProxyFactory                  common.Address `json:"SafeProxyFactory"`
	SafeSingleton                     common.Address `json:"SafeSingleton"`
	SuperchainConfig                  common.Address `json:"SuperchainConfig"`
	SuperchainConfigProxy             common.Address `json:"SuperchainConfigProxy"`
	SystemConfig                      common.Address `json:"SystemConfig"`
	SystemConfigProxy                 common.Address `json:"SystemConfigProxy"`
	SystemOwnerSafe                   common.Address `json:"SystemOwnerSafe"`
}
