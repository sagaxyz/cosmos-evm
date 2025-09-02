package evm

import (
	"errors"
	"math/big"
	"strings"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	anteinterfaces "github.com/cosmos/evm/ante/interfaces"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx"
)

// ValidateMsg validates an Ethereum specific message type and returns an error
// if invalid. It checks the following requirements:
// - If the transaction is a contract creation or call, the corresponding operation must be enabled in the EVM parameters
func ValidateMsg(
	evmParams evmtypes.Params,
	ethTx *ethtypes.Transaction,
) error {
	if ethTx == nil {
		return errorsmod.Wrap(errortypes.ErrInvalidRequest, "transaction is nil")
	}
	return checkDisabledCreateCall(
		ethTx,
		&evmParams.AccessControl,
	)
}

// checkDisabledCreateCall checks if the transaction is a contract creation or call,
// and if those actions are disabled through governance.
func checkDisabledCreateCall(
	ethTx *ethtypes.Transaction,
	permissions *evmtypes.AccessControl,
) error {
	to := ethTx.To()
	blockCreate := permissions.Create.AccessType == evmtypes.AccessTypeRestricted
	blockCall := permissions.Call.AccessType == evmtypes.AccessTypeRestricted

	// return error if contract creation or call are disabled
	// and the transaction is trying to create a contract or call a contract.
	if blockCreate && to == nil {
		return errorsmod.Wrap(evmtypes.ErrCreateDisabled, "failed to create new contract")
	} else if blockCall && to != nil {
		return errorsmod.Wrap(evmtypes.ErrCallDisabled, "failed to perform a call")
	}
	return nil
}

// ValidateTx validates an Ethereum specific transaction type and returns an error if invalid.
//
// FIXME: this shouldn't be required if the tx was an Ethereum transaction type.
func ValidateTx(tx sdktypes.Tx) (*tx.Fee, error) {
	var feePayer bool
	if t, ok := tx.(sdktypes.HasValidateBasic); ok {
		err := t.ValidateBasic()
		if err != nil {
			if errors.Is(err, errortypes.ErrNoSignatures) {
				// ErrNoSignatures is fine with eth tx
			} else if errors.Is(err, errortypes.ErrUnauthorized) && strings.HasPrefix(err.Error(), "wrong number of signers") {
				// One missing signature is also fine when fee payer is used
				feePayer = true
			} else {
				return nil, errorsmod.Wrap(err, "tx basic validation failed")
			}
		}
	}

	// For eth type cosmos tx, some fields should be verified as zero values,
	// since we will only verify the signature against the hash of the MsgEthereumTx.Data
	wrapperTx, ok := tx.(anteinterfaces.ProtoTxProvider)
	if !ok {
		return nil, errorsmod.Wrapf(errortypes.ErrUnknownRequest, "invalid tx type %T, didn't implement interface ProtoTxProvider", tx)
	}

	protoTx := wrapperTx.GetProtoTx()
	body := protoTx.Body
	if body.Memo != "" || body.TimeoutHeight != uint64(0) || len(body.NonCriticalExtensionOptions) > 0 {
		return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest,
			"for eth tx body Memo TimeoutHeight NonCriticalExtensionOptions should be empty")
	}

	if len(body.ExtensionOptions) != 1 {
		return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx length of ExtensionOptions should be 1")
	}

	authInfo := protoTx.AuthInfo
	if authInfo.Fee.Granter != "" {
		return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx AuthInfo Fee granter should be empty")
	}

	if feePayer {
		if len(authInfo.SignerInfos) != 1 {
			return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx AuthInfo SignerInfos can only be the fee payer")
		}

		if authInfo.Fee.Payer == "" {
			return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx with additional signatures AuthInfo Fee payer has to be non-empty")
		}

		sigs := protoTx.Signatures
		if len(sigs) == 0 {
			return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest, "missing fee payer signature")
		}
		if len(sigs) > 1 {
			return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx Signatures can only contain the fee payer")
		}
	} else {
		if len(authInfo.SignerInfos) > 0 {
			return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx AuthInfo SignerInfos should be empty")
		}
		if authInfo.Fee.Payer != "" {
			return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx AuthInfo Fee payer should be empty")
		}

		sigs := protoTx.Signatures
		if len(sigs) > 0 {
			return nil, errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx Signatures should be empty")
		}
	}

	return authInfo.Fee, nil
}

// CheckTxFee checks if the Amount and GasLimit fields of the txFeeInfo input
// are equal to the txFee coins and the txGasLimit value.
// The function expects txFeeInfo to contains coins in the original decimal
// representation.
func CheckTxFee(txFeeInfo *tx.Fee, txFee *big.Int, txGasLimit uint64) error {
	if txFeeInfo == nil {
		return nil
	}

	// NOTE: When an evm tx comes in, it goes through the process of converting it
	// to MsgEthereumTx, which is a sdk tx. Here, the denom will be a uatom, not aatom.
	// BuildTx then converts uatom to aatom meaning that logic that interacts with the user
	// will use uatom and internal processing such as the ante handler will operate based on aatom.
	evmExtendedDenom := evmtypes.GetEVMCoinExtendedDenom()
	if !txFeeInfo.Amount.AmountOf(evmExtendedDenom).Equal(sdkmath.NewIntFromBigInt(txFee)) {
		return errorsmod.Wrapf(errortypes.ErrInvalidRequest, "invalid AuthInfo Fee Amount (%s != %s)", txFeeInfo.Amount, txFee)
	}

	if txFeeInfo.GasLimit != txGasLimit {
		return errorsmod.Wrapf(errortypes.ErrInvalidRequest, "invalid AuthInfo Fee GasLimit (%d != %d)", txFeeInfo.GasLimit, txGasLimit)
	}

	return nil
}
