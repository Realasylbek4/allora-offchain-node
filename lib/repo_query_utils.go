package lib

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cosmos/cosmos-sdk/types/query"
)

// QueryDataWithRetry attempts to query data with a uniform backoff strategy for retries.
func QueryDataWithRetry[T any](
	ctx context.Context,
	maxRetries int64,
	delay time.Duration,
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

		// Wait for the uniform delay before retrying
		time.Sleep(delay)
	}

	// All retries failed, return the last error
	return result, fmt.Errorf("query failed after %d retries: %w", maxRetries, err)
}
