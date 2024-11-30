package lib

import (
	"context"
	"time"

	cosmossdk_io_math "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func (node *NodeConfig) GetBalance() (cosmossdk_io_math.Int, error) {
	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*banktypes.QueryBalanceResponse, error) {
			return node.Chain.BankQueryClient.Balance(ctx, &banktypes.QueryBalanceRequest{
				Address: node.Chain.Address,
				Denom:   node.Chain.DefaultBondDenom,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"get balance",
	)
	if err != nil {
		return cosmossdk_io_math.Int{}, err
	}

	return resp.Balance.Amount, nil
}
