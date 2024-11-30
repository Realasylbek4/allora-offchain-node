package usecase

import (
	"allora_offchain_node/lib"

	"github.com/stretchr/testify/mock"
)

type MockAlloraAdapter struct {
	mock.Mock
}

func (m *MockAlloraAdapter) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockAlloraAdapter) CalcInference(config lib.WorkerConfig, timestamp int64) (string, error) {
	args := m.Called(config, timestamp)
	return args.String(0), args.Error(1)
}

func (m *MockAlloraAdapter) CalcForecast(config lib.WorkerConfig, timestamp int64) ([]lib.NodeValue, error) {
	args := m.Called(config, timestamp)
	return args.Get(0).([]lib.NodeValue), args.Error(1) // nolint: forcetypeassert
}

func (m *MockAlloraAdapter) GroundTruth(config lib.ReputerConfig, timestamp int64) (lib.Truth, error) {
	args := m.Called(config, timestamp)
	return args.Get(0).(lib.Truth), args.Error(1) // nolint: forcetypeassert
}

// Update LossFunction to match the new signature
func (m *MockAlloraAdapter) LossFunction(node lib.ReputerConfig, sourceTruth string, inferenceValue string, options map[string]string) (string, error) {
	args := m.Called(node, sourceTruth, inferenceValue, options)
	return args.String(0), args.Error(1)
}

func (m *MockAlloraAdapter) CanInfer() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockAlloraAdapter) CanForecast() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockAlloraAdapter) CanSourceGroundTruthAndComputeLoss() bool {
	args := m.Called()
	return args.Bool(0)
}

// Add the new IsLossFunctionNeverNegative method
func (m *MockAlloraAdapter) IsLossFunctionNeverNegative(node lib.ReputerConfig, options map[string]string) (bool, error) {
	args := m.Called(node, options)
	return args.Bool(0), args.Error(1)
}

func NewMockAlloraAdapter() *MockAlloraAdapter {
	m := &MockAlloraAdapter{} // nolint: exhaustruct

	return m
}

func ReturnBasicMockAlloraAdapter() *MockAlloraAdapter {
	return NewMockAlloraAdapter()
}
