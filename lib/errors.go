package lib

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	errorsmod "cosmossdk.io/errors"
	emissions "github.com/allora-network/allora-chain/x/emissions/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/rs/zerolog/log"
	feemarkettypes "github.com/skip-mev/feemarket/x/feemarket/types"
)

// Error codes for the module
const ERROR_CODESPACE = "allora-offchain-lib"

var (
	ErrTooManyRequests  = errorsmod.Register(ERROR_CODESPACE, 1, "too many requests")
	ErrNotEnoughBalance = errorsmod.Register(ERROR_CODESPACE, 2, "not enough balance")
	ErrNotRegistered    = errorsmod.Register(ERROR_CODESPACE, 3, "not registered")
	ErrStakeBelowMin    = errorsmod.Register(ERROR_CODESPACE, 4, "stake below minimum")
)

const ERROR_MESSAGE_ABCI_ERROR_CODE_MARKER = "error code:"
const ERROR_MESSAGE_DATA_ALREADY_SUBMITTED = "already submitted"
const ERROR_MESSAGE_CANNOT_UPDATE_EMA = "cannot update EMA"
const ERROR_MESSAGE_WAITING_FOR_NEXT_BLOCK = "waiting for next block" // This means tx is accepted in mempool but not yet included in a block
const ERROR_MESSAGE_ACCOUNT_SEQUENCE_MISMATCH = "account sequence mismatch"
const ERROR_MESSAGE_TIMEOUT_HEIGHT = "timeout height"
const ERROR_MESSAGE_NOT_PERMITTED_TO_SUBMIT_PAYLOAD = "not permitted to submit payload"
const ERROR_MESSAGE_NOT_PERMITTED_TO_ADD_STAKE = "not permitted to add stake"

const EXCESS_CORRECTION_IN_GAS = 20000

// Error processing types
// - "continue", nil: tx was not successful, but special error type. Handled, ready for retry
// - "ok", nil: tx was successful, error handled and not re-raised
// - "error", error: tx failed, with regular error type
// - "fees": tx failed, because of insufficient fees
// - "failure": tx failed, and should not be retried anymore
const ERROR_PROCESSING_CONTINUE = "continue"
const ERROR_PROCESSING_OK = "ok"
const ERROR_PROCESSING_FEES = "fees"
const ERROR_PROCESSING_ERROR = "error"
const ERROR_PROCESSING_FAILURE = "failure"
const ERROR_PROCESSING_SWITCHING_NODE = "switch"

// HTTP status codes that trigger node switching
var HTTP_STATUS_CODES_SWITCHING_NODE = map[int]bool{
	429: true, // Too Many Requests
}

// calculateExponentialBackoffDelay returns a duration based on retry count and base delay
func calculateExponentialBackoffDelaySeconds(baseDelay int64, retryCount int64) int64 {
	return int64(math.Pow(float64(baseDelay), float64(retryCount)))
}

// processError handles the error messages.
func ProcessErrorTx(ctx context.Context, err error, infoMsg string, retryCount int64, node *NodeConfig) (string, error) {
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
					delay := calculateExponentialBackoffDelaySeconds(node.Wallet.RetryDelay, retryCount)
					if DoneOrWait(ctx, delay) {
						return ERROR_PROCESSING_ERROR, ctx.Err()
					}
					return ERROR_PROCESSING_CONTINUE, nil
				case int(sdkerrors.ErrWrongSequence.ABCICode()), int(sdkerrors.ErrInvalidSequence.ABCICode()):
					log.Warn().
						Err(err).
						Str("msg", infoMsg).
						Int64("delay", node.Wallet.AccountSequenceRetryDelay).
						Msg("Account sequence mismatch detected, retrying with fixed delay")
					// Wait a fixed block-related waiting time
					if DoneOrWait(ctx, node.Wallet.AccountSequenceRetryDelay) {
						return ERROR_PROCESSING_ERROR, ctx.Err()
					}
					return ERROR_PROCESSING_CONTINUE, nil
				case int(sdkerrors.ErrInsufficientFee.ABCICode()):
					log.Info().
						Err(err).
						Str("msg", infoMsg).
						Msg("Insufficient fees")
					return ERROR_PROCESSING_FEES, nil
				case int(feemarkettypes.ErrNoFeeCoins.ABCICode()):
					log.Info().
						Err(err).
						Str("msg", infoMsg).
						Msg("No fee coins")
					return ERROR_PROCESSING_FEES, nil
				case int(sdkerrors.ErrTxTooLarge.ABCICode()):
					return ERROR_PROCESSING_ERROR, errorsmod.Wrapf(err, "tx too large")
				case int(sdkerrors.ErrTxInMempoolCache.ABCICode()):
					return ERROR_PROCESSING_ERROR, errorsmod.Wrapf(err, "tx already in mempool cache")
				case int(sdkerrors.ErrInvalidChainID.ABCICode()):
					return ERROR_PROCESSING_ERROR, errorsmod.Wrapf(err, "invalid chain-id")
				case int(sdkerrors.ErrTxTimeoutHeight.ABCICode()):
					return ERROR_PROCESSING_FAILURE, errorsmod.Wrapf(err, "tx timeout height")
				case int(emissions.ErrWorkerNonceWindowNotAvailable.ABCICode()):
					log.Warn().
						Err(err).
						Str("msg", infoMsg).
						Msg("Worker window not available, retrying with exponential backoff")
					delay := calculateExponentialBackoffDelaySeconds(node.Wallet.RetryDelay, retryCount)
					if DoneOrWait(ctx, delay) {
						return ERROR_PROCESSING_ERROR, ctx.Err()
					}
					return ERROR_PROCESSING_CONTINUE, nil
				case int(emissions.ErrReputerNonceWindowNotAvailable.ABCICode()):
					log.Warn().
						Err(err).
						Str("msg", infoMsg).
						Msg("Reputer window not available, retrying with exponential backoff")
					delay := calculateExponentialBackoffDelaySeconds(node.Wallet.RetryDelay, retryCount)
					if DoneOrWait(ctx, delay) {
						return ERROR_PROCESSING_ERROR, ctx.Err()
					}
					return ERROR_PROCESSING_CONTINUE, nil
				default:
					log.Info().Int("errorCode", errorCode).Str("msg", infoMsg).Msg("ABCI error, but not special case - regular retry")
				}
			}
		} else {
			log.Warn().Str("msg", infoMsg).Msg("Unmatched error format, cannot classify as ABCI error")
		}
	}

	// Check if error is HTTP status code
	if statusCode, statusMessage, error := ParseHTTPStatus(err.Error()); error == nil {
		log.Warn().Int("statusCode", statusCode).Str("statusMessage", statusMessage).Str("msg", infoMsg).Msg("HTTP status code detected")
		if statusCode, statusMessage, err := ParseHTTPStatus(err.Error()); err == nil {
			log.Warn().Int("statusCode", statusCode).Str("statusMessage", statusMessage).Str("msg", infoMsg).Msg("HTTP status code detected")

			if HTTP_STATUS_CODES_SWITCHING_NODE[statusCode] {
				log.Warn().
					Int("statusCode", statusCode).
					Str("msg", infoMsg).
					Msg("HTTP status error code detected, switching to next node")
				return ERROR_PROCESSING_SWITCHING_NODE, ErrTooManyRequests
			}
		}
	}

	// NOT ABCI error code: keep on checking for specially handled error types
	if strings.Contains(err.Error(), ERROR_MESSAGE_ACCOUNT_SEQUENCE_MISMATCH) {
		log.Warn().
			Err(err).
			Str("msg", infoMsg).
			Int64("delay", node.Wallet.AccountSequenceRetryDelay).
			Msg("Account sequence mismatch detected, re-fetching sequence")
		if DoneOrWait(ctx, node.Wallet.AccountSequenceRetryDelay) {
			return ERROR_PROCESSING_ERROR, ctx.Err()
		}
		return ERROR_PROCESSING_CONTINUE, nil
	} else if strings.Contains(err.Error(), ERROR_MESSAGE_WAITING_FOR_NEXT_BLOCK) {
		log.Warn().Err(err).Str("msg", infoMsg).Msg("Tx accepted in mempool, it will be included in the following block(s) - not retrying")
		return ERROR_PROCESSING_OK, nil
	} else if strings.Contains(err.Error(), ERROR_MESSAGE_DATA_ALREADY_SUBMITTED) || strings.Contains(err.Error(), ERROR_MESSAGE_CANNOT_UPDATE_EMA) {
		log.Warn().Err(err).Str("msg", infoMsg).Msg("Already submitted data for this epoch.")
		return ERROR_PROCESSING_OK, nil
	} else if strings.Contains(err.Error(), ERROR_MESSAGE_TIMEOUT_HEIGHT) {
		log.Warn().Err(err).Str("msg", infoMsg).Msg("Tx failed because of timeout height")
		return ERROR_PROCESSING_FAILURE, err
	} else if strings.Contains(err.Error(), ERROR_MESSAGE_NOT_PERMITTED_TO_SUBMIT_PAYLOAD) {
		log.Warn().Err(err).Str("msg", infoMsg).Msg("Actor is not permitted to submit payload")
		return ERROR_PROCESSING_FAILURE, err
	} else if strings.Contains(err.Error(), ERROR_MESSAGE_NOT_PERMITTED_TO_ADD_STAKE) {
		log.Warn().Err(err).Str("msg", infoMsg).Msg("Actor is not permitted to add stake")
		return ERROR_PROCESSING_FAILURE, err
	}

	return ERROR_PROCESSING_ERROR, errorsmod.Wrapf(err, "failed to process error")
}

// func ProcessErrorQuery(ctx context.Context, err error, infoMsg string, retryCount int64, node *NodeConfig) (string, error) {

// }

// ParseStatus parses a status code and message from a given text string.
func ParseHTTPStatus(input string) (int, string, error) {
	// Regular expression to match "Status: <code> <message>" or similar patterns in text
	re := regexp.MustCompile(`(?i)status:\s*(\d+)\s*([^,]*)`)

	matches := re.FindStringSubmatch(input)
	if len(matches) < 3 {
		return 0, "", errors.New("invalid input format")
	}

	// Parse the status code
	statusCode, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, "", fmt.Errorf("invalid status code: %v", err)
	}

	// Get the status message
	statusMessage := strings.TrimSpace(matches[2])

	return statusCode, statusMessage, nil
}

// Returns true if the error is a switching-node error
func IsErrorSwitchingNode(err error) bool {
	return errors.Is(err, ErrTooManyRequests)
}
