package wrappers

import (
	"math/big"

	sdkmath "cosmossdk.io/math"

	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	"github.com/cosmos/evm/x/vm/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// legacyDecPrecisionMultiplier is 10^18, used to correct the base_fee
// encoding mismatch when querying historical state from before the
// math.Int → LegacyDec migration.
var legacyDecPrecisionMultiplier = sdkmath.NewIntWithDecimal(1, sdkmath.LegacyPrecision)

// fixLegacyBaseFeeEncoding detects and corrects a base fee that was stored as
// math.Int but deserialized as LegacyDec. When the proto type changed from
// math.Int to math.LegacyDec, historical state kept the old wire bytes.
// Unmarshaling "N" (an Int) as LegacyDec sets the internal big.Int to N,
// yielding a decimal value of N×10⁻¹⁸ instead of N.
//
// Detection: EIP-1559 enforces a floor of minUnitGas = 1/ConversionFactor on
// the base fee. For 18-decimal chains (ConversionFactor=1) this floor is 1.0,
// so any legitimate base fee is ≥ 1.0. A misinterpreted old Int N always
// produces N×10⁻¹⁸ < 1.0 (for any N < 10^18, which covers all practical
// base fees). The fix multiplies by 10^18 to restore the original value.
//
// For chains with fewer decimals the floor is lower (e.g. 10⁻¹² for
// 6-decimal chains), but the heuristic remains safe as long as the old
// Int base fee was smaller than the chain's ConversionFactor — which holds
// for all known Saga-based deployments where 18 decimals are used.
func fixLegacyBaseFeeEncoding(baseFee sdkmath.LegacyDec) sdkmath.LegacyDec {
	if baseFee.IsPositive() && baseFee.LT(sdkmath.LegacyOneDec()) {
		return baseFee.MulInt(legacyDecPrecisionMultiplier)
	}
	return baseFee
}

// FeeMarketWrapper is a wrapper around the feemarket keeper
// that is used to manage an evm denom with 6 or 18 decimals.
// The wrapper makes the corresponding conversions to achieve:
//   - With the EVM, the wrapper works always with 18 decimals.
//   - With the feemarket module, the wrapper works always
//     with the bank module decimals (either 6 or 18).
type FeeMarketWrapper struct {
	types.FeeMarketKeeper
}

// NewFeeMarketWrapper creates a new feemarket Keeper wrapper instance.
// The BankWrapper is used to manage an evm denom with 6 or 18 decimals.
func NewFeeMarketWrapper(
	fk types.FeeMarketKeeper,
) *FeeMarketWrapper {
	return &FeeMarketWrapper{
		fk,
	}
}

// GetBaseFee returns the base fee converted to 18 decimals.
func (w FeeMarketWrapper) GetBaseFee(ctx sdk.Context, decimals types.Decimals) *big.Int {
	baseFee := w.FeeMarketKeeper.GetBaseFee(ctx)
	if baseFee.IsNil() {
		return nil
	}

	baseFee = fixLegacyBaseFeeEncoding(baseFee)

	return baseFee.MulInt(decimals.ConversionFactor()).TruncateInt().BigInt()
}

// CalculateBaseFee returns the calculated base fee converted to 18 decimals.
func (w FeeMarketWrapper) CalculateBaseFee(ctx sdk.Context) *big.Int {
	baseFee := w.FeeMarketKeeper.CalculateBaseFee(ctx)
	if baseFee.IsNil() {
		return nil
	}
	return types.ConvertAmountTo18DecimalsLegacy(baseFee).TruncateInt().BigInt()
}

// GetParams returns the params with associated fees values converted to 18 decimals.
func (w FeeMarketWrapper) GetParams(ctx sdk.Context) feemarkettypes.Params {
	params := w.FeeMarketKeeper.GetParams(ctx)
	if !params.BaseFee.IsNil() {
		params.BaseFee = fixLegacyBaseFeeEncoding(params.BaseFee)
		params.BaseFee = types.ConvertAmountTo18DecimalsLegacy(params.BaseFee)
	}
	params.MinGasPrice = types.ConvertAmountTo18DecimalsLegacy(params.MinGasPrice)
	return params
}
