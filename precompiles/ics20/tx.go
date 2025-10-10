package ics20

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"
	transfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	connectiontypes "github.com/cosmos/ibc-go/v10/modules/core/03-connection/types"
	channeltypes "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v10/modules/core/24-host"

	erc20types "github.com/cosmos/evm/x/erc20/types"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// TODO TEST suite for precompile

const (
	// TransferMethod defines the ABI method name for the ICS20 Transfer
	// transaction.
	TransferMethod = "transfer"
)

// validateV1TransferChannel does the following validation on an ibc v1 channel specified in a MsgTransfer:
// - check if the channel exists
// - check if the channel is OPEN
// - check if the underlying connection exists
// - check if the underlying connection is OPEN
func (p *Precompile) validateV1TransferChannel(ctx sdk.Context, msg *transfertypes.MsgTransfer) error {
	if msg == nil {
		return fmt.Errorf("msg cannot be nil")
	}

	if err := msg.ValidateBasic(); err != nil {
		return fmt.Errorf("msg invalid: %w", err)
	}

	// check if channel exists and is open
	channel, found := p.channelKeeper.GetChannel(ctx, msg.SourcePort, msg.SourceChannel)
	if !found {
		return errorsmod.Wrapf(
			channeltypes.ErrChannelNotFound,
			"port ID (%s) channel ID (%s)",
			msg.SourcePort,
			msg.SourceChannel,
		)
	}
	if err := channel.ValidateBasic(); err != nil {
		return fmt.Errorf("channel invalid: %w", err)
	}

	// Validate channel is in OPEN state
	if channel.State != channeltypes.OPEN {
		return errorsmod.Wrapf(
			channeltypes.ErrInvalidChannelState,
			"channel (%s) is not open, current state: %s",
			msg.SourceChannel,
			channel.State.String(),
		)
	}

	// Validate underlying connection exists and is active
	connection, err := p.channelKeeper.GetConnection(ctx, channel.ConnectionHops[0])
	if err != nil {
		return errorsmod.Wrapf(
			err,
			"connection (%s) not found for channel (%s)",
			channel.ConnectionHops[0],
			msg.SourceChannel,
		)
	}

	// Validate connection is in OPEN state
	if connection.State != connectiontypes.OPEN {
		return errorsmod.Wrapf(
			connectiontypes.ErrInvalidConnectionState,
			"connection (%s) is not open, current state: %s",
			channel.ConnectionHops[0],
			connection.State.String(),
		)
	}

	return nil
}

// Transfer implements the ICS20 transfer transactions.
func (p *Precompile) Transfer(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	msg, sender, err := NewMsgTransfer(method, args)
	if err != nil {
		return nil, err
	}

	// If the channel is in v1 format, check if channel exists and is open
	if channeltypes.IsChannelIDFormat(msg.SourceChannel) {
		if err := p.validateV1TransferChannel(ctx, msg); err != nil {
			return nil, err
		}
		// otherwise, it’s a v2 packet, so perform client ID validation
	} else if v2ClientIDErr := host.ClientIdentifierValidator(msg.SourceChannel); v2ClientIDErr != nil {
		return nil, errorsmod.Wrapf(
			channeltypes.ErrInvalidChannel,
			"invalid channel ID (%s) on v2 packet",
			msg.SourceChannel,
		)
	}

	msgSender := contract.Caller()
	if msgSender != sender {
		return nil, fmt.Errorf(cmn.ErrRequesterIsNotMsgSender, msgSender.String(), sender.String())
	}

	res, err := p.transferKeeper.Transfer(ctx, msg)
	if err != nil {
		return nil, err
	}

	if err = EmitIBCTransferEvent(
		ctx,
		stateDB,
		p.Events[EventTypeIBCTransfer],
		p.Address(),
		sender,
		msg.Receiver,
		msg.SourcePort,
		msg.SourceChannel,
		msg.Token,
		msg.Memo,
	); err != nil {
		return nil, err
	}

	// get ERC20 contract address for the token denom, and only emit the event if it is an ERC20 token (or has been registered)
	tokenPairID := p.erc20Keeper.GetTokenPairID(ctx, msg.Token.Denom)
	found := false
	var tokenPair erc20types.TokenPair

	if len(tokenPairID) != 0 {
		tokenPair, found = p.erc20Keeper.GetTokenPair(ctx, tokenPairID)

		fullDenomPath := tokenPair.Denom
		// deconstruct the token denomination into the denomination trace info
		// to determine if the sender is the source chain
		if strings.HasPrefix(tokenPair.Denom, "ibc/") {
			fullDenomPath, err = p.transferKeeper.DenomPathFromHash(ctx, tokenPair.Denom)
			if err != nil {
				return nil, err
			}
		}

		// This mimics the behavior of the IBC transfer module, by emitting transfers to the escrow address
		// if the sender is the source chain, and to the null address (burn) if the sender is the destination chain
		if found {
			erc20Addr := tokenPair.GetERC20Contract()
			if transfertypes.SenderChainIsSource(msg.SourcePort, msg.SourceChannel, fullDenomPath) {
				// obtain the escrow address for the source channel end
				escrowAddress := transfertypes.GetEscrowAddress(msg.SourcePort, msg.SourceChannel)
				escrowHexAddr := common.BytesToAddress(escrowAddress)

				// Emit Transfer event sending the tokens to the escrow address
				if err := EmitTransferEvent(ctx, stateDB, erc20Addr, sender, escrowHexAddr, msg.Token.Amount.BigInt()); err != nil {
					return nil, err
				}

			} else {
				// Emit Transfer event sending the tokens to the null address to show a burn
				if err := EmitTransferEvent(ctx, stateDB, erc20Addr, sender, common.HexToAddress("0x0000000000000000000000000000000000000000"), msg.Token.Amount.BigInt()); err != nil {
					return nil, err
				}
			}

		}
	}

	return method.Outputs.Pack(res.Sequence)
}
