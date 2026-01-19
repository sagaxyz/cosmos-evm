package backend

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/pkg/errors"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtrpctypes "github.com/cometbft/cometbft/rpc/core/types"

	rpctypes "github.com/cosmos/evm/rpc/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// RPCBlockFromCometBlock returns a JSON-RPC compatible Ethereum block from a
// given CometBFT block and its block result.
func (b *Backend) RPCHeaderFromCometBlock(
	resBlock *cmtrpctypes.ResultBlock,
	blockRes *cmtrpctypes.ResultBlockResults,
) (map[string]interface{}, error) {
	ethBlock, err := b.EthBlockFromCometBlock(resBlock, blockRes)
	if err != nil {
		return nil, fmt.Errorf("failed to get rpc block from comet block: %w", err)
	}

	return rpctypes.RPCMarshalHeader(ethBlock.Header(), resBlock.BlockID.Hash), nil
}

// RPCBlockFromCometBlock returns a JSON-RPC compatible Ethereum block from a
// given CometBFT block and its block result.
func (b *Backend) RPCBlockFromCometBlock(
	resBlock *cmtrpctypes.ResultBlock,
	blockRes *cmtrpctypes.ResultBlockResults,
	fullTx bool,
) (map[string]interface{}, error) {
	msgs := b.EthMsgsFromCometBlock(resBlock, blockRes)
	ethBlock, err := b.EthBlockFromCometBlock(resBlock, blockRes)
	if err != nil {
		return nil, fmt.Errorf("failed to get rpc block from comet block: %w", err)
	}

	return rpctypes.RPCMarshalBlock(ethBlock, resBlock, msgs, true, fullTx, b.ChainConfig())
}

// BlockNumberFromComet returns the BlockNumber from BlockNumberOrHash
func (b *Backend) BlockNumberFromComet(blockNrOrHash rpctypes.BlockNumberOrHash) (rpctypes.BlockNumber, error) {
	switch {
	case blockNrOrHash.BlockHash == nil && blockNrOrHash.BlockNumber == nil:
		return rpctypes.EthEarliestBlockNumber, fmt.Errorf("types BlockHash and BlockNumber cannot be both nil")
	case blockNrOrHash.BlockHash != nil:
		blockNumber, err := b.BlockNumberFromCometByHash(*blockNrOrHash.BlockHash)
		if err != nil {
			return rpctypes.EthEarliestBlockNumber, err
		}
		return rpctypes.NewBlockNumber(blockNumber), nil
	case blockNrOrHash.BlockNumber != nil:
		return *blockNrOrHash.BlockNumber, nil
	default:
		return rpctypes.EthEarliestBlockNumber, nil
	}
}

// BlockNumberFromCometByHash returns the block height of given block hash
func (b *Backend) BlockNumberFromCometByHash(blockHash common.Hash) (*big.Int, error) {
	resHeader, err := b.RPCClient.HeaderByHash(b.Ctx, blockHash.Bytes())
	if err != nil {
		return nil, err
	}

	if resHeader == nil || resHeader.Header == nil {
		return nil, errors.Errorf("header not found for hash %s", blockHash.Hex())
	}

	return big.NewInt(resHeader.Header.Height), nil
}

// EthMsgWithContext wraps an Ethereum message with its block-local context
// needed for building receipts without relying on the indexer.
type EthMsgWithContext struct {
	Msg        *evmtypes.MsgEthereumTx
	TxIndex    int                // Cosmos tx index in the block
	MsgIndex   int                // Message index within the Cosmos tx
	EthTxIndex int32              // Sequential Ethereum tx index in the block
	TxResult   *abci.ExecTxResult // The execution result for this Cosmos tx
}

// EthMsgsFromCometBlock returns all real MsgEthereumTxs from a
// CometBFT block. It also ensures consistency over the correct txs indexes
// across RPC endpoints
func (b *Backend) EthMsgsFromCometBlock(
	resBlock *cmtrpctypes.ResultBlock,
	blockRes *cmtrpctypes.ResultBlockResults,
) []*evmtypes.MsgEthereumTx {
	msgsWithCtx := b.EthMsgsWithContextFromCometBlock(resBlock, blockRes)
	result := make([]*evmtypes.MsgEthereumTx, len(msgsWithCtx))
	for i, msgCtx := range msgsWithCtx {
		result[i] = msgCtx.Msg
	}
	return result
}

// EthMsgsWithContextFromCometBlock returns all real MsgEthereumTxs from a
// CometBFT block along with their block-local context (tx index, msg index, etc.).
// This allows building receipts without relying on the indexer.
func (b *Backend) EthMsgsWithContextFromCometBlock(
	resBlock *cmtrpctypes.ResultBlock,
	blockRes *cmtrpctypes.ResultBlockResults,
) []EthMsgWithContext {
	var result []EthMsgWithContext
	block := resBlock.Block

	txResults := blockRes.TxsResults
	var ethTxIndex int32

	for i, tx := range block.Txs {
		// Check if tx exists on EVM by cross checking with blockResults:
		//  - Include unsuccessful tx that exceeds block gas limit
		//  - Include unsuccessful tx that failed when committing changes to stateDB
		//  - Exclude unsuccessful tx with any other error but ExceedBlockGasLimit
		if !rpctypes.TxSucessOrExpectedFailure(txResults[i]) {
			b.Logger.Debug("invalid tx result code", "cosmos-hash", hexutil.Encode(tx.Hash()))
			continue
		}

		decodedTx, err := b.ClientCtx.TxConfig.TxDecoder()(tx)
		if err != nil {
			// Try legacy format
			decodedTx, err = decodeLegacyTx(b.ClientCtx.TxConfig.TxDecoder(), tx)
			if err != nil {
				b.Logger.Debug("failed to decode transaction in block", "height", block.Height, "error", err.Error())
				continue
			}
		}

		for msgIndex, msg := range decodedTx.GetMsgs() {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				continue
			}

			result = append(result, EthMsgWithContext{
				Msg:        ethMsg,
				TxIndex:    i,
				MsgIndex:   msgIndex,
				EthTxIndex: ethTxIndex,
				TxResult:   txResults[i],
			})
			ethTxIndex++
		}
	}

	return result
}

// RPCBlockFromCometBlock returns a JSON-RPC compatible Ethereum block from a
// given CometBFT block and its block result.
func (b *Backend) EthBlockFromCometBlock(
	resBlock *cmtrpctypes.ResultBlock,
	blockRes *cmtrpctypes.ResultBlockResults,
) (*ethtypes.Block, error) {
	cmtBlock := resBlock.Block

	// 1. get base fee
	baseFee, err := b.BaseFee(blockRes)
	if err != nil {
		// handle the error for pruned node.
		b.Logger.Error("failed to fetch Base Fee from prunned block. Check node prunning configuration", "height", cmtBlock.Height, "error", err)
	}

	// 2. get miner
	miner, err := b.MinerFromCometBlock(resBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to get miner(block proposer) address from comet block")
	}

	// 3. get block gasLimit
	ctx := rpctypes.ContextWithHeight(cmtBlock.Height)
	gasLimit, err := rpctypes.BlockMaxGasFromConsensusParams(ctx, b.ClientCtx, cmtBlock.Height)
	if err != nil {
		b.Logger.Error("failed to query consensus params", "error", err.Error())
	}

	// 4. create blockHeader without transactions, receipts, withdrawals, ...
	ethHeader := rpctypes.MakeHeader(cmtBlock.Header, gasLimit, miner, baseFee)

	// 5. get MsgEthereumTxs with their block-local context
	msgsWithCtx := b.EthMsgsWithContextFromCometBlock(resBlock, blockRes)
	txs := make([]*ethtypes.Transaction, len(msgsWithCtx))
	for i, msgCtx := range msgsWithCtx {
		txs[i] = msgCtx.Msg.AsTransaction()
	}

	// 6. create ethBlock body with transactions
	body := &ethtypes.Body{
		Transactions: txs,
		Uncles:       []*ethtypes.Header{},
		Withdrawals:  []*ethtypes.Withdrawal{},
	}

	// 7. receipts
	receipts, err := b.ReceiptsFromCometBlock(resBlock, blockRes, msgsWithCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get receipts from comet block: %w", err)
	}

	// 8. Gas Used
	gasUsed := uint64(0)
	for _, txsResult := range blockRes.TxsResults {
		// workaround for cosmos-sdk bug. https://github.com/cosmos/cosmos-sdk/issues/10832
		if ShouldIgnoreGasUsed(txsResult) {
			// block gas limit has exceeded, other txs must have failed with same reason.
			break
		}
		gasUsed += uint64(txsResult.GetGasUsed()) // #nosec G115 -- checked for int overflow already
	}
	ethHeader.GasUsed = gasUsed

	// 9. create eth block
	ethBlock := ethtypes.NewBlock(ethHeader, body, receipts, trie.NewStackTrie(nil))
	return ethBlock, nil
}

func (b *Backend) MinerFromCometBlock(
	resBlock *cmtrpctypes.ResultBlock,
) (common.Address, error) {
	cmtBlock := resBlock.Block

	req := &evmtypes.QueryValidatorAccountRequest{
		ConsAddress: sdk.ConsAddress(cmtBlock.Header.ProposerAddress).String(),
	}

	var validatorAccAddr sdk.AccAddress

	ctx := rpctypes.ContextWithHeight(cmtBlock.Height)
	res, err := b.QueryClient.ValidatorAccount(ctx, req)
	if err != nil {
		b.Logger.Debug(
			"failed to query validator operator address",
			"height", cmtBlock.Height,
			"cons-address", req.ConsAddress,
			"error", err.Error(),
		)
		// use zero address as the validator operator address
		validatorAccAddr = sdk.AccAddress(common.Address{}.Bytes())
	} else {
		validatorAccAddr, err = sdk.AccAddressFromBech32(res.AccountAddress)
		if err != nil {
			return common.Address{}, err
		}
	}

	return common.BytesToAddress(validatorAccAddr), nil
}

func (b *Backend) ReceiptsFromCometBlock(
	resBlock *cmtrpctypes.ResultBlock,
	blockRes *cmtrpctypes.ResultBlockResults,
	msgsWithCtx []EthMsgWithContext,
) ([]*ethtypes.Receipt, error) {
	baseFee, err := b.BaseFee(blockRes)
	if err != nil {
		// handle the error for pruned node.
		b.Logger.Error("failed to fetch Base Fee from prunned block. Check node prunning configuration", "height", resBlock.Block.Height, "error", err)
	}

	blockHash := common.BytesToHash(resBlock.BlockID.Hash)
	receipts := make([]*ethtypes.Receipt, len(msgsWithCtx))
	cumulatedGasUsed := uint64(0)
	for i, msgCtx := range msgsWithCtx {
		ethMsg := msgCtx.Msg

		// Parse gas used and failed status from block-local tx result events.
		// This avoids relying on the indexer, which can have stale data for duplicate tx hashes.
		var gasUsed uint64
		var failed bool
		if msgCtx.TxResult.Code != 0 {
			// Transaction failed - use gas limit as that's what's charged
			gasUsed = ethMsg.GetGas()
			failed = true
		} else {
			// Parse from events to get actual gas used.
			// For success case (Code == 0), ParseTxResult doesn't need the decoded tx,
			// it only parses from the result events.
			parsedTxs, err := rpctypes.ParseTxResult(msgCtx.TxResult, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to parse tx result events: %w", err)
			}
			parsedTx := parsedTxs.GetTxByMsgIndex(msgCtx.MsgIndex)
			if parsedTx == nil {
				// Fallback: use total gas from result (works for single-msg txs)
				gasUsed = uint64(msgCtx.TxResult.GasUsed) // #nosec G115
				failed = false
			} else {
				gasUsed = parsedTx.GasUsed
				failed = parsedTx.Failed
			}
		}

		cumulatedGasUsed += gasUsed

		var effectiveGasPrice *big.Int
		if baseFee != nil {
			effectiveGasPrice = rpctypes.EffectiveGasPrice(ethMsg.Raw.Transaction, baseFee)
		} else {
			effectiveGasPrice = ethMsg.Raw.GasFeeCap()
		}

		var status uint64
		if failed {
			status = ethtypes.ReceiptStatusFailed
		} else {
			status = ethtypes.ReceiptStatusSuccessful
		}

		contractAddress := common.Address{}
		if ethMsg.Raw.To() == nil {
			contractAddress = crypto.CreateAddress(ethMsg.GetSender(), ethMsg.Raw.Nonce())
		}

		logs, err := evmtypes.DecodeMsgLogs(
			msgCtx.TxResult.Data,
			msgCtx.MsgIndex,
			uint64(resBlock.Block.Height), // #nosec G115 -- checked for int overflow already
		)
		if err != nil {
			return nil, fmt.Errorf("failed to convert tx result to eth receipt: %w", err)
		}

		bloom := ethtypes.CreateBloom(&ethtypes.Receipt{Logs: logs})

		receipt := &ethtypes.Receipt{
			// Consensus fields: These fields are defined by the Yellow Paper
			Type:              ethMsg.Raw.Type(),
			PostState:         nil,
			Status:            status, // convert to 1=success, 0=failure
			CumulativeGasUsed: cumulatedGasUsed,
			Bloom:             bloom,
			Logs:              logs,

			// Implementation fields: These fields are added by geth when processing a transaction.
			TxHash:            ethMsg.Hash(),
			ContractAddress:   contractAddress,
			GasUsed:           gasUsed,
			EffectiveGasPrice: effectiveGasPrice,
			BlobGasUsed:       uint64(0),     // TODO: fill this field
			BlobGasPrice:      big.NewInt(0), // TODO: fill this field

			// Inclusion information: These fields provide information about the inclusion of the
			// transaction corresponding to this receipt.
			BlockHash:        blockHash,
			BlockNumber:      big.NewInt(resBlock.Block.Height),
			TransactionIndex: uint(msgCtx.EthTxIndex), // #nosec G115 -- checked for int overflow already
		}

		receipts[i] = receipt
	}

	return receipts, nil
}

// BlockBloom query block bloom filter from block results
func (b *Backend) BlockBloomFromCometBlock(blockRes *cmtrpctypes.ResultBlockResults) (ethtypes.Bloom, error) {
	for _, event := range blockRes.FinalizeBlockEvents {
		if event.Type != evmtypes.EventTypeBlockBloom {
			continue
		}

		for _, attr := range event.Attributes {
			if attr.Key == evmtypes.AttributeKeyEthereumBloom {
				return ethtypes.BytesToBloom([]byte(attr.Value)), nil
			}
		}
	}
	return ethtypes.Bloom{}, errors.New("block bloom event is not found")
}
