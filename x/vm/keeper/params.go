package keeper

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/evm/utils"
	legacyevm "github.com/cosmos/evm/x/vm/evmos"
	"github.com/cosmos/evm/x/vm/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// GetParams returns the total set of evm parameters.
func (k Keeper) GetParams(ctx sdk.Context) (params types.Params) {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.KeyPrefixParams)
	if bz == nil {
		return params
	}

	// Try new format (post-upgrade) first
	if err := k.cdc.Unmarshal(bz, &params); err == nil {
		return params
	}

	// Fallback to legacy format (evmos-originated)
	var legacyParams legacyevm.Params
	if legacyErr := k.cdc.Unmarshal(bz, &legacyParams); legacyErr != nil {
		// Both formats failed - log the error and return default params
		// This can happen with very old data formats or corrupted state
		ctx.Logger().Error(
			"failed to unmarshal params in both new and legacy format, using defaults",
			"error", legacyErr,
			"height", ctx.BlockHeight(),
			"data_len", len(bz),
		)
		return types.DefaultParams()
	}

	// Convert legacy params to current format
	// Convert ExtraEIPs from []string to []int64
	extraEIPs := make([]int64, 0, len(legacyParams.ExtraEIPs))
	for _, eipStr := range legacyParams.ExtraEIPs {
		// evmos format: "ethereum_1234" or just "1234"
		eipStr = strings.TrimPrefix(eipStr, "ethereum_")
		eipInt, err := strconv.ParseInt(eipStr, 10, 64)
		if err != nil {
			ctx.Logger().Error("failed to parse EIP number, skipping", "eip", eipStr, "error", err)
			continue
		}
		extraEIPs = append(extraEIPs, eipInt)
	}

	// Convert legacy AccessControl to current format
	accessControl := types.AccessControl{
		Create: types.AccessControlType{
			AccessType:        types.AccessType(legacyParams.AccessControl.Create.AccessType),
			AccessControlList: legacyParams.AccessControl.Create.AccessControlList,
		},
		Call: types.AccessControlType{
			AccessType:        types.AccessType(legacyParams.AccessControl.Call.AccessType),
			AccessControlList: legacyParams.AccessControl.Call.AccessControlList,
		},
	}

	params = types.Params{
		EvmDenom:                legacyParams.EvmDenom,
		ExtraEIPs:               extraEIPs,
		EVMChannels:             legacyParams.EVMChannels,
		AccessControl:           accessControl,
		ActiveStaticPrecompiles: legacyParams.ActiveStaticPrecompiles,
		HistoryServeWindow:      types.DefaultHistoryServeWindow,
		ExtendedDenomOptions:    &types.ExtendedDenomOptions{ExtendedDenom: legacyParams.EvmDenom},
	}

	return params
}

// SetParams sets the EVM params each in their individual key for better get performance
func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	// NOTE: We need to sort the precompiles in order to enable searching with binary search
	// in params.IsActivePrecompile.
	slices.Sort(params.ActiveStaticPrecompiles)

	if err := params.Validate(); err != nil {
		return err
	}

	store := ctx.KVStore(k.storeKey)
	bz, err := k.cdc.Marshal(&params)
	if err != nil {
		return err
	}

	store.Set(types.KeyPrefixParams, bz)
	return nil
}

// EnableStaticPrecompiles appends the addresses of the given Precompiles to the list
// of active static precompiles.
func (k Keeper) EnableStaticPrecompiles(ctx sdk.Context, addresses ...common.Address) error {
	params := k.GetParams(ctx)
	activePrecompiles := params.ActiveStaticPrecompiles

	// Append and sort the new precompiles
	updatedPrecompiles, err := appendPrecompiles(activePrecompiles, addresses...)
	if err != nil {
		return err
	}

	params.ActiveStaticPrecompiles = updatedPrecompiles
	return k.SetParams(ctx, params)
}

func appendPrecompiles(existingPrecompiles []string, addresses ...common.Address) ([]string, error) {
	// check for duplicates
	hexAddresses := make([]string, len(addresses))
	for i := range addresses {
		addrHex := addresses[i].Hex()
		if slices.Contains(existingPrecompiles, addrHex) {
			return nil, fmt.Errorf("precompile already registered: %s", addrHex)
		}
		hexAddresses[i] = addrHex
	}

	existingLength := len(existingPrecompiles)
	updatedPrecompiles := make([]string, existingLength+len(hexAddresses))
	copy(updatedPrecompiles, existingPrecompiles)
	copy(updatedPrecompiles[existingLength:], hexAddresses)

	utils.SortSlice(updatedPrecompiles)
	return updatedPrecompiles, nil
}

// EnableEIPs enables the given EIPs in the EVM parameters.
func (k Keeper) EnableEIPs(ctx sdk.Context, eips ...int64) error {
	evmParams := k.GetParams(ctx)
	evmParams.ExtraEIPs = append(evmParams.ExtraEIPs, eips...)

	sort.Slice(evmParams.ExtraEIPs, func(i, j int) bool {
		return evmParams.ExtraEIPs[i] < evmParams.ExtraEIPs[j]
	})

	return k.SetParams(ctx, evmParams)
}
