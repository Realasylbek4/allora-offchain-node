package lib

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/rs/zerolog/log"
	feemarkettypes "github.com/skip-mev/feemarket/x/feemarket/types"
)

// GetBaseFee queries the current base fee from the feemarket module
func (node *NodeConfig) GetBaseFee(ctx context.Context) (float64, error) {
	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*feemarkettypes.GasPriceResponse, error) {
			return node.Chain.FeeMarketQueryClient.GasPrice(ctx, &feemarkettypes.GasPriceRequest{Denom: node.Chain.DefaultBondDenom})
		},
		query.PageRequest{},
		"get base fee",
	)
	if err != nil {
		return 0, err
	}

	// Convert legacyDec to string first, then to float64
	baseFeeStr := resp.Price.Amount.String()
	baseFee, err := strconv.ParseFloat(baseFeeStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse base fee: %w", err)
	}

	log.Debug().Float64("baseFee", baseFee).Msg("Retrieved base fee from chain")
	return baseFee, nil
}
