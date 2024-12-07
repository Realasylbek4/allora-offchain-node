package lib

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/cosmos/cosmos-sdk/types/query"
)

// QueryDataWithRetry attempts to query data with a uniform backoff strategy for retries.
func QueryDataWithRetry[T any](
	ctx context.Context,
	maxRetries int64,
	delaySeconds int64,
	queryFunc func(context.Context, query.PageRequest) (T, error),
	req query.PageRequest,
	infoMsg string,
) (T, error) {
	var result T
	var err error

	for retryCount := int64(0); retryCount <= maxRetries; retryCount++ {
		log.Trace().Msgf("QueryDataWithRetry iteration started (%d/%d): %s", retryCount, maxRetries, infoMsg)
		result, err = queryFunc(ctx, req)
		if err == nil {
			return result, nil
		}

		// Log the error for each retry.
		log.Error().Err(err).Msgf("Query failed, retrying... (Retry %d/%d): %s", retryCount, maxRetries, infoMsg)

		if IsErrorSwitchingNode(err) {
			return result, err
		}

		// Wait for the uniform delay before retrying
		if DoneOrWait(ctx, delaySeconds) {
			break
		}
	}

	// All retries failed, return the last error
	return result, err
}
