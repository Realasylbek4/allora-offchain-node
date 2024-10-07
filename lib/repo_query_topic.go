package lib

import (
	"context"
	"errors"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
)

func (node *NodeConfig) GetTopicInfo(topicId emissionstypes.TopicId) (*emissionstypes.Topic, error) {
	ctx := context.Background()

	res, err := node.Chain.EmissionsQueryClient.GetTopic(
		ctx,
		&emissionstypes.GetTopicRequest{TopicId: topicId},
	)
	if err != nil {
		return nil, err
	}

	if res.Topic == nil {
		return nil, errors.New("Topic not found")
	}
	return res.Topic, nil
}
