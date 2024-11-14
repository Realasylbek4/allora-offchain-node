package lib

import (
	"context"
	"errors"
	"time"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// Gets topic info for a given topic ID, with retries
func (node *NodeConfig) GetTopicInfo(topicId emissionstypes.TopicId) (*emissionstypes.Topic, error) {
	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.GetTopicResponse, error) {
			return node.Chain.EmissionsQueryClient.GetTopic(ctx, &emissionstypes.GetTopicRequest{
				TopicId: topicId,
			})
		},
		query.PageRequest{},
		"get topic info",
	)
	if err != nil {
		return nil, err
	}

	if resp.Topic == nil {
		return nil, errors.New("Topic not found")
	}
	return resp.Topic, nil
}
