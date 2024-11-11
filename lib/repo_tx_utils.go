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

const ERROR_MESSAGE_DATA_ALREADY_SUBMITTED = "already submitted"
const ERROR_MESSAGE_CANNOT_UPDATE_EMA = "cannot update EMA"
const ERROR_MESSAGE_WAITING_FOR_NEXT_BLOCK = "waiting for next block" // This means tx is accepted in mempool but not yet included in a block
const ERROR_MESSAGE_ACCOUNT_SEQUENCE_MISMATCH = "account sequence mismatch"
const ERROR_MESSAGE_ABCI_ERROR_CODE_MARKER = "error code:"
const EXCESS_CORRECTION_IN_GAS = 20000

const ERROR_PROCESSING_CONTINUE = "continue"
const ERROR_PROCESSING_OK = "ok"
const ERROR_PROCESSING_FEES = "fees"
const ERROR_PROCESSING_ERROR = "error"

// calculateExponentialBackoffDelay returns a duration based on retry count and base delay
func calculateExponentialBackoffDelay(baseDelay int64, retryCount int64) time.Duration {
	return time.Duration(math.Pow(float64(baseDelay), float64(retryCount))) * time.Second
}

// processError handles the error messages.
// Returns:
// - "continue", nil: tx was not successful, but special error type. Handled, ready for retry
// - "ok", nil: tx was successful
// - "error", error: tx failed, with regular error type
func processError(err error, infoMsg string, retryCount int64, node *NodeConfig) (string, error) {
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
					delay := calculateExponentialBackoffDelay(node.Wallet.RetryDelay, retryCount)
					time.Sleep(delay)
					return ERROR_PROCESSING_CONTINUE, nil
				case int(sdkerrors.ErrWrongSequence.ABCICode()), int(sdkerrors.ErrInvalidSequence.ABCICode()):
					log.Warn().
						Err(err).
						Str("msg", infoMsg).
						Int64("delay", node.Wallet.AccountSequenceRetryDelay).
						Msg("Account sequence mismatch detected, retrying with fixed delay")
					// Wait a fixed block-related waiting time
					time.Sleep(time.Duration(node.Wallet.AccountSequenceRetryDelay) * time.Second)
					return ERROR_PROCESSING_CONTINUE, nil
				case int(sdkerrors.ErrInsufficientFee.ABCICode()):
					log.Warn().Str("msg", infoMsg).Msg("Insufficient fee")
					return ERROR_PROCESSING_FEES, nil
				case int(sdkerrors.ErrTxTooLarge.ABCICode()):
					return ERROR_PROCESSING_ERROR, errorsmod.Wrapf(err, "tx too large")
				case int(sdkerrors.ErrTxInMempoolCache.ABCICode()):
					return ERROR_PROCESSING_ERROR, errorsmod.Wrapf(err, "tx already in mempool cache")
				case int(sdkerrors.ErrInvalidChainID.ABCICode()):
					return ERROR_PROCESSING_ERROR, errorsmod.Wrapf(err, "invalid chain-id")
				default:
					log.Info().Int("errorCode", errorCode).Str("msg", infoMsg).Msg("ABCI error, but not special case - regular retry")
				}
			}
		} else {
			log.Error().Str("msg", infoMsg).Msg("Unmatched error format, cannot classify as ABCI error")
		}
	}

	// NOT ABCI error code: keep on checking for specially handled error types
	if strings.Contains(err.Error(), ERROR_MESSAGE_ACCOUNT_SEQUENCE_MISMATCH) {
		log.Warn().
			Err(err).
			Str("msg", infoMsg).
			Int64("delay", node.Wallet.AccountSequenceRetryDelay).
			Msg("Account sequence mismatch detected, re-fetching sequence")
		time.Sleep(time.Duration(node.Wallet.AccountSequenceRetryDelay) * time.Second)
		return ERROR_PROCESSING_CONTINUE, nil
	} else if strings.Contains(err.Error(), ERROR_MESSAGE_WAITING_FOR_NEXT_BLOCK) {
		log.Warn().Str("msg", infoMsg).Msg("Tx accepted in mempool, it will be included in the following block(s) - not retrying")
		return ERROR_PROCESSING_OK, nil
	} else if strings.Contains(err.Error(), ERROR_MESSAGE_DATA_ALREADY_SUBMITTED) || strings.Contains(err.Error(), ERROR_MESSAGE_CANNOT_UPDATE_EMA) {
		log.Warn().Err(err).Str("msg", infoMsg).Msg("Already submitted data for this epoch.")
		return ERROR_PROCESSING_OK, nil
	}

	return ERROR_PROCESSING_ERROR, errorsmod.Wrapf(err, "failed to process error")
}

// SendDataWithRetry attempts to send data, handling retries, with fee awareness.
// Custom handling for different errors.
func (node *NodeConfig) SendDataWithRetry(ctx context.Context, req sdktypes.Msg, infoMsg string) (*cosmosclient.Response, error) {
	var txResp *cosmosclient.Response
	// Excess fees correction factor translated to fees using configured gas prices
	excessFactorFees := float64(EXCESS_CORRECTION_IN_GAS) * node.Wallet.GasPrices
	// Keep track of how many times fees need to be recalculated to avoid missing fee info between errors
	recalculateFees := 0
	// Use to keep track of expected sequence number between errors
	globalExpectedSeqNum := uint64(0)

	for retryCount := int64(0); retryCount <= node.Wallet.MaxRetries; retryCount++ {
		log.Debug().Msgf("SendDataWithRetry iteration started (%d/%d)", retryCount, node.Wallet.MaxRetries)
		// Create tx without fees
		txOptions := cosmosclient.TxOptions{}
		if globalExpectedSeqNum > 0 {
			log.Info().
				Uint64("expected", globalExpectedSeqNum).
				Uint64("current", node.Chain.Client.TxFactory.Sequence()).
				Msg("Resetting sequence to expected from previous sequence errors")
			node.Chain.Client.TxFactory = node.Chain.Client.TxFactory.WithSequence(globalExpectedSeqNum)
		}
		txService, err := node.Chain.Client.CreateTxWithOptions(ctx, node.Chain.Account, txOptions, req)
		if err != nil {
			// Handle error on creation of tx, before broadcasting
			if strings.Contains(err.Error(), ERROR_MESSAGE_ACCOUNT_SEQUENCE_MISMATCH) {
				log.Warn().Err(err).Str("msg", infoMsg).Msg("Account sequence mismatch detected, resetting sequence")
				expectedSeqNum, currentSeqNum, err := parseSequenceFromAccountMismatchError(err.Error())
				if err != nil {
					log.Error().Err(err).Str("msg", infoMsg).Msg("Failed to parse sequence from error - retrying with regular delay")
					time.Sleep(time.Duration(node.Wallet.RetryDelay) * time.Second)
					continue
				}
				// Reset sequence to expected in the client's tx factory
				node.Chain.Client.TxFactory = node.Chain.Client.TxFactory.WithSequence(expectedSeqNum)
				log.Info().Uint64("expected", expectedSeqNum).Uint64("current", currentSeqNum).Msg("Retrying resetting sequence from current to expected")
				txService, err = node.Chain.Client.CreateTxWithOptions(ctx, node.Chain.Account, txOptions, req)
				if err != nil {
					log.Error().Err(err).Str("msg", infoMsg).Msg("Failed to reset sequence second time, retrying with regular delay")
					time.Sleep(time.Duration(node.Wallet.RetryDelay) * time.Second)
					continue
				}
				// if creation is successful, make the expected sequence number persistent
				globalExpectedSeqNum = expectedSeqNum
			} else {
				errorResponse, err := processError(err, infoMsg, retryCount, node)
				switch errorResponse {
				case ERROR_PROCESSING_OK:
					return txResp, nil
				case ERROR_PROCESSING_ERROR:
					// if error has not been handled, sleep and retry with regular delay
					if err != nil {
						log.Error().Err(err).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, node.Wallet.MaxRetries)
						// Wait for the uniform delay before retrying
						time.Sleep(time.Duration(node.Wallet.RetryDelay) * time.Second)
						continue
					}
				case ERROR_PROCESSING_CONTINUE:
					// Error has not been handled, just continue next iteration
					continue
				case ERROR_PROCESSING_FEES:
					// Error has not been handled, just mark as recalculate fees on this iteration
					log.Debug().Msg("Marking fee recalculation on tx creation")
					recalculateFees += 1
				default:
					return nil, errorsmod.Wrapf(err, "failed to process error")
				}
			}
		} else {
			log.Trace().Msg("Create tx with account sequence OK")
		}

		// Handle fees if necessary
		if node.Wallet.GasPrices > 0 && recalculateFees > 0 {
			// Precalculate fees
			fees := uint64(float64(txService.Gas()+EXCESS_CORRECTION_IN_GAS) * node.Wallet.GasPrices)
			// Add excess fees correction factor to increase with each fee-problematic retry
			fees = fees + uint64(float64(recalculateFees)*excessFactorFees)
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

		// Broadcast tx
		txResponse, err := txService.Broadcast(ctx)
		if err == nil {
			log.Info().Str("msg", infoMsg).Str("txHash", txResponse.TxHash).Msg("Success")
			return txResp, nil
		}
		// Handle error on broadcasting
		errorResponse, err := processError(err, infoMsg, retryCount, node)
		switch errorResponse {
		case ERROR_PROCESSING_OK:
			return txResp, nil
		case ERROR_PROCESSING_ERROR:
			// Error has not been handled, sleep and retry with regular delay
			if err != nil {
				log.Error().Err(err).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, node.Wallet.MaxRetries)
				// Wait for the uniform delay before retrying
				time.Sleep(time.Duration(node.Wallet.RetryDelay) * time.Second)
				continue
			}
		case ERROR_PROCESSING_CONTINUE:
			// Error has not been handled, just continue next iteration
			continue
		case ERROR_PROCESSING_FEES:
			// Error has not been handled, just mark as recalculate fees on this iteration
			log.Debug().Msg("Marking fee recalculation on tx broadcasting")
			recalculateFees += 1
			continue
		default:
			return nil, errorsmod.Wrapf(err, "failed to process error")
		}
	}

	return nil, errors.New("Tx not able to complete after failing max retries")
}

// Extract expected and current sequence numbers from the error message
func parseSequenceFromAccountMismatchError(errorMessage string) (uint64, uint64, error) {
	re := regexp.MustCompile(`account sequence mismatch, expected (\d+), got (\d+)`)
	matches := re.FindStringSubmatch(errorMessage)

	if len(matches) == 3 {
		expected, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil {
			return 0, 0, err
		}

		current, err := strconv.ParseUint(matches[2], 10, 64)
		if err != nil {
			return 0, 0, err
		}

		return expected, current, nil
	}
	return 0, 0, fmt.Errorf("sequence numbers not found in error message")
}
