package lib

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	errorsmod "cosmossdk.io/errors"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
)

// SendDataWithRetry attempts to send data, handling retries, with fee awareness.
// Custom handling for different errors.
func (node *NodeConfig) SendDataWithRetry(ctx context.Context, req sdktypes.Msg, infoMsg string, timeoutHeight uint64) (*cosmosclient.Response, error) {
	var txResp *cosmosclient.Response
	// Excess fees correction factor translated to fees using configured gas prices
	// This value is updated by the fee price update routine - making copy for consistency within method
	gasPrices := GetGasPrice()

	excessFactorFees := float64(EXCESS_CORRECTION_IN_GAS) * gasPrices
	// Keep track of how many times fees need to be recalculated to avoid missing fee info between errors
	recalculateFees := 1
	// Use to keep track of expected sequence number between errors
	globalExpectedSeqNum := uint64(0)

	// Create tx options with timeout height if specified
	if timeoutHeight > 0 {
		log.Debug().Uint64("timeoutHeight", timeoutHeight).Msg("Setting timeout height for tx")
		node.Chain.Client.TxFactory = node.Chain.Client.TxFactory.WithTimeoutHeight(timeoutHeight)
	}

	for retryCount := int64(0); retryCount <= node.Wallet.MaxRetries; retryCount++ {
		log.Debug().Msgf("SendDataWithRetry iteration started (%d/%d)", retryCount, node.Wallet.MaxRetries)
		// Create tx without fees to simulate tx creation and get estimated gas and seq number
		txOptions := cosmosclient.TxOptions{} // nolint: exhaustruct
		if globalExpectedSeqNum > 0 && node.Chain.Client.TxFactory.Sequence() != globalExpectedSeqNum {
			log.Debug().
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
					if DoneOrWait(ctx, node.Wallet.RetryDelay) {
						return nil, ctx.Err()
					}
					continue
				}
				// Reset sequence to expected in the client's tx factory
				node.Chain.Client.TxFactory = node.Chain.Client.TxFactory.WithSequence(expectedSeqNum)
				log.Info().Uint64("expected", expectedSeqNum).Uint64("current", currentSeqNum).Msg("Retrying resetting sequence from current to expected")
				txService, err = node.Chain.Client.CreateTxWithOptions(ctx, node.Chain.Account, txOptions, req)
				if err != nil {
					log.Error().Err(err).Str("msg", infoMsg).Msg("Failed to reset sequence second time, retrying with regular delay")
					if DoneOrWait(ctx, node.Wallet.RetryDelay) {
						return nil, ctx.Err()
					}
					continue
				}
				// if creation is successful, make the expected sequence number persistent
				globalExpectedSeqNum = expectedSeqNum
			} else {
				errorResponse, err := ProcessErrorTx(ctx, err, infoMsg, retryCount, node)
				switch errorResponse {
				case ERROR_PROCESSING_OK:
					return txResp, nil
				case ERROR_PROCESSING_ERROR:
					// if error has not been handled, sleep and retry with regular delay
					if err != nil {
						log.Error().Err(err).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, node.Wallet.MaxRetries)
						// Wait for the uniform delay before retrying
						if DoneOrWait(ctx, node.Wallet.RetryDelay) {
							return nil, ctx.Err()
						}
						continue
					}
				case ERROR_PROCESSING_CONTINUE:
					// Error has not been handled, just continue next iteration
					continue
				case ERROR_PROCESSING_FEES:
					// Error has not been handled, just mark as recalculate fees on this iteration
					log.Debug().Msg("Marking fee recalculation on tx creation")
				case ERROR_PROCESSING_FAILURE:
					return nil, errorsmod.Wrapf(err, "tx failed and not retried")
				case ERROR_PROCESSING_SWITCHING_NODE:
					return nil, err
				default:
					return nil, errorsmod.Wrapf(err, "failed to process error")
				}
			}
		} else {
			log.Trace().Msg("Create tx with account sequence OK")
		}

		// Handle fees if necessary
		if gasPrices > 0 {
			// Precalculate fees
			estimatedGas := float64(txService.Gas()) * node.Wallet.GasAdjustment
			fees := uint64(float64(estimatedGas+EXCESS_CORRECTION_IN_GAS) * gasPrices)
			// Add excess fees correction factor to increase with each fee-problematic retry
			fees = fees + uint64(float64(recalculateFees)*excessFactorFees)
			// Limit fees to maxFees
			if fees > node.Wallet.MaxFees {
				log.Warn().Uint64("gas", txService.Gas()).Uint64("limit", node.Wallet.MaxFees).Msg("Gas limit exceeded, using maxFees instead")
				fees = node.Wallet.MaxFees
			}
			txOptions := cosmosclient.TxOptions{ // nolint: exhaustruct
				Fees: fmt.Sprintf("%duallo", fees),
			}
			log.Info().Str("fees", txOptions.Fees).Msg("Attempting tx with calculated fees")
			txService, err = node.Chain.Client.CreateTxWithOptions(ctx, node.Chain.Account, txOptions, req)
			if err != nil {
				return nil, err
			}
		}

		log.Trace().Msg("Creation of tx successful, broadcasting tx")
		// Broadcast tx
		txResponse, err := txService.Broadcast(ctx)
		if err == nil {
			log.Info().Str("msg", infoMsg).Str("txHash", txResponse.TxHash).Msg("Success")
			return txResp, nil
		}
		// Handle error on broadcasting
		errorResponse, err := ProcessErrorTx(ctx, err, infoMsg, retryCount, node)
		switch errorResponse {
		case ERROR_PROCESSING_OK:
			return txResp, nil
		case ERROR_PROCESSING_ERROR:
			// Error has not been handled, sleep and retry with regular delay
			if err != nil {
				log.Error().Err(err).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, node.Wallet.MaxRetries)
				// Wait for the uniform delay before retrying
				if DoneOrWait(ctx, node.Wallet.RetryDelay) {
					return nil, ctx.Err()
				}
				continue
			}
		case ERROR_PROCESSING_CONTINUE:
			// Error has not been handled, just continue next iteration
			continue
		case ERROR_PROCESSING_FEES:
			// Error has not been handled, just mark as recalculate fees on this iteration
			log.Info().Msg("Insufficient fees, marking fee recalculation on tx broadcasting for retrial")
			recalculateFees += 1
			continue
		case ERROR_PROCESSING_FAILURE:
			return nil, errorsmod.Wrapf(err, "tx failed and not retried")
		case ERROR_PROCESSING_SWITCHING_NODE:
			return nil, err
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
