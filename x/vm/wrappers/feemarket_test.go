package wrappers_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	testconstants "github.com/cosmos/evm/testutil/constants"
	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	"github.com/cosmos/evm/x/vm/wrappers"
	"github.com/cosmos/evm/x/vm/wrappers/testutil"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func TestGetBaseFee(t *testing.T) {
	testCases := []struct {
		name      string
		coinInfo  evmtypes.EvmCoinInfo
		expResult *big.Int
		mockSetup func(*testutil.MockFeeMarketKeeper)
	}{
		{
			name:      "success - does not convert 18 decimals",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.ExampleChainID],
			expResult: big.NewInt(1e18), // 1 token in 18 decimals
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDec(1e18))
			},
		},
		{
			name:      "success - convert 6 decimals to 18 decimals",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: big.NewInt(1e18), // 1 token in 18 decimals
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDec(1_000_000))
			},
		},
		{
			name:      "success - nil base fee",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: nil,
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyDec{})
			},
		},
		{
			name:      "success - small amount 18 decimals",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: big.NewInt(1e12), // 0.000001 token in 18 decimals
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDec(1))
			},
		},
		{
			name:      "success - base fee is zero",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: big.NewInt(0),
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDec(0))
			},
		},
		{
			name:      "success - fix legacy encoding, base fee 7 (18 decimals)",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.ExampleChainID],
			expResult: big.NewInt(7),
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDecWithPrec(7, 18)) // old math.Int "7" deserialized as LegacyDec → 7e-18
			},
		},
		{
			name:      "success - fix legacy encoding, base fee 100 (18 decimals)",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.ExampleChainID],
			expResult: big.NewInt(100),
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDecWithPrec(100, 18)) // old math.Int "100" → 100e-18
			},
		},
		{
			name:      "success - fix legacy encoding, base fee 1 gwei (18 decimals)",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.ExampleChainID],
			expResult: big.NewInt(1e9),
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDecWithPrec(1e9, 18)) // old math.Int "1000000000" → 1e-9
			},
		},
		{
			name:      "success - fix legacy encoding, base fee 1000 gwei (18 decimals)",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.ExampleChainID],
			expResult: big.NewInt(1e12),
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDecWithPrec(1e12, 18)) // old math.Int "1000000000000" → 1e-6
			},
		},
		{
			name:      "success - fix legacy encoding, base fee 7 (6 decimals)",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: big.NewInt(7e12), // corrected to 7, then ×1e12 conversion factor
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDecWithPrec(7, 18))
			},
		},
		{
			// Regression: a legitimate base fee on a low-decimal chain sits below
			// 1.0 but at or above that chain's EIP-1559 floor of 1/ConversionFactor.
			// It must NOT be treated as a legacy math.Int encoding. Comparing
			// against a hardcoded 1.0 inflated these by 10^18.
			name:      "success - legitimate sub-one base fee is untouched (6 decimals)",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: big.NewInt(1), // 1e-12 is the floor, ×1e12 conversion factor = 1
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDecWithPrec(1, 12))
			},
		},
		{
			// Same, two decimals: floor is 1e-16.
			name:      "success - legitimate sub-one base fee is untouched (2 decimals)",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.TwoDecimalsChainID],
			expResult: big.NewInt(1), // 1e-16 is the floor, ×1e16 conversion factor = 1
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDecWithPrec(1, 16))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup EVM configurator to have access to the EVM coin info.
			configurator := evmtypes.NewEVMConfigurator()
			configurator.ResetTestConfig()
			err := configurator.WithEVMCoinInfo(tc.coinInfo).Configure()
			require.NoError(t, err, "failed to configure EVMConfigurator")

			ctrl := gomock.NewController(t)
			mockFeeMarketKeeper := testutil.NewMockFeeMarketKeeper(ctrl)
			tc.mockSetup(mockFeeMarketKeeper)

			feeMarketWrapper := wrappers.NewFeeMarketWrapper(mockFeeMarketKeeper)
			result := feeMarketWrapper.GetBaseFee(sdk.Context{}, evmtypes.Decimals(tc.coinInfo.Decimals))

			require.Equal(t, tc.expResult, result)
		})
	}
}

func TestCalculateBaseFee(t *testing.T) {
	testCases := []struct {
		name      string
		coinInfo  evmtypes.EvmCoinInfo
		baseFee   sdkmath.LegacyDec
		expResult *big.Int
		mockSetup func(*testutil.MockFeeMarketKeeper)
	}{
		{
			name:      "success - does not convert 18 decimals",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.ExampleChainID],
			expResult: big.NewInt(1e18), // 1 token in 18 decimals
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					CalculateBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDec(1e18))
			},
		},
		{
			name:      "success - convert 6 decimals to 18 decimals",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: big.NewInt(1e18), // 1 token in 18 decimals
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					CalculateBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDec(1_000_000))
			},
		},
		{
			name:      "success - nil base fee",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: nil,
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					CalculateBaseFee(gomock.Any()).
					Return(sdkmath.LegacyDec{})
			},
		},
		{
			name:      "success - small amount 18 decimals",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: big.NewInt(1e12), // 0.000001 token in 18 decimals
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					CalculateBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDec(1))
			},
		},
		{
			name:      "success - base fee is zero",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: big.NewInt(0),
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					CalculateBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDec(0))
			},
		},
		{
			name:      "success - truncate decimals with number less than 1",
			coinInfo:  testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expResult: big.NewInt(0), // 0.000001 token in 18 decimals
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					CalculateBaseFee(gomock.Any()).
					Return(sdkmath.LegacyNewDecWithPrec(1, 13)) // multiplied by 1e12 is still less than 1
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup EVM configurator to have access to the EVM coin info.
			configurator := evmtypes.NewEVMConfigurator()
			configurator.ResetTestConfig()
			err := configurator.WithEVMCoinInfo(tc.coinInfo).Configure()
			require.NoError(t, err, "failed to configure EVMConfigurator")

			ctrl := gomock.NewController(t)
			mockFeeMarketKeeper := testutil.NewMockFeeMarketKeeper(ctrl)
			tc.mockSetup(mockFeeMarketKeeper)

			feeMarketWrapper := wrappers.NewFeeMarketWrapper(mockFeeMarketKeeper)
			result := feeMarketWrapper.CalculateBaseFee(sdk.Context{})

			require.Equal(t, tc.expResult, result)
		})
	}
}

func TestGetParams(t *testing.T) {
	testCases := []struct {
		name      string
		coinInfo  evmtypes.EvmCoinInfo
		expParams feemarkettypes.Params
		mockSetup func(*testutil.MockFeeMarketKeeper)
	}{
		{
			name:     "success - convert 6 decimals to 18 decimals",
			coinInfo: testconstants.ExampleChainCoinInfo[testconstants.SixDecimalsChainID],
			expParams: feemarkettypes.Params{
				BaseFee:     sdkmath.LegacyNewDec(1e18),
				MinGasPrice: sdkmath.LegacyNewDec(1e18),
			},
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetParams(gomock.Any()).
					Return(feemarkettypes.Params{
						BaseFee:     sdkmath.LegacyNewDec(1_000_000),
						MinGasPrice: sdkmath.LegacyNewDec(1_000_000),
					})
			},
		},
		{
			name:     "success - does not convert 18 decimals",
			coinInfo: testconstants.ExampleChainCoinInfo[testconstants.ExampleChainID],
			expParams: feemarkettypes.Params{
				BaseFee:     sdkmath.LegacyNewDec(1e18),
				MinGasPrice: sdkmath.LegacyNewDec(1e18),
			},
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetParams(gomock.Any()).
					Return(feemarkettypes.Params{
						BaseFee:     sdkmath.LegacyNewDec(1e18),
						MinGasPrice: sdkmath.LegacyNewDec(1e18),
					})
			},
		},
		{
			name:     "success - nil base fee",
			coinInfo: testconstants.ExampleChainCoinInfo[testconstants.ExampleChainID],
			expParams: feemarkettypes.Params{
				MinGasPrice: sdkmath.LegacyNewDec(1e18),
			},
			mockSetup: func(mfk *testutil.MockFeeMarketKeeper) {
				mfk.EXPECT().
					GetParams(gomock.Any()).
					Return(feemarkettypes.Params{
						MinGasPrice: sdkmath.LegacyNewDec(1e18),
					})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup EVM configurator to have access to the EVM coin info.
			configurator := evmtypes.NewEVMConfigurator()
			configurator.ResetTestConfig()
			err := configurator.WithEVMCoinInfo(tc.coinInfo).Configure()
			require.NoError(t, err, "failed to configure EVMConfigurator")

			ctrl := gomock.NewController(t)
			mockFeeMarketKeeper := testutil.NewMockFeeMarketKeeper(ctrl)
			tc.mockSetup(mockFeeMarketKeeper)

			feeMarketWrapper := wrappers.NewFeeMarketWrapper(mockFeeMarketKeeper)
			result := feeMarketWrapper.GetParams(sdk.Context{})

			require.Equal(t, tc.expParams, result)
		})
	}
}
