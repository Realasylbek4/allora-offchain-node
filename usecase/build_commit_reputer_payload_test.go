package usecase

import (
	"allora_offchain_node/lib"
	"errors"
	"testing"

	alloraMath "github.com/allora-network/allora-chain/math"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestComputeLossBundle(t *testing.T) {
	reputerOptions := map[string]string{
		"method": "sqe",
	}
	reputerConfig := lib.ReputerConfig{ // nolint: exhaustruct
		LossFunctionParameters: lib.LossFunctionParameters{ // nolint: exhaustruct
			LossMethodOptions: reputerOptions,
			IsNeverNegative:   &[]bool{false}[0],
		},
	}

	tests := []struct {
		name                string
		sourceTruth         string
		valueBundle         *emissionstypes.ValueBundle
		reputerConfig       lib.ReputerConfig
		expectedLossStrings map[string]string
		mockSetup           func(*MockAlloraAdapter)
		expectError         bool
		errorContains       string
	}{
		{ // nolint: exhaustruct
			name:        "Happy path - all positive values",
			sourceTruth: "10.0",
			valueBundle: func() *emissionstypes.ValueBundle {
				combined, _ := alloraMath.NewDecFromString("9.5")
				naive, _ := alloraMath.NewDecFromString("9.0")
				inferer, _ := alloraMath.NewDecFromString("9.7")
				forecaster, _ := alloraMath.NewDecFromString("9.8")
				return &emissionstypes.ValueBundle{ // nolint: exhaustruct
					CombinedValue: combined,
					NaiveValue:    naive,
					InfererValues: []*emissionstypes.WorkerAttributedValue{
						{Value: inferer},
					},
					ForecasterValues: []*emissionstypes.WorkerAttributedValue{
						{Value: forecaster},
					},
				}
			}(),
			reputerConfig: reputerConfig,
			expectedLossStrings: map[string]string{
				"CombinedValue":    "0.25",
				"NaiveValue":       "1.00",
				"InfererValues":    "0.09",
				"ForecasterValues": "0.04",
			},
			mockSetup: func(m *MockAlloraAdapter) {
				m.On("LossFunction", mock.AnythingOfType("lib.ReputerConfig"), "10.0", "9.5", reputerOptions).Return("0.25", nil)
				m.On("LossFunction", mock.AnythingOfType("lib.ReputerConfig"), "10.0", "9.0", reputerOptions).Return("1.00", nil)
				m.On("LossFunction", mock.AnythingOfType("lib.ReputerConfig"), "10.0", "9.7", reputerOptions).Return("0.09", nil)
				m.On("LossFunction", mock.AnythingOfType("lib.ReputerConfig"), "10.0", "9.8", reputerOptions).Return("0.04", nil)
				// m.On("IsLossFunctionNeverNegative", mock.AnythingOfType("lib.ReputerConfig"), mock.AnythingOfType("string")).Return(true, nil)
			},
			expectError: false,
		},
		{ // nolint: exhaustruct
			name:        "Error in LossFunction",
			sourceTruth: "10.0",
			valueBundle: func() *emissionstypes.ValueBundle {
				combined, _ := alloraMath.NewDecFromString("9.5")
				return &emissionstypes.ValueBundle{ // nolint: exhaustruct
					CombinedValue: combined,
				}
			}(),
			reputerConfig: reputerConfig,
			mockSetup: func(m *MockAlloraAdapter) {
				m.On("LossFunction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", errors.New("loss function error"))
			},
			expectError:   true,
			errorContains: "error computing loss for combined value",
		},
		{ // nolint: exhaustruct
			name:        "Invalid loss value",
			sourceTruth: "10.0",
			valueBundle: func() *emissionstypes.ValueBundle {
				combined, _ := alloraMath.NewDecFromString("9.5")
				return &emissionstypes.ValueBundle{ // nolint: exhaustruct
					CombinedValue: combined,
				}
			}(),
			reputerConfig: reputerConfig,
			mockSetup: func(m *MockAlloraAdapter) {
				m.On("LossFunction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("invalid", nil)
			},
			expectError:   true,
			errorContains: "error parsing loss",
		},
		{ // nolint: exhaustruct
			name:          "Nil ValueBundle",
			sourceTruth:   "10.0",
			valueBundle:   nil,
			reputerConfig: reputerConfig,
			mockSetup:     func(m *MockAlloraAdapter) {},
			expectError:   true,
			errorContains: "nil ValueBundle",
		},
		{ // nolint: exhaustruct
			name:          "Empty ValueBundle",
			sourceTruth:   "10.0",
			valueBundle:   &emissionstypes.ValueBundle{}, // nolint: exhaustruct
			reputerConfig: reputerConfig,
			mockSetup:     func(m *MockAlloraAdapter) {},
			expectError:   true,
			errorContains: "empty ValueBundle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAdapter := ReturnBasicMockAlloraAdapter()
			tt.mockSetup(mockAdapter)
			tt.reputerConfig.GroundTruthEntrypoint = mockAdapter
			tt.reputerConfig.LossFunctionEntrypoint = mockAdapter

			suite := &UseCaseSuite{} // nolint: exhaustruct
			result, err := suite.ComputeLossBundle(tt.sourceTruth, tt.valueBundle, tt.reputerConfig)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedLossStrings["CombinedValue"], result.CombinedValue.String(), "Mismatch for CombinedValue")
				assert.Equal(t, tt.expectedLossStrings["NaiveValue"], result.NaiveValue.String(), "Mismatch for NaiveValue")
				for i, inferer := range result.InfererValues {
					assert.Equal(t, tt.expectedLossStrings["InfererValues"], inferer.Value.String(), "Mismatch for InfererValue %d", i)
				}
				for i, forecaster := range result.ForecasterValues {
					assert.Equal(t, tt.expectedLossStrings["ForecasterValues"], forecaster.Value.String(), "Mismatch for ForecasterValue %d", i)
				}
			}

			mockAdapter.AssertExpectations(t)
		})
	}
}
