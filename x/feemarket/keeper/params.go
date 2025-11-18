package keeper

import (
	"math/big"

	"github.com/cosmos/evm/x/feemarket/types"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// GetParams returns the total set of fee market parameters.
func (k Keeper) GetParams(ctx sdk.Context) (params types.Params) {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.ParamsKey)
	if bz == nil {
		return types.DefaultParams()
	}

	// Try to unmarshal params
	err := k.cdc.Unmarshal(bz, &params)
	if err != nil {
		// If unmarshal fails, it might be due to schema changes from upgrades
		// Log the error and return default params to maintain backward compatibility
		k.Logger(ctx).Debug("failed to unmarshal fee market params, using defaults", "error", err)
		return types.DefaultParams()
	}

	return params
}

// SetParams sets the fee market params in a single key
func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	store := ctx.KVStore(k.storeKey)
	bz, err := k.cdc.Marshal(&params)
	if err != nil {
		return err
	}

	store.Set(types.ParamsKey, bz)

	return nil
}

// GetBaseFeeEnabled returns true if base fee is enabled
func (k Keeper) GetBaseFeeEnabled(ctx sdk.Context) bool {
	params := k.GetParams(ctx)
	return !params.NoBaseFee && ctx.BlockHeight() >= params.EnableHeight
}

// ----------------------------------------------------------------------------
// Parent Base Fee
// Required by EIP1559 base fee calculation.
// ----------------------------------------------------------------------------

// GetBaseFee gets the base fee from the store
func (k Keeper) GetBaseFee(ctx sdk.Context) *big.Int {
	params := k.GetParams(ctx)
	if params.NoBaseFee {
		return nil
	}

	baseFee := params.BaseFee.BigInt()
	if baseFee == nil || baseFee.Sign() == 0 {
		// try v1 format
		return k.GetBaseFeeV1(ctx)
	}
	return baseFee
}

// SetBaseFee set's the base fee in the store
func (k Keeper) SetBaseFee(ctx sdk.Context, baseFee *big.Int) {
	params := k.GetParams(ctx)
	params.BaseFee = math.NewIntFromBigInt(baseFee)
	err := k.SetParams(ctx, params)
	if err != nil {
		return
	}
}
