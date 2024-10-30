package lib

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	errorsmod "cosmossdk.io/errors"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
)

const ERROR_MESSAGE_EMA_ALREADY_SENT = "already submitted"
const ERROR_MESSAGE_TX_INCLUDED_IN_BLOCK = "waiting for next block"
const ERROR_MESSAGE_ACCOUNT_SEQUENCE_MISMATCH = "account sequence mismatch"
const ERROR_MESSAGE_ABCI_ERROR_CODE_MARKER = "error code:"
const EXCESS_FEES_CORRECTION = 20000

// SendDataWithRetry attempts to send data, handling retries, with fee awareness.
// Custom handling for different errors.
func (node *NodeConfig) SendDataWithRetry(ctx context.Context, req sdktypes.Msg, infoMsg string) (*cosmosclient.Response, error) {
	var txResp *cosmosclient.Response
	var hadEOFTxError bool
	// Excess fees correction factor in fees
	excessFactorFees := float64(EXCESS_FEES_CORRECTION) * node.Wallet.GasPrices

	for retryCount := int64(0); retryCount <= node.Wallet.MaxRetries; retryCount++ {

		txOptions := cosmosclient.TxOptions{}
		txService, err := node.Chain.Client.CreateTxWithOptions(ctx, node.Chain.Account, txOptions, req)
		if err != nil {
			return nil, err
		}

		if node.Wallet.GasPrices > 0 {
			// Precalculate fees
			fees := uint64(float64(txService.Gas()+EXCESS_FEES_CORRECTION) * node.Wallet.GasPrices)
			// Add excess fees correction factor to increase with each retry
			fees = fees + uint64(float64(retryCount+1)*excessFactorFees)
			// Limit fees to maxFees
			if fees > node.Wallet.MaxFees {
				log.Warn().Uint64("gas", txService.Gas()).Uint64("limit", node.Wallet.MaxFees).Msg("Gas limit exceeded, using maxFees instead")
				fees = node.Wallet.MaxFees
			}
			txOptions := cosmosclient.TxOptions{
				Fees: fmt.Sprintf("%duallo", fees),
			}
			log.Info().Str("fees", txOptions.Fees).Msg("Attempting tx with calculated fees")
			txService, err = node.Chain.Client.CreateTxWithOptions(ctx, node.Chain.Account, txOptions, req)
			if err != nil {
				return nil, err
			}
		}
		txResponse, err := txService.Broadcast(ctx)
		if err == nil {
			log.Info().Str("msg", infoMsg).Str("txHash", txResponse.TxHash).Msg("Success")
			return txResp, nil
		}
		txResp = &txResponse
		if strings.Contains(err.Error(), ERROR_MESSAGE_ABCI_ERROR_CODE_MARKER) {
			re := regexp.MustCompile(`error code: '(\d+)'`)
			matches := re.FindStringSubmatch(err.Error())
			if len(matches) == 2 {
				errorCode, parseErr := strconv.Atoi(matches[1])
				if parseErr != nil {
					log.Error().Err(parseErr).Str("msg", infoMsg).Msg("Failed to parse ABCI error code")
				} else {
					switch errorCode {
					case int(sdkerrors.ErrMempoolIsFull.ABCICode()):
						log.Warn().
							Err(err).
							Str("msg", infoMsg).
							Msg("Mempool is full, retrying with exponential backoff")
						delay := time.Duration(math.Pow(float64(node.Wallet.RetryDelay), float64(retryCount))) * time.Second
						time.Sleep(delay)
						continue
					case int(sdkerrors.ErrWrongSequence.ABCICode()), int(sdkerrors.ErrInvalidSequence.ABCICode()):
						log.Warn().
							Err(err).
							Str("msg", infoMsg).
							Int64("delay", node.Wallet.AccountSequenceRetryDelay).
							Msg("Account sequence mismatch detected, retrying with fixed delay")
						// Wait a fixed block-related waiting time
						time.Sleep(time.Duration(node.Wallet.AccountSequenceRetryDelay) * time.Second)
						continue
					case int(sdkerrors.ErrInsufficientFee.ABCICode()):
						log.Warn().Str("msg", infoMsg).Msg("Insufficient fee")
						continue
					case int(sdkerrors.ErrTxTooLarge.ABCICode()):
						return nil, errorsmod.Wrapf(err, "tx too large")
					case int(sdkerrors.ErrTxInMempoolCache.ABCICode()):
						return nil, errorsmod.Wrapf(err, "tx already in mempool cache")
					case int(sdkerrors.ErrInvalidChainID.ABCICode()):
						return nil, errorsmod.Wrapf(err, "invalid chain-id")
					default:
						log.Info().Str("msg", infoMsg).Msg("ABCI error, but not special case - regular retry")
					}
				}
			} else {
				log.Error().Str("msg", infoMsg).Msg("Unmatched error format, cannot classify as ABCI error")
			}
		}

		// NOT ABCI error code: keep on checking for specially handled error types
		if strings.Contains(err.Error(), ERROR_MESSAGE_ACCOUNT_SEQUENCE_MISMATCH) {
			log.Warn().Str("msg", infoMsg).Msg("Account sequence mismatch detected, re-fetching sequence")
			time.Sleep(time.Duration(node.Wallet.AccountSequenceRetryDelay) * time.Second)
			continue
		} else if strings.Contains(err.Error(), ERROR_MESSAGE_TX_INCLUDED_IN_BLOCK) {
			// First time seeing this error, set up the EOFTxError flag and retry normally
			if !hadEOFTxError {
				hadEOFTxError = true
				log.Warn().Err(err).Str("msg", infoMsg).Msg("Tx sent, waiting for tx to be included in a block, regular retry")
			}
			// Wait for the next block
		} else if strings.Contains(err.Error(), ERROR_MESSAGE_EMA_ALREADY_SENT) {
			if hadEOFTxError {
				log.Info().Str("msg", infoMsg).Msg("Confirmation: the tx sent for this epoch has been accepted")
			} else {
				log.Info().Err(err).Str("msg", infoMsg).Msg("Already sent data for this epoch.")
			}
			return txResp, nil
		}
		log.Error().Err(err).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, node.Wallet.MaxRetries)
		// Wait for the uniform delay before retrying
		time.Sleep(time.Duration(node.Wallet.RetryDelay) * time.Second)
	}

	return nil, errors.New("Tx not able to complete after failing max retries")
}
