package evmos

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
)

// RegisterLegacyInterfaces registers the legacy Evmos v0.13 interfaces
// This is needed to decode historical transactions that use the ethermint.evm.v1 namespace
func RegisterLegacyInterfaces(registry codectypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*tx.TxExtensionOptionI)(nil),
		&ExtensionOptionsEthereumTx{},
	)

	// Also register the concrete type for Any unmarshaling
	registry.RegisterInterface(
		"ethermint.evm.v1.ExtensionOptionsEthereumTx",
		(*tx.TxExtensionOptionI)(nil),
		&ExtensionOptionsEthereumTx{},
	)
}
