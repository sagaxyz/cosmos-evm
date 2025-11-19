package erc20

import (
	"math/big"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestZeroAmount(t *testing.T) {
	amount := big.NewInt(0)
	coin := sdk.Coin{Denom: "test", Amount: math.NewIntFromBigInt(amount)}
	err := coin.Validate()
	require.NoError(t, err)

	// coins should fail
	coins := sdk.Coins{coin}
	err = coins.Validate()
	require.Error(t, err)
}
