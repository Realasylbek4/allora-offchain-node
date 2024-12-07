package usecase

import (
	"allora_offchain_node/lib"
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/stretchr/testify/mock"
)

type MockRPCManager struct {
	mock.Mock
}

func (m *MockRPCManager) GetCurrentNode() *lib.NodeConfig {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*lib.NodeConfig)
}

func (m *MockRPCManager) SwitchToNextNode() *lib.NodeConfig {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*lib.NodeConfig)
}

func (m *MockRPCManager) GetStats() (int, map[int]int) {
	args := m.Called()
	return args.Int(0), args.Get(1).(map[int]int)
}

func (m *MockRPCManager) GetNodes() ([]lib.NodeConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]lib.NodeConfig), args.Error(1)
}

func (m *MockRPCManager) SendDataWithNodeRetry(ctx context.Context, msg sdk.Msg, timeoutHeight uint64, operationName string) (*cosmosclient.Response, error) {
	args := m.Called(ctx, msg, timeoutHeight, operationName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cosmosclient.Response), args.Error(1)
}
