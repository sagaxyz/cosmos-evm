package evmos

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// ToEthereumTx converts a LegacyTx to a go-ethereum Transaction
func (tx *LegacyTx) ToEthereumTx() *ethtypes.Transaction {
	var to *common.Address
	if tx.To != "" {
		addr := common.HexToAddress(tx.To)
		to = &addr
	}

	v, r, s := rawSignatureValues(tx.V, tx.R, tx.S)

	var gasPrice *big.Int
	if tx.GasPrice != nil {
		gasPrice = tx.GasPrice.BigInt()
	}

	var amount *big.Int
	if tx.Amount != nil {
		amount = tx.Amount.BigInt()
	}

	return ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce:    tx.Nonce,
		GasPrice: gasPrice,
		Gas:      tx.GasLimit,
		To:       to,
		Value:    amount,
		Data:     tx.Data,
		V:        v,
		R:        r,
		S:        s,
	})
}

// ToEthereumTx converts an AccessListTx to a go-ethereum Transaction
func (tx *AccessListTx) ToEthereumTx() *ethtypes.Transaction {
	var to *common.Address
	if tx.To != "" {
		addr := common.HexToAddress(tx.To)
		to = &addr
	}

	v, r, s := rawSignatureValues(tx.V, tx.R, tx.S)

	var gasPrice *big.Int
	if tx.GasPrice != nil {
		gasPrice = tx.GasPrice.BigInt()
	}

	var amount *big.Int
	if tx.Amount != nil {
		amount = tx.Amount.BigInt()
	}

	var chainID *big.Int
	if tx.ChainID != nil {
		chainID = tx.ChainID.BigInt()
	}

	// Convert access list
	accessList := make(ethtypes.AccessList, len(tx.Accesses))
	for i, tuple := range tx.Accesses {
		storageKeys := make([]common.Hash, len(tuple.StorageKeys))
		for j, key := range tuple.StorageKeys {
			storageKeys[j] = common.HexToHash(key)
		}
		accessList[i] = ethtypes.AccessTuple{
			Address:     common.HexToAddress(tuple.Address),
			StorageKeys: storageKeys,
		}
	}

	return ethtypes.NewTx(&ethtypes.AccessListTx{
		ChainID:    chainID,
		Nonce:      tx.Nonce,
		GasPrice:   gasPrice,
		Gas:        tx.GasLimit,
		To:         to,
		Value:      amount,
		Data:       tx.Data,
		AccessList: accessList,
		V:          v,
		R:          r,
		S:          s,
	})
}

// ToEthereumTx converts a DynamicFeeTx to a go-ethereum Transaction
func (tx *DynamicFeeTx) ToEthereumTx() *ethtypes.Transaction {
	var to *common.Address
	if tx.To != "" {
		addr := common.HexToAddress(tx.To)
		to = &addr
	}

	v, r, s := rawSignatureValues(tx.V, tx.R, tx.S)

	var gasTipCap *big.Int
	if tx.GasTipCap != nil {
		gasTipCap = tx.GasTipCap.BigInt()
	}

	var gasFeeCap *big.Int
	if tx.GasFeeCap != nil {
		gasFeeCap = tx.GasFeeCap.BigInt()
	}

	var amount *big.Int
	if tx.Amount != nil {
		amount = tx.Amount.BigInt()
	}

	var chainID *big.Int
	if tx.ChainID != nil {
		chainID = tx.ChainID.BigInt()
	}

	// Convert access list
	accessList := make(ethtypes.AccessList, len(tx.Accesses))
	for i, tuple := range tx.Accesses {
		storageKeys := make([]common.Hash, len(tuple.StorageKeys))
		for j, key := range tuple.StorageKeys {
			storageKeys[j] = common.HexToHash(key)
		}
		accessList[i] = ethtypes.AccessTuple{
			Address:     common.HexToAddress(tuple.Address),
			StorageKeys: storageKeys,
		}
	}

	return ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:    chainID,
		Nonce:      tx.Nonce,
		GasTipCap:  gasTipCap,
		GasFeeCap:  gasFeeCap,
		Gas:        tx.GasLimit,
		To:         to,
		Value:      amount,
		Data:       tx.Data,
		AccessList: accessList,
		V:          v,
		R:          r,
		S:          s,
	})
}

// rawSignatureValues converts signature bytes to big.Int values
func rawSignatureValues(vBz, rBz, sBz []byte) (v, r, s *big.Int) {
	if len(vBz) > 0 {
		v = new(big.Int).SetBytes(vBz)
	}
	if len(rBz) > 0 {
		r = new(big.Int).SetBytes(rBz)
	}
	if len(sBz) > 0 {
		s = new(big.Int).SetBytes(sBz)
	}
	return v, r, s
}
