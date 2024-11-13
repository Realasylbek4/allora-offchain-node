package lib

import (
	"context"
	"errors"
	"time"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// Checks if the worker is registered in a topic, with retries
func (node *NodeConfig) IsWorkerRegistered(topicId uint64) (bool, error) {
	if node.Worker == nil {
		return false, errors.New("no worker to register")
	}

	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.IsWorkerRegisteredInTopicIdResponse, error) {
			return node.Chain.EmissionsQueryClient.IsWorkerRegisteredInTopicId(ctx, &emissionstypes.IsWorkerRegisteredInTopicIdRequest{
				TopicId: topicId,
				Address: node.Wallet.Address,
			})
		},
		query.PageRequest{},
		"is worker registered in topic",
	)
	if err != nil {
		return false, err
	}

	return resp.IsRegistered, nil
}

// Checks if the reputer is registered in a topic, with retries
func (node *NodeConfig) IsReputerRegistered(topicId uint64) (bool, error) {
	if node.Reputer == nil {
		return false, errors.New("no reputer to register")
	}

	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.IsReputerRegisteredInTopicIdResponse, error) {
			return node.Chain.EmissionsQueryClient.IsReputerRegisteredInTopicId(ctx, &emissionstypes.IsReputerRegisteredInTopicIdRequest{
				TopicId: topicId,
				Address: node.Wallet.Address,
			})
		},
		query.PageRequest{},
		"is reputer registered in topic",
	)
	if err != nil {
		return false, err
	}

	return resp.IsRegistered, nil
}
