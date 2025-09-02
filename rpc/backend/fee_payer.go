package backend

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	tmrpcclient "github.com/cometbft/cometbft/rpc/client"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	rpctypes "github.com/cosmos/evm/rpc/types"
	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

var baseFeeDeltaBlocks = big.NewInt(2)

type res struct {
	TxHash common.Hash
	Error  error
}

type msg struct {
	Msg      *evmtypes.MsgEthereumTx
	EvmDenom string
	Ret      chan res
}

type feePayer struct {
	ctx         context.Context
	clientCtx   client.Context
	queryClient *rpctypes.QueryClient
	logger      log.Logger

	privKey secp256k1.PrivKey
	pubKey  cryptotypes.PubKey
	address sdk.AccAddress

	messages chan msg
}

func newFeePayer(ctx context.Context, clientCtx client.Context, queryClient *rpctypes.QueryClient, logger log.Logger, feePayerPrivKey string) (fp *feePayer, err error) {
	if feePayerPrivKey == "" {
		panic("empty fee payer private key")
	}

	privKeyBytes, err := hex.DecodeString(feePayerPrivKey)
	if err != nil {
		return
	}
	privKey := secp256k1.PrivKey{
		Key: privKeyBytes,
	}

	fp = &feePayer{
		ctx:         ctx,
		clientCtx:   clientCtx,
		queryClient: queryClient,
		logger:      logger.With("module", "fee_payer"),
		privKey:     privKey,
		pubKey:      privKey.PubKey(),
		address:     sdk.AccAddress(privKey.PubKey().Address()),
		messages:    make(chan msg, 1<<14),
	}
	fp.logger.Info("node has fee payer signing enabled")
	return
}

func (fp *feePayer) enqueueMsg(m *evmtypes.MsgEthereumTx, evmDenom string) chan res {
	ret := make(chan res, 1)
	fp.messages <- msg{
		Msg:      m,
		EvmDenom: evmDenom,
		Ret:      ret,
	}
	return ret
}

func (fp *feePayer) Worker() {
	var resp *sdk.TxResponse
	var err error
	var msg msg

	var accountSeq uint64
	var accountNum uint64
	getAccount := true
	for {
		select {
		case msg = <-fp.messages:
		case <-fp.ctx.Done():
			return
		}

		if getAccount {
			accountNum, accountSeq, err = fp.clientCtx.AccountRetriever.GetAccountNumberSequence(fp.clientCtx, fp.address)
			if err != nil {
				msg.Ret <- res{
					Error: fmt.Errorf("failed to get account: %w", err),
				}
				continue
			}
			getAccount = false

			fp.logger.Info("account number and sequence updated", "account_number", accountNum, "account_sequence", accountSeq)
		}

		resp, err = fp.sendMsg(msg.Msg, msg.EvmDenom, accountNum, accountSeq)
		if err != nil {
			if resp != nil {
				err = errorsmod.ABCIError(resp.Codespace, resp.Code, resp.RawLog)
			}
			msg.Ret <- res{
				Error: err,
			}
			continue
		}

		if resp.Code != 0 && resp.Code != sdkerrors.ErrTxInMempoolCache.ABCICode() {
			if resp.Code == sdkerrors.ErrWrongSequence.ABCICode() {
				getAccount = true
			}

			msg.Ret <- res{
				Error: errorsmod.ABCIError(resp.Codespace, resp.Code, resp.RawLog),
			}
			continue
		}

		msg.Ret <- res{
			TxHash: msg.Msg.AsTransaction().Hash(),
		}

		accountSeq++
	}
}

func (fp *feePayer) calculateFeePayerFees(gas uint64) (amount sdkmath.Int, err error) {
	// Get current base fee
	var baseFee *big.Int
	blockRes, err := fp.TendermintBlockResultByNumber(nil)
	if err != nil {
		err = fmt.Errorf("failed to query latest block: %w", err)
		return
	}
	res, err := fp.queryClient.BaseFee(rpctypes.ContextWithHeight(blockRes.Height), &evmtypes.QueryBaseFeeRequest{})
	if err != nil || res.BaseFee == nil {
		err = fmt.Errorf("failed to query base fee: %w", err)
		return
	}
	if res.BaseFee.Sign() == 0 {
		sdkmath.NewInt(0)
		return
	}
	baseFee = res.BaseFee.BigInt()

	// Get fee market params
	params, err := fp.queryClient.FeeMarket.Params(fp.ctx, &feemarkettypes.QueryParamsRequest{})
	if err != nil {
		err = fmt.Errorf("failed to query params: %w", err)
		return
	}

	// Adjust to cover maximum increase of base fee in `baseFeeDeltaBlocks` blocks
	// (X(a+1)^b)/a^b where
	//   X is the original base fee
	//   a is the base fee change denominator
	//   b is `baseFeeDeltaBlocks`
	baseFeeChangeDenominator := big.NewInt(int64(params.Params.BaseFeeChangeDenominator))
	d := new(big.Int).Exp(baseFeeChangeDenominator, baseFeeDeltaBlocks, nil)
	m := new(big.Int).Exp(new(big.Int).Add(baseFeeChangeDenominator, big.NewInt(1)), baseFeeDeltaBlocks, nil)
	newBaseFee := new(big.Int).Div(new(big.Int).Mul(baseFee, m), d)
	baseFee = bigMax(
		newBaseFee,
		new(big.Int).Mul(big.NewInt(1), baseFeeDeltaBlocks), // Minimum delta is 1
	)

	gasInt := new(big.Int).SetUint64(gas)
	amount = sdkmath.NewIntFromBigInt(new(big.Int).Mul(baseFee, gasInt))
	return
}

func (fp *feePayer) buildTx(ethereumMsg *evmtypes.MsgEthereumTx, evmDenom string, accountNumber, accountSequence uint64) (cosmosTx authsigning.Tx, err error) {
	// Add the extension options to the transaction for the ethereum message
	b := fp.clientCtx.TxConfig.NewTxBuilder()
	txBuilder, ok := b.(authtx.ExtensionOptionsTxBuilder)
	if !ok {
		err = fmt.Errorf("unsupported builder: %T", b)
		return
	}
	option, err := codectypes.NewAnyWithValue(&evmtypes.ExtensionOptionsEthereumTx{})
	if err != nil {
		return
	}
	txBuilder.SetExtensionOptions(option)

	// Set gas limit from the ethereum message
	gas := ethereumMsg.GetGas()
	txBuilder.SetGasLimit(gas)

	// Overwrite user-provided fees
	feeAmt, err := fp.calculateFeePayerFees(gas)
	if err != nil {
		return
	}
	fees := make(sdk.Coins, 0, 1)
	if feeAmt.Sign() > 0 {
		fees = append(fees, sdk.NewCoin(evmDenom, feeAmt))
	}
	txBuilder.SetFeeAmount(fees)

	// A valid msg should have empty From field
	ethereumMsg.From = []byte{}

	// Set message in the transaction
	err = txBuilder.SetMsgs(ethereumMsg)
	if err != nil {
		return
	}

	// Add the fee payer information
	feepayerAddress := sdk.AccAddress(fp.pubKey.Address())
	txBuilder.SetFeePayer(feepayerAddress)

	// Make sure AuthInfo is complete before signing
	sigData := signing.SingleSignatureData{
		SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
		Signature: nil,
	}
	sigV2 := signing.SignatureV2{
		PubKey:   fp.pubKey,
		Data:     &sigData,
		Sequence: accountSequence,
	}
	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return
	}

	// Sign and set signatures
	signerData := authsigning.SignerData{
		ChainID:       fp.clientCtx.ChainID,
		AccountNumber: accountNumber,
		Sequence:      accountSequence,
	}
	sig, err := clienttx.SignWithPrivKey(
		fp.ctx,
		signing.SignMode_SIGN_MODE_DIRECT,
		signerData,
		txBuilder,
		&fp.privKey,
		fp.clientCtx.TxConfig,
		accountSequence,
	)
	if err != nil {
		err = fmt.Errorf("failed to sign transaction: %w", err)
		return
	}
	err = txBuilder.SetSignatures(sig)
	if err != nil {
		err = fmt.Errorf("failed to set signatures: %w", err)
		return
	}

	cosmosTx = txBuilder.GetTx()
	return
}

func (fp *feePayer) sendMsg(ethereumMsg *evmtypes.MsgEthereumTx, evmDenom string, accountNumber, accountSequence uint64) (txResp *sdk.TxResponse, err error) {
	cosmosTx, err := fp.buildTx(ethereumMsg, evmDenom, accountNumber, accountSequence)
	if err != nil {
		return
	}

	// Encode transaction by default Tx encoder
	txBytes, err := fp.clientCtx.TxConfig.TxEncoder()(cosmosTx)
	if err != nil {
		return
	}

	// Broadcast
	syncCtx := fp.clientCtx.WithBroadcastMode(flags.BroadcastSync)
	txResp, err = syncCtx.BroadcastTx(txBytes)
	if err != nil {
		return
	}

	return
}

// TendermintBlockResultByNumber returns a Tendermint-formatted block result
// by block number
func (fp *feePayer) TendermintBlockResultByNumber(height *int64) (*tmrpctypes.ResultBlockResults, error) {
	sc, ok := fp.clientCtx.Client.(tmrpcclient.SignClient)
	if !ok {
		return nil, errors.New("invalid rpc client")
	}
	return sc.BlockResults(fp.ctx, height)
}

func bigMax(x, y *big.Int) *big.Int {
	if x.Cmp(y) < 0 {
		return y
	}
	return x
}
