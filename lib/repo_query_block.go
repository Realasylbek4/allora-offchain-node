package lib

import (
	"context"
	"time"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

func (node *NodeConfig) GetReputerValuesAtBlock(topicId emissionstypes.TopicId, nonce BlockHeight) (*emissionstypes.ValueBundle, error) {
	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.GetNetworkInferencesAtBlockResponse, error) {
			return node.Chain.EmissionsQueryClient.GetNetworkInferencesAtBlock(ctx, &emissionstypes.GetNetworkInferencesAtBlockRequest{
				TopicId:                  topicId,
				BlockHeightLastInference: nonce,
			})
		},
		query.PageRequest{},
		"get reputer values at block",
	)
	if err != nil {
		return &emissionstypes.ValueBundle{}, err
	}

	return resp.NetworkInferences, nil
}
