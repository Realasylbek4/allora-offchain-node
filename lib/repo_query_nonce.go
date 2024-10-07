package lib

import (
	"context"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
)

func (node *NodeConfig) GetLatestOpenWorkerNonceByTopicId(topicId emissionstypes.TopicId) (*emissionstypes.Nonce, error) {
	ctx := context.Background()

	res, err := node.Chain.EmissionsQueryClient.GetUnfulfilledWorkerNonces(
		ctx,
		&emissionstypes.GetUnfulfilledWorkerNoncesRequest{TopicId: topicId},
	)
	if err != nil {
		return &emissionstypes.Nonce{}, err
	}

	if len(res.Nonces.Nonces) == 0 {
		return &emissionstypes.Nonce{}, nil
	}
	// Per `AddWorkerNonce()` in `allora-chain/x/emissions/keeper.go`, the latest nonce is first
	return res.Nonces.Nonces[0], nil
}

func (node *NodeConfig) GetOldestReputerNonceByTopicId(topicId emissionstypes.TopicId) (BlockHeight, error) {
	ctx := context.Background()

	res, err := node.Chain.EmissionsQueryClient.GetUnfulfilledReputerNonces(
		ctx,
		&emissionstypes.GetUnfulfilledReputerNoncesRequest{TopicId: topicId},
	)
	if err != nil {
		return 0, err
	}

	if len(res.Nonces.Nonces) == 0 {
		return 0, nil
	}
	// Per `AddWorkerNonce()` in `allora-chain/x/emissions/keeper.go`, the oldest nonce is last
	return res.Nonces.Nonces[len(res.Nonces.Nonces)-1].ReputerNonce.BlockHeight, nil
}
