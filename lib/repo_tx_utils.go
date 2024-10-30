package lib

import (
	"context"
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

const ERROR_MESSAGE_EMA_ALREADY_SENT = "cannot update EMA"
const ERROR_MESSAGE_TX_INCLUDED_IN_BLOCK = "waiting for next block"
const ERROR_MESSAGE_ACCOUNT_SEQUENCE_MISMATCH = "account sequence mismatch"
const ERROR_MESSAGE_ABCI_ERROR_CODE_MARKER = "error code:"

// SendDataWithRetry attempts to send data with a uniform backoff strategy for retries.
// uniform backoff is preferred to avoid exiting the open submission windows
func (node *NodeConfig) SendDataWithRetry(ctx context.Context, req sdktypes.Msg, infoMsg string) (*cosmosclient.Response, error) {
	var txResp *cosmosclient.Response
	var err error
	var hadEOFTxError bool
	for retryCount := int64(0); retryCount <= node.Wallet.MaxRetries; retryCount++ {
		txResponse, err := node.Chain.Client.BroadcastTx(ctx, node.Chain.Account, req)
		txResp = &txResponse
		if err == nil {
			log.Info().Str("msg", infoMsg).Str("txHash", txResp.TxHash).Msg("Success")
			return txResp, nil
		}

		if strings.Contains(err.Error(), ERROR_MESSAGE_ABCI_ERROR_CODE_MARKER) {
			// Parse ABCI numerical error code
			re := regexp.MustCompile(`error code: '(\d+)'`)
			matches := re.FindStringSubmatch(err.Error())
			if len(matches) == 2 {
				errorCode, parseErr := strconv.Atoi(matches[1])
				if parseErr != nil {
					log.Error().Err(parseErr).Str("msg", infoMsg).Msg("Failed to parse ABCI error code")
				} else {
					switch errorCode {
					case int(sdkerrors.ErrMempoolIsFull.ABCICode()):
						// Mempool is full, retry with exponential backoff
						log.Warn().Str("msg", infoMsg).Msg("Mempool is full, retrying with exponential backoff")
						delay := time.Duration(math.Pow(float64(node.Wallet.RetryDelay), float64(retryCount))) * time.Second
						time.Sleep(delay)
						continue
					case int(sdkerrors.ErrWrongSequence.ABCICode()):
					case int(sdkerrors.ErrInvalidSequence.ABCICode()):
						log.Warn().Str("msg", infoMsg).Msg("Account sequence mismatch detected, allow re-fetching sequence")
						// Wait a fixed block-related waiting time
						time.Sleep(time.Duration(node.Wallet.AccountSequenceRetryDelay) * time.Second)
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
			// Wait a fixed block-related waiting time
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
		// Log the error for each retry.
		log.Error().Err(err).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, node.Wallet.MaxRetries)
		// Wait for the uniform delay before retrying
		time.Sleep(time.Duration(node.Wallet.RetryDelay) * time.Second)
	}
	// All retries failed, return the last error
	return nil, err
}
