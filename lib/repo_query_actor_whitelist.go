package lib

import (
	"context"
	"time"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// Checks if an worker address is whitelisted for a given topic
func (node *NodeConfig) IsWorkerWhitelisted(topicId emissionstypes.TopicId, address string) (bool, error) {
	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.IsWhitelistedTopicWorkerResponse, error) {
			return node.Chain.EmissionsQueryClient.IsWhitelistedTopicWorker(ctx, &emissionstypes.IsWhitelistedTopicWorkerRequest{
				TopicId: topicId,
				Address: address,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"check worker whitelist",
	)
	if err != nil {
		return false, err
	}

	return resp.IsWhitelistedTopicWorker, nil
}

// Checks if a reputer address is whitelisted for a given topic
func (node *NodeConfig) IsReputerWhitelisted(topicId emissionstypes.TopicId, address string) (bool, error) {
	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.IsWhitelistedTopicReputerResponse, error) {
			return node.Chain.EmissionsQueryClient.IsWhitelistedTopicReputer(ctx, &emissionstypes.IsWhitelistedTopicReputerRequest{
				TopicId: topicId,
				Address: address,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"check reputer whitelist",
	)
	if err != nil {
		return false, err
	}

	return resp.IsWhitelistedTopicReputer, nil
}

// Checks if an address is whitelisted as a global actor
func (node *NodeConfig) IsWhitelistedGlobalActor(address string) (bool, error) {
	ctx := context.Background()

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		time.Duration(node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.IsWhitelistedGlobalActorResponse, error) {
			return node.Chain.EmissionsQueryClient.IsWhitelistedGlobalActor(ctx, &emissionstypes.IsWhitelistedGlobalActorRequest{
				Address: address,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"check global actor whitelist",
	)
	if err != nil {
		return false, err
	}

	return resp.IsWhitelistedGlobalActor, nil
}

// Checks if a worker can submit to a given topic
func (node *NodeConfig) CanSubmitWorker(topicId emissionstypes.TopicId, address string) (bool, error) {
	// Check local whitelist
	isWhitelisted, err := node.IsWorkerWhitelisted(topicId, address)
	if err != nil {
		return false, err
	}
	if isWhitelisted {
		return true, nil
	}

	// Check global whitelist
	isGlobalActorWhitelisted, err := node.IsWhitelistedGlobalActor(address)
	if err != nil {
		return false, err
	}

	return isGlobalActorWhitelisted, nil
}

// Checks if a reputer can submit to a given topic
func (node *NodeConfig) CanSubmitReputer(topicId emissionstypes.TopicId, address string) (bool, error) {
	// Check local whitelist
	isWhitelisted, err := node.IsReputerWhitelisted(topicId, address)
	if err != nil {
		return false, err
	}
	if isWhitelisted {
		return true, nil
	}

	// Check global whitelist
	isGlobalActorWhitelisted, err := node.IsWhitelistedGlobalActor(address)
	if err != nil {
		return false, err
	}

	return isGlobalActorWhitelisted, nil
}
