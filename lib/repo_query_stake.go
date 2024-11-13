package lib

import (
	"context"
	"time"

	cosmossdk_io_math "cosmossdk.io/math"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// Gets the stake from a reputer in a given topic, with retries
func (node *NodeConfig) GetReputerStakeInTopic(
	topicId emissionstypes.TopicId,
	reputer Address,
) (cosmossdk_io_math.Int, error) {
	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.GetStakeFromReputerInTopicInSelfResponse, error) {
			return node.Chain.EmissionsQueryClient.GetStakeFromReputerInTopicInSelf(ctx, &emissionstypes.GetStakeFromReputerInTopicInSelfRequest{
				ReputerAddress: reputer,
				TopicId:        topicId,
			})
		},
		query.PageRequest{},
		"get reputer stake in topic",
	)
	if err != nil {
		return cosmossdk_io_math.Int{}, err
	}
	return resp.Amount, nil
}
