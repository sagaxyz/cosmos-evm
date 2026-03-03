package evmos

import (
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"

	"github.com/cosmos/evm/x/erc20/types"
)

// RouterKey is the module name router key for legacy amino compatibility
const RouterKey = types.RouterKey

// Compile-time interface checks to ensure the proto-generated legacy Evmos types
// implement the governance Content interface
var (
	_ govv1beta1.Content = (*RegisterERC20Proposal)(nil)
	_ govv1beta1.Content = (*ToggleTokenConversionProposal)(nil)
	_ govv1beta1.Content = (*RegisterCoinProposal)(nil)
)

// Content interface implementations for RegisterERC20Proposal

func (p *RegisterERC20Proposal) ProposalRoute() string { return RouterKey }
func (p *RegisterERC20Proposal) ProposalType() string  { return types.ProposalTypeRegisterERC20 }
func (p *RegisterERC20Proposal) ValidateBasic() error {
	return govv1beta1.ValidateAbstract(p)
}

// Content interface implementations for ToggleTokenConversionProposal

func (p *ToggleTokenConversionProposal) ProposalRoute() string { return RouterKey }
func (p *ToggleTokenConversionProposal) ProposalType() string {
	return types.ProposalTypeToggleTokenConversion
}
func (p *ToggleTokenConversionProposal) ValidateBasic() error {
	return govv1beta1.ValidateAbstract(p)
}

// Content interface implementations for RegisterCoinProposal

func (p *RegisterCoinProposal) ProposalRoute() string { return RouterKey }
func (p *RegisterCoinProposal) ProposalType() string  { return types.ProposalTypeRegisterCoin }
func (p *RegisterCoinProposal) ValidateBasic() error {
	return govv1beta1.ValidateAbstract(p)
}

// RegisterInterfaces registers the legacy Evmos proposal types to handle
// deserialization of proposals stored with the legacy /evmos.erc20.v1.* type URLs
func RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*govv1beta1.Content)(nil),
		&RegisterERC20Proposal{},
		&ToggleTokenConversionProposal{},
		&RegisterCoinProposal{},
	)
}

// RegisterLegacyAminoCodec registers the Evmos legacy proposal types for amino encoding.
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&RegisterERC20Proposal{}, "evmos/RegisterERC20Proposal", nil)
	cdc.RegisterConcrete(&ToggleTokenConversionProposal{}, "evmos/ToggleTokenConversionProposal", nil)
	cdc.RegisterConcrete(&RegisterCoinProposal{}, "evmos/RegisterCoinProposal", nil)
}
