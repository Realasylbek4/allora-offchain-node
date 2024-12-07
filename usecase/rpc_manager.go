package usecase

import (
	"allora_offchain_node/lib"
	"context"
	"fmt"
	"net/url"
	"sync"

	errorsmod "cosmossdk.io/errors"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog/log"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type RPCManagerInterface interface {
	GetCurrentNode() *lib.NodeConfig
	SwitchToNextNode() *lib.NodeConfig
	GetStats() (int, map[int]int)
	SendDataWithNodeRetry(ctx context.Context, msg sdk.Msg, timeoutHeight uint64, operationName string) (*cosmosclient.Response, error)
	GetNodes() ([]lib.NodeConfig, error)
}

type RPCManager struct {
	nodes      []lib.NodeConfig
	currentIdx int
	mu         sync.RWMutex
	stats      RPCStats
}

type RPCStats struct {
	nodeFailures map[int]int
	nodeSwitches int
	mu           sync.RWMutex
}

func (r *RPCManager) GetNodes() ([]lib.NodeConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nodes, nil
}

// newRPCManager is now private, used internally by InitRPCManager
func NewRPCManager(userConfig lib.UserConfig) (*RPCManager, error) {
	if len(userConfig.Wallet.NodeRPCs) == 0 {
		return nil, fmt.Errorf("no RPC nodes provided")
	}

	validatedNodes, err := validateAndDeduplicateNodes(userConfig.Wallet.NodeRPCs)
	if err != nil {
		return nil, err
	}

	// Load here the nodeconfigs
	var nodes []lib.NodeConfig
	for _, rpc := range validatedNodes {
		log.Info().Str("rpc", rpc).Msg("Initializing rpc")
		nodeConfig, err := userConfig.GenerateNodeConfig(rpc)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *nodeConfig)
	}

	return &RPCManager{
		nodes:      nodes,
		currentIdx: 0,
		stats: RPCStats{
			nodeFailures: make(map[int]int),
		},
	}, nil
}

func validateAndDeduplicateNodes(nodes []string) ([]string, error) {
	seen := make(map[string]bool)
	validated := make([]string, 0)

	for _, node := range nodes {
		// Validate URL
		_, err := url.ParseRequestURI(node)
		if err != nil {
			return nil, fmt.Errorf("invalid RPC URL %s: %w", node, err)
		}

		// Deduplicate
		if !seen[node] {
			seen[node] = true
			validated = append(validated, node)
		}
	}

	return validated, nil
}

func (rm *RPCManager) GetCurrentNode() *lib.NodeConfig {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return &rm.nodes[rm.currentIdx]
}

func (rm *RPCManager) SwitchToNextNode() *lib.NodeConfig {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	oldIndex := rm.currentIdx
	rm.currentIdx = (rm.currentIdx + 1) % len(rm.nodes)
	newNode := rm.nodes[rm.currentIdx]

	// Update stats
	rm.stats.mu.Lock()
	rm.stats.nodeFailures[oldIndex]++
	rm.stats.nodeSwitches++
	rm.stats.mu.Unlock()

	log.Warn().
		Int("from_node", oldIndex).
		Int("to_node", rm.currentIdx).
		Int("total_switches", rm.stats.nodeSwitches).
		Int("node_failures", rm.stats.nodeFailures[oldIndex]).
		Msg("Switching to next RPC node")

	return &newNode
}

func (rm *RPCManager) GetStats() (int, map[int]int) {
	rm.stats.mu.RLock()
	defer rm.stats.mu.RUnlock()

	// Create a copy of the stats to return
	failuresCopy := make(map[int]int)
	for k, v := range rm.stats.nodeFailures {
		failuresCopy[k] = v
	}

	return rm.stats.nodeSwitches, failuresCopy
}

// SendDataWithNodeRetry attempts to send data to the chain, switching nodes if necessary
func (r *RPCManager) SendDataWithNodeRetry(
	ctx context.Context,
	msg sdk.Msg,
	timeoutHeight uint64,
	operationName string,
) (*cosmosclient.Response, error) {
	// Track which nodes we've tried
	triedNodes := make(map[string]bool)
	totalNodes := len(r.nodes)

	for attempts := 0; attempts < totalNodes; attempts++ {
		currentNode := r.GetCurrentNode()
		nodeKey := currentNode.Chain.Address

		// Skip if we've already tried this node
		if triedNodes[nodeKey] {
			r.SwitchToNextNode()
			continue
		}

		// Mark this node as tried
		triedNodes[nodeKey] = true

		// Attempt to send data using existing retry mechanism
		res, err := currentNode.SendDataWithRetry(ctx, msg, operationName, timeoutHeight)
		if err == nil {
			return res, nil
		}

		// If it's a node switching error, switch to next node and continue
		if lib.IsErrorSwitchingNode(err) {
			log.Warn().
				Str("node", nodeKey).
				Str("operation", operationName).
				Msg("Switching to next node")
			r.SwitchToNextNode()
			continue
		}

		// For any other error, return it immediately
		return nil, errorsmod.Wrapf(err, "error during %s", operationName)
	}

	// If we've tried all nodes and none worked
	return nil, errorsmod.Wrapf(ErrAllNodesExhausted,
		"tried %d nodes during %s", totalNodes, operationName)
}

// RunWithNodeRetry executes an operation that returns (T, error) on nodes until success or all nodes are exhausted
func RunWithNodeRetry[T any](
	ctx context.Context,
	r RPCManagerInterface,
	operation func(*lib.NodeConfig) (T, error),
	operationName string,
) (T, error) {
	var zeroValue T

	// Track which nodes we've tried
	triedNodes := make(map[string]bool)
	nodes, err := r.GetNodes()
	if err != nil {
		return zeroValue, errorsmod.Wrapf(err, "error getting nodes")
	}
	totalNodes := len(nodes)

	for attempts := 0; attempts < totalNodes; attempts++ {
		currentNode := r.GetCurrentNode()
		nodeKey := currentNode.Chain.Address

		// Skip if we've already tried this node
		if triedNodes[nodeKey] {
			r.SwitchToNextNode()
			continue
		}

		// Mark this node as tried
		triedNodes[nodeKey] = true

		// Attempt operation on current node
		result, err := operation(currentNode)
		if err == nil {
			return result, nil
		}

		// If it's a node switching error, switch to next node and continue
		if lib.IsErrorSwitchingNode(err) {
			log.Warn().
				Str("node", nodeKey).
				Str("operation", operationName).
				Msg("Switching to next node")
			r.SwitchToNextNode()
			continue
		}

		// For any other error, return it immediately
		return zeroValue, errorsmod.Wrapf(err, "error during %s", operationName)
	}

	// If we've tried all nodes and none worked
	return zeroValue, errorsmod.Wrapf(ErrAllNodesExhausted,
		"tried %d nodes during %s", totalNodes, operationName)
}
