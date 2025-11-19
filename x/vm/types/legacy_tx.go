package types

import (
	"fmt"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/evm/x/vm/evmos"
)

// LegacyMsgEthereumTx represents the evmos v0.13 MsgEthereumTx structure
// This is used for backward compatibility when decoding pre-upgrade transactions
// Proto definition from evmos:
//
//	message MsgEthereumTx {
//	  google.protobuf.Any data = 1;
//	  double size = 2;
//	  string hash = 3;
//	  string from = 4;
//	}
type LegacyMsgEthereumTx struct {
	Data *codectypes.Any `protobuf:"bytes,1,opt,name=data,proto3" json:"data,omitempty"`
	Size float64         `protobuf:"fixed64,2,opt,name=size,proto3" json:"-"`
	Hash string          `protobuf:"bytes,3,opt,name=hash,proto3" json:"hash,omitempty"`
	From string          `protobuf:"bytes,4,opt,name=from,proto3" json:"from,omitempty"`
}

// Reset implements proto.Message
func (m *LegacyMsgEthereumTx) Reset() { *m = LegacyMsgEthereumTx{} }

// String implements proto.Message
func (m *LegacyMsgEthereumTx) String() string { return proto.CompactTextString(m) }

// ProtoMessage implements proto.Message
func (*LegacyMsgEthereumTx) ProtoMessage() {}

// ConvertToCurrentFormat converts a legacy evmos MsgEthereumTx to the current cosmos-evm format
// It extracts the ethereum transaction from the Any field and converts it
func (m *LegacyMsgEthereumTx) ConvertToCurrentFormat() (*MsgEthereumTx, error) {
	if m == nil {
		return nil, fmt.Errorf("legacy tx is nil")
	}

	// Convert from address (hex string to bytes)
	var fromBytes []byte
	if m.From != "" {
		fromBytes = common.HexToAddress(m.From).Bytes()
	}

	// Try to extract the ethereum transaction from the Data Any field
	var ethTx *ethtypes.Transaction
	if m.Data != nil && m.Data.Value != nil {
		// Try to parse as RLP-encoded transaction
		// The Any.Value should contain the protobuf-encoded transaction data
		// We'll attempt to extract and convert it to go-ethereum format
		var err error
		ethTx, err = decodeEvmosTxData(m.Data)
		if err != nil {
			// If we can't decode, we'll just use the from address
			// The transaction can still be looked up by hash
			ethTx = nil
		}
	}

	currentMsg := &MsgEthereumTx{
		From: fromBytes,
		Raw:  EthereumTx{Transaction: ethTx},
	}

	return currentMsg, nil
}

// decodeEvmosTxData attempts to decode the evmos TxData from an Any field
// and convert it to a go-ethereum Transaction
func decodeEvmosTxData(any *codectypes.Any) (*ethtypes.Transaction, error) {
	if any == nil || any.Value == nil {
		return nil, fmt.Errorf("any is nil or empty")
	}

	// The Any.TypeUrl tells us which type of transaction this is
	// /ethermint.evm.v1.LegacyTx
	// /ethermint.evm.v1.AccessListTx
	// /ethermint.evm.v1.DynamicFeeTx

	switch any.TypeUrl {
	case "/ethermint.evm.v1.LegacyTx":
		return decodeLegacyTxData(any.Value)
	case "/ethermint.evm.v1.AccessListTx":
		return decodeAccessListTxData(any.Value)
	case "/ethermint.evm.v1.DynamicFeeTx":
		return decodeDynamicFeeTxData(any.Value)
	default:
		return nil, fmt.Errorf("unknown transaction type: %s", any.TypeUrl)
	}
}

// decodeLegacyTxData decodes a legacy transaction from proto bytes
func decodeLegacyTxData(data []byte) (*ethtypes.Transaction, error) {
	var tx evmos.LegacyTx
	if err := tx.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal legacy tx: %w", err)
	}
	return tx.ToEthereumTx(), nil
}

// decodeAccessListTxData decodes an access list transaction from proto bytes
func decodeAccessListTxData(data []byte) (*ethtypes.Transaction, error) {
	var tx evmos.AccessListTx
	if err := tx.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal access list tx: %w", err)
	}
	return tx.ToEthereumTx(), nil
}

// decodeDynamicFeeTxData decodes a dynamic fee transaction from proto bytes
func decodeDynamicFeeTxData(data []byte) (*ethtypes.Transaction, error) {
	var tx evmos.DynamicFeeTx
	if err := tx.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal dynamic fee tx: %w", err)
	}
	return tx.ToEthereumTx(), nil
} // GetFrom returns the from address as sdk.AccAddress
func (m *LegacyMsgEthereumTx) GetFrom() sdk.AccAddress {
	if m.From == "" {
		return nil
	}
	return sdk.AccAddress(common.HexToAddress(m.From).Bytes())
}

// GetEthTxHash returns the transaction hash as common.Hash
func (m *LegacyMsgEthereumTx) GetEthTxHash() common.Hash {
	if m.Hash == "" {
		return common.Hash{}
	}
	return common.HexToHash(m.Hash)
}
