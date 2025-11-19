package evmos

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
)

// RegisterLegacyInterfaces registers the legacy Evmos v0.13 interfaces
// This is needed to decode historical transactions that use the ethermint.evm.v1 namespace
func RegisterLegacyInterfaces(registry codectypes.InterfaceRegistry) {
	// Register extension option
	registry.RegisterImplementations(
		(*tx.TxExtensionOptionI)(nil),
		&ExtensionOptionsEthereumTx{},
	)

	// Register MsgEthereumTx as a message type
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgEthereumTx{},
	)
}
