package lib

import (
	"context"
	"time"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// Gets the latest open worker nonce for a given topic, with retries
func (node *NodeConfig) GetLatestOpenWorkerNonceByTopicId(topicId emissionstypes.TopicId) (*emissionstypes.Nonce, error) {
	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.GetUnfulfilledWorkerNoncesResponse, error) {
			return node.Chain.EmissionsQueryClient.GetUnfulfilledWorkerNonces(ctx, &emissionstypes.GetUnfulfilledWorkerNoncesRequest{
				TopicId: topicId,
			})
		},
		query.PageRequest{},
		"get open worker nonce",
	)
	if err != nil {
		return &emissionstypes.Nonce{}, err
	}

	if len(resp.Nonces.Nonces) == 0 {
		return &emissionstypes.Nonce{}, nil
	}
	// Per `AddWorkerNonce()` in `allora-chain/x/emissions/keeper.go`, the latest nonce is first
	return resp.Nonces.Nonces[0], nil
}

// Gets the oldest open reputer nonce for a given topic, with retries
func (node *NodeConfig) GetOldestReputerNonceByTopicId(topicId emissionstypes.TopicId) (*emissionstypes.Nonce, error) {
	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.GetUnfulfilledReputerNoncesResponse, error) {
			return node.Chain.EmissionsQueryClient.GetUnfulfilledReputerNonces(ctx, &emissionstypes.GetUnfulfilledReputerNoncesRequest{
				TopicId: topicId,
			})
		},
		query.PageRequest{},
		"get open reputer nonce",
	)
	if err != nil {
		return &emissionstypes.Nonce{}, err
	}

	if len(resp.Nonces.Nonces) == 0 {
		return &emissionstypes.Nonce{}, nil
	}
	// Per `AddWorkerNonce()` in `allora-chain/x/emissions/keeper.go`, the oldest nonce is last
	return resp.Nonces.Nonces[len(resp.Nonces.Nonces)-1].ReputerNonce, nil
}
