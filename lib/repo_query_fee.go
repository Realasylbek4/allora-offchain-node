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

// Keeps track of the current gas price
var gasPrice float64 = 0

// UpdateGasPriceRoutine continuously updates the gas price at a specified interval
func (node *NodeConfig) UpdateGasPriceRoutine(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Updating fee price routine: terminating.")
			return
		default:
			var err error
			price, err := node.GetBaseFee(ctx)
			if err != nil {
				log.Error().Err(err).Msg("Error updating gas prices")
			}
			gasPrice = price
			log.Debug().Float64("gasPrice", gasPrice).Msg("Updating fee price routine: updating value.")
			time.Sleep(time.Duration(node.Wallet.GasPriceUpdateInterval) * time.Second)
		}
	}
}

// GetGasPrice returns the current gas price
func GetGasPrice() float64 {
	return gasPrice
}

// SetGasPrice sets the current gas price
func SetGasPrice(price float64) {
	gasPrice = price
}

// GetBaseFee queries the current base fee from the feemarket module
func (node *NodeConfig) GetBaseFee(ctx context.Context) (float64, error) {
	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		node.Wallet.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*feemarkettypes.GasPriceResponse, error) {
			return node.Chain.FeeMarketQueryClient.GasPrice(ctx, &feemarkettypes.GasPriceRequest{Denom: node.Chain.DefaultBondDenom})
		},
		query.PageRequest{}, // nolint:exhaustruct
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
