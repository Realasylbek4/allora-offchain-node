package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateTimeDistanceInSeconds(t *testing.T) {
	tests := []struct {
		name                   string
		distanceUntilNextEpoch int64
		correctionFactor       float64
		blockAvgSeconds        float64
		expectedTimeDistance   int64
		expectedError          bool
	}{
		{
			name:                   "Basic calculation",
			distanceUntilNextEpoch: 100,
			correctionFactor:       1.0,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   460, // 100 * 4.6 * 1.0
			expectedError:          false,
		},
		{
			name:                   "With correction factor",
			distanceUntilNextEpoch: 100,
			correctionFactor:       0.75,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   345, // 100 * 4.6 * 0.75
			expectedError:          false,
		},
		{
			name:                   "Zero distance",
			distanceUntilNextEpoch: 0,
			correctionFactor:       1.0,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   0,
			expectedError:          false,
		},
		{
			name:                   "Large distance",
			distanceUntilNextEpoch: 1000000,
			correctionFactor:       1.0,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   4600000, // 1000000 * 4.6 * 1.0
			expectedError:          false,
		},
		{
			name:                   "Small correction factor",
			distanceUntilNextEpoch: 100,
			correctionFactor:       0.1,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   46, // 100 * 4.6 * 0.1
			expectedError:          false,
		},
		{
			name:                   "Negative distance",
			distanceUntilNextEpoch: -100,
			correctionFactor:       1.0,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   0,
			expectedError:          true,
		},
		{
			name:                   "Negative correction factor",
			distanceUntilNextEpoch: 100,
			correctionFactor:       -0.5,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   0,
			expectedError:          true,
		},
		{
			name:                   "Both negative",
			distanceUntilNextEpoch: -100,
			correctionFactor:       -0.5,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   0,
			expectedError:          true,
		},
		// tests with different blockAvgSeconds
		{
			name:                   "Basic calculation with 6s blocks",
			distanceUntilNextEpoch: 100,
			correctionFactor:       1.0,
			blockAvgSeconds:        6.0,
			expectedTimeDistance:   600, // 100 * 6.0 * 1.0
			expectedError:          false,
		},
		{
			name:                   "With correction factor and 3s blocks",
			distanceUntilNextEpoch: 100,
			correctionFactor:       0.75,
			blockAvgSeconds:        3.0,
			expectedTimeDistance:   225, // 100 * 3.0 * 0.75
			expectedError:          false,
		},
		{
			name:                   "Large distance with 5.5s blocks",
			distanceUntilNextEpoch: 1000000,
			correctionFactor:       1.0,
			blockAvgSeconds:        5.5,
			expectedTimeDistance:   5500000, // 1000000 * 5.5 * 1.0
			expectedError:          false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := calculateTimeDistanceInSeconds(test.distanceUntilNextEpoch, test.blockAvgSeconds, test.correctionFactor)
			if test.expectedError {
				assert.Error(t, err, "Expected an error for negative input")
				assert.Equal(t, int64(0), result, "Expected 0 result when error occurs")
			} else {
				assert.NoError(t, err, "Did not expect an error")
				assert.Equal(t, test.expectedTimeDistance, result, "Calculated time distance should match expected value")
			}
		})
	}
}

func TestGenerateFairRandomOffset(t *testing.T) {
	tests := []struct {
		name                   string
		workerSubmissionWindow int64
		expectedMin            int64
		expectedMax            int64
		expectedCenter         int64
		iterations             int
		allowedDeltaFromCenter float64
	}{
		{
			name:                   "Standard window",
			workerSubmissionWindow: 100,
			expectedMin:            0,
			expectedMax:            50,
			expectedCenter:         25,
			iterations:             10000,
			allowedDeltaFromCenter: 1.0,
		},
		{
			name:                   "Large window",
			workerSubmissionWindow: 1000,
			expectedMin:            0,
			expectedMax:            500,
			expectedCenter:         250,
			iterations:             10000,
			allowedDeltaFromCenter: 5.0,
		},
		{
			name:                   "Small window",
			workerSubmissionWindow: 20,
			expectedMin:            0,
			expectedMax:            10,
			expectedCenter:         5,
			iterations:             10000,
			allowedDeltaFromCenter: 0.5,
		},
		{
			name:                   "Odd window size",
			workerSubmissionWindow: 101,
			expectedMin:            0,
			expectedMax:            50,
			expectedCenter:         25,
			iterations:             10000,
			allowedDeltaFromCenter: 1.0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sum := int64(0)
			for i := 0; i < test.iterations; i++ {
				result := generateFairOffset(test.workerSubmissionWindow)
				assert.GreaterOrEqual(t, result, test.expectedMin, "Result should be greater than or equal to the minimum value")
				assert.LessOrEqual(t, result, test.expectedMax, "Result should be less than or equal to the maximum value")
				sum += result
			}

			// Check that the average is close to the center (allowing for some randomness)
			average := float64(sum) / float64(test.iterations)
			assert.InDelta(t, float64(test.expectedCenter), average, test.allowedDeltaFromCenter, "Average should be close to the center")
		})
	}
}
