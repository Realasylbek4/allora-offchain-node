package usecase

import (
	"allora_offchain_node/lib"
	"context"
	"errors"
	"math"
	"sync"
	"time"

	errorsmod "cosmossdk.io/errors"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/rand"
)

// Number of submission windows considered to be "near" the new window
// When it is near, the checks are more frequent
const SUBMISSION_WINDOWS_TO_BE_NEAR_NEW_WINDOW int64 = 2

func (suite *UseCaseSuite) Spawn() {
	var wg sync.WaitGroup

	// Run worker process per topic
	alreadyStartedWorkerForTopic := make(map[emissionstypes.TopicId]bool)
	for _, worker := range suite.Node.Worker {
		if _, ok := alreadyStartedWorkerForTopic[worker.TopicId]; ok {
			log.Debug().Uint64("topicId", worker.TopicId).Msg("Worker already started for topicId")
			continue
		}
		alreadyStartedWorkerForTopic[worker.TopicId] = true

		wg.Add(1)
		go func(worker lib.WorkerConfig) {
			defer wg.Done()
			suite.runWorkerProcess(worker)
		}(worker)
	}

	// Run reputer process per topic
	alreadyStartedReputerForTopic := make(map[emissionstypes.TopicId]bool)
	for _, reputer := range suite.Node.Reputer {
		if _, ok := alreadyStartedReputerForTopic[reputer.TopicId]; ok {
			log.Debug().Uint64("topicId", reputer.TopicId).Msg("Reputer already started for topicId")
			continue
		}
		alreadyStartedReputerForTopic[reputer.TopicId] = true

		wg.Add(1)
		go func(reputer lib.ReputerConfig) {
			defer wg.Done()
			suite.runReputerProcess(reputer)
		}(reputer)
	}

	// Wait for all goroutines to finish
	wg.Wait()
}

// Attempts to build and commit a worker payload for a given nonce
func (suite *UseCaseSuite) processWorkerPayload(worker lib.WorkerConfig, latestNonceHeightActedUpon int64) (int64, error) {
	latestOpenWorkerNonce, err := lib.QueryDataWithRetry(
		context.Background(),
		suite.Node.Wallet.MaxRetries,
		time.Duration(suite.Node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.Nonce, error) {
			return suite.Node.GetLatestOpenWorkerNonceByTopicId(worker.TopicId)
		},
		query.PageRequest{}, // Empty page request as GetLatestOpenWorkerNonceByTopicId doesn't use pagination
	)

	if err != nil {
		log.Warn().Err(err).Uint64("topicId", worker.TopicId).Msg("Error getting latest open worker nonce on topic - node availability issue?")
		return latestNonceHeightActedUpon, err
	}

	if latestOpenWorkerNonce.BlockHeight > latestNonceHeightActedUpon {
		log.Debug().Uint64("topicId", worker.TopicId).Int64("BlockHeight", latestOpenWorkerNonce.BlockHeight).
			Msg("Building and committing worker payload for topic")

		err := suite.BuildCommitWorkerPayload(worker, latestOpenWorkerNonce)
		if err != nil {
			return latestNonceHeightActedUpon, errorsmod.Wrapf(err, "error building and committing worker payload for topic")
		}
		return latestOpenWorkerNonce.BlockHeight, nil
	} else {
		log.Debug().Uint64("topicId", worker.TopicId).
			Int64("BlockHeight", latestOpenWorkerNonce.BlockHeight).
			Int64("latestNonceHeightActedUpon", latestNonceHeightActedUpon).Msg("No new worker nonce found")
		return latestNonceHeightActedUpon, nil
	}
}

func (suite *UseCaseSuite) processReputerPayload(reputer lib.ReputerConfig, latestNonceHeightActedUpon int64) (int64, error) {
	nonce, err := lib.QueryDataWithRetry(
		context.Background(),
		suite.Node.Wallet.MaxRetries,
		time.Duration(suite.Node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.Nonce, error) {
			return suite.Node.GetOldestReputerNonceByTopicId(reputer.TopicId)
		},
		query.PageRequest{}, // Empty page request as GetLatestOpenWorkerNonceByTopicId doesn't use pagination
	)

	if err != nil {
		log.Warn().Err(err).Uint64("topicId", reputer.TopicId).Msg("Error getting latest open worker nonce on topic - node availability issue?")
		return latestNonceHeightActedUpon, err
	}

	if nonce.BlockHeight > latestNonceHeightActedUpon {
		log.Debug().Uint64("topicId", reputer.TopicId).Int64("BlockHeight", nonce.BlockHeight).
			Msg("Building and committing worker payload for topic")

		err := suite.BuildCommitReputerPayload(reputer, nonce.BlockHeight)
		if err != nil {
			return latestNonceHeightActedUpon, errorsmod.Wrapf(err, "error building and committing worker payload for topic")
		}
		return nonce.BlockHeight, nil
	} else {
		log.Debug().Uint64("topicId", reputer.TopicId).
			Int64("BlockHeight", nonce.BlockHeight).
			Int64("latestNonceHeightActedUpon", latestNonceHeightActedUpon).Msg("No new worker nonce found")
		return latestNonceHeightActedUpon, nil
	}
}

// Calculate the time distance based on the distance until the next epoch
func calculateTimeDistanceInSeconds(distanceUntilNextEpoch int64, blockDurationAvg, correctionFactor float64) (int64, error) {
	if distanceUntilNextEpoch < 0 || correctionFactor < 0 {
		return 0, errors.New("distanceUntilNextEpoch and correctionFactor must be positive")
	}
	correctedTimeDistance := float64(distanceUntilNextEpoch) * blockDurationAvg * correctionFactor
	return int64(math.Round(correctedTimeDistance)), nil
}

func generateFairOffset(workerSubmissionWindow int64) int64 {
	// Ensure the random number generator is seeded
	source := rand.NewSource(uint64(time.Now().UnixNano()))
	rng := rand.New(source)

	// Calculate the center of the window
	center := workerSubmissionWindow / 2

	// Generate a random number between -maxOffset and +maxOffset
	offset := rng.Int63n(center + 1)

	return offset
}

func (suite *UseCaseSuite) runWorkerProcess(worker lib.WorkerConfig) {
	log.Info().Uint64("topicId", worker.TopicId).Msg("Running worker process for topic")

	registered := suite.Node.RegisterWorkerIdempotently(worker)
	if !registered {
		log.Error().Uint64("topicId", worker.TopicId).Msg("Failed to register worker for topic")
		return
	}
	log.Debug().Uint64("topicId", worker.TopicId).Msg("Worker registered")

	topicInfo, err := lib.QueryDataWithRetry(
		context.Background(),
		suite.Node.Wallet.MaxRetries,
		time.Duration(suite.Node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.Topic, error) {
			return suite.Node.GetTopicInfo(worker.TopicId)
		},
		query.PageRequest{}, // Empty page request as GetTopicInfo doesn't use pagination
	)
	if err != nil {
		log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Failed to get topic info after retries")
		return
	}

	// Get epoch length and worker submission window static info
	epochLength := topicInfo.EpochLength
	workerSubmissionWindow := topicInfo.WorkerSubmissionWindow
	minBlocksToCheck := workerSubmissionWindow * SUBMISSION_WINDOWS_TO_BE_NEAR_NEW_WINDOW

	// Last nonce successfully sent tx for
	latestNonceHeightSentTxFor := int64(0)

	// Keep this to estimate block duration
	var currentBlockHeight int64

	for {
		// Query the latest block
		status, err := suite.Node.Chain.Client.Status(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("Failed to get status")
			suite.Wait(1)
			continue
		}
		currentBlockHeight = status.SyncInfo.LatestBlockHeight

		topicInfo, err := lib.QueryDataWithRetry(
			context.Background(),
			suite.Node.Wallet.MaxRetries,
			time.Duration(suite.Node.Wallet.RetryDelay)*time.Second,
			func(ctx context.Context, req query.PageRequest) (*emissionstypes.Topic, error) {
				return suite.Node.GetTopicInfo(worker.TopicId)
			},
			query.PageRequest{}, // Empty page request as GetTopicInfo doesn't use pagination
		)
		if err != nil {
			log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Error getting topic info")
			return
		}
		log.Debug().Int64("currentBlockHeight", currentBlockHeight).
			Int64("EpochLastEnded", topicInfo.EpochLastEnded).
			Int64("EpochLength", epochLength).
			Msg("Info from topic")
		epochLastEnded := topicInfo.EpochLastEnded
		epochEnd := epochLastEnded + epochLength

		// Check if block is within the epoch submission window
		if currentBlockHeight-epochLastEnded <= workerSubmissionWindow {
			// Attempt to submit worker payload
			latestNonceHeightSentTxFor, err = suite.processWorkerPayload(worker, latestNonceHeightSentTxFor)
			if err != nil {
				log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Error processing worker payload - could not complete transaction")
			} else {
				log.Debug().Uint64("topicId", worker.TopicId).Msg("Successfully sent worker payload")
			}
			// Wait until the epoch submission window opens
			distanceUntilNextEpoch := epochEnd - currentBlockHeight
			correctedTimeDistanceInSeconds, err := calculateTimeDistanceInSeconds(distanceUntilNextEpoch,
				suite.Node.Wallet.BlockDurationEstimated, suite.Node.Wallet.WindowCorrectionFactor)
			if err != nil {
				log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Error calculating time distance to next epoch after sending tx")
				return
			}
			log.Debug().Uint64("topicId", worker.TopicId).
				Int64("currentBlockHeight", currentBlockHeight).
				Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
				Int64("correctedTimeDistanceInSeconds", correctedTimeDistanceInSeconds).
				Msg("Waiting until the epoch submission window opens")
			suite.Wait(correctedTimeDistanceInSeconds)
		} else if currentBlockHeight > epochEnd {
			correctedTimeDistanceInSeconds, err := calculateTimeDistanceInSeconds(epochLength, suite.Node.Wallet.BlockDurationEstimated, 1.0)
			if err != nil {
				log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("epochLength and correctionFactor must be positive")
				return
			}
			log.Warn().Uint64("topicId", worker.TopicId).Msg("Current block height is greater than next epoch length, inactive topic? Waiting one epoch length")
			suite.Wait(correctedTimeDistanceInSeconds)
		} else {
			// Check distance until next epoch
			distanceUntilNextEpoch := epochEnd - currentBlockHeight

			if distanceUntilNextEpoch <= minBlocksToCheck {
				// Wait until the center of the epoch submission window
				offset := generateFairOffset(workerSubmissionWindow)
				closeBlockDistance := distanceUntilNextEpoch + offset
				correctedTimeDistanceInSeconds, err := calculateTimeDistanceInSeconds(closeBlockDistance, suite.Node.Wallet.BlockDurationEstimated, 1.0)
				if err != nil {
					log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Error calculating close distance to epochLength")
					return
				}
				log.Debug().Uint64("topicId", worker.TopicId).
					Int64("offset", offset).
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("closeBlockDistance", closeBlockDistance).
					Int64("correctedTimeDistanceInSeconds", correctedTimeDistanceInSeconds).
					Msg("Close to the window, waiting until next epoch submission window")
				suite.Wait(correctedTimeDistanceInSeconds)
			} else {
				// Wait until the epoch submission window opens
				correctedTimeDistanceInSeconds, err := calculateTimeDistanceInSeconds(distanceUntilNextEpoch,
					suite.Node.Wallet.BlockDurationEstimated, suite.Node.Wallet.WindowCorrectionFactor)
				if err != nil {
					log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Error calculating far distance to epochLength")
					return
				}
				log.Debug().Uint64("topicId", worker.TopicId).
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("correctedTimeDistanceInSeconds", correctedTimeDistanceInSeconds).
					Msg("Waiting until the epoch submission window opens")
				suite.Wait(correctedTimeDistanceInSeconds)
			}
		}
	}
}

func (suite *UseCaseSuite) runReputerProcess(reputer lib.ReputerConfig) {
	log.Debug().Uint64("topicId", reputer.TopicId).Msg("Running reputer process for topic")

	registeredAndStaked := suite.Node.RegisterAndStakeReputerIdempotently(reputer)
	if !registeredAndStaked {
		log.Error().Uint64("topicId", reputer.TopicId).Msg("Failed to register or sufficiently stake reputer for topic")
		return
	}
	log.Debug().Uint64("topicId", reputer.TopicId).Msg("Reputer registered and staked")

	topicInfo, err := lib.QueryDataWithRetry(
		context.Background(),
		suite.Node.Wallet.MaxRetries,
		time.Duration(suite.Node.Wallet.RetryDelay)*time.Second,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.Topic, error) {
			return suite.Node.GetTopicInfo(reputer.TopicId)
		},
		query.PageRequest{}, // Empty page request as GetTopicInfo doesn't use pagination
	)
	if err != nil {
		log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Failed to get topic info after retries")
		return
	}

	// Get epoch length and worker submission window static info
	epochLength := topicInfo.EpochLength
	workerSubmissionWindow := topicInfo.WorkerSubmissionWindow
	minBlocksToCheck := workerSubmissionWindow * SUBMISSION_WINDOWS_TO_BE_NEAR_NEW_WINDOW

	// Last nonce successfully sent tx for
	latestNonceHeightSentTxFor := int64(0)

	// Keep this to estimate block duration
	var currentBlockHeight int64

	for {
		log.Trace().Msg("Starting reputer process loop")
		// Query the latest block
		status, err := suite.Node.Chain.Client.Status(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("Failed to get status")
			suite.Wait(1)
			continue
		}
		currentBlockHeight = status.SyncInfo.LatestBlockHeight

		topicInfo, err := lib.QueryDataWithRetry(
			context.Background(),
			suite.Node.Wallet.MaxRetries,
			time.Duration(suite.Node.Wallet.RetryDelay)*time.Second,
			func(ctx context.Context, req query.PageRequest) (*emissionstypes.Topic, error) {
				return suite.Node.GetTopicInfo(reputer.TopicId)
			},
			query.PageRequest{}, // Empty page request as GetTopicInfo doesn't use pagination
		)
		if err != nil {
			log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Error getting topic info")
			return
		}
		log.Debug().Int64("currentBlockHeight", currentBlockHeight).
			Int64("EpochLastEnded", topicInfo.EpochLastEnded).
			Int64("EpochLength", epochLength).
			Msg("Info from topic")
		epochLastEnded := topicInfo.EpochLastEnded
		epochEnd := epochLastEnded + epochLength

		// Check if block is within the epoch submission window
		if currentBlockHeight-epochLastEnded <= epochLength {
			// Attempt to submit worker payload
			latestNonceHeightSentTxFor, err = suite.processReputerPayload(reputer, latestNonceHeightSentTxFor)
			if err != nil {
				log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Error processing reputer payload - could not complete transaction")
			} else {
				log.Debug().Uint64("topicId", reputer.TopicId).Msg("Successfully sent reputer payload")
			}
			// Wait until the epoch submission window opens
			distanceUntilNextEpoch := epochEnd - currentBlockHeight
			correctedTimeDistanceInSeconds, err := calculateTimeDistanceInSeconds(distanceUntilNextEpoch,
				suite.Node.Wallet.BlockDurationEstimated, suite.Node.Wallet.WindowCorrectionFactor)
			if err != nil {
				log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Error calculating time distance to next epoch after sending tx")
				return
			}
			log.Debug().Uint64("topicId", reputer.TopicId).
				Int64("currentBlockHeight", currentBlockHeight).
				Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
				Int64("correctedTimeDistanceInSeconds", correctedTimeDistanceInSeconds).
				Msg("Waiting until the epoch submission window opens")
			suite.Wait(correctedTimeDistanceInSeconds)
		} else if currentBlockHeight > epochEnd {
			correctedTimeDistanceInSeconds, err := calculateTimeDistanceInSeconds(epochLength, suite.Node.Wallet.BlockDurationEstimated, 1.0)
			if err != nil {
				log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("epochLength and correctionFactor must be positive")
				return
			}
			log.Warn().Uint64("topicId", reputer.TopicId).Msg("Current block height is greater than next epoch length, inactive topic? Waiting one epoch length")
			suite.Wait(correctedTimeDistanceInSeconds)
		} else {
			// Check distance until next epoch
			distanceUntilNextEpoch := epochEnd - currentBlockHeight

			if distanceUntilNextEpoch <= minBlocksToCheck {
				// Wait until the center of the epoch submission window
				offset := generateFairOffset(epochLength)
				closeBlockDistance := distanceUntilNextEpoch + offset
				correctedTimeDistanceInSeconds, err := calculateTimeDistanceInSeconds(closeBlockDistance, suite.Node.Wallet.BlockDurationEstimated, 1.0)
				if err != nil {
					log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Error calculating close distance to epochLength")
					return
				}
				log.Debug().Uint64("topicId", reputer.TopicId).
					Int64("offset", offset).
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("closeBlockDistance", closeBlockDistance).
					Int64("correctedTimeDistanceInSeconds", correctedTimeDistanceInSeconds).
					Msg("Close to the window, waiting until next epoch submission window")
				suite.Wait(correctedTimeDistanceInSeconds)
			} else {
				// Wait until the epoch submission window opens
				correctedTimeDistanceInSeconds, err := calculateTimeDistanceInSeconds(distanceUntilNextEpoch,
					suite.Node.Wallet.BlockDurationEstimated, suite.Node.Wallet.WindowCorrectionFactor)
				if err != nil {
					log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Error calculating far distance to epochLength")
					return
				}
				log.Debug().Uint64("topicId", reputer.TopicId).
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("correctedTimeDistanceInSeconds", correctedTimeDistanceInSeconds).
					Msg("Waiting until the epoch submission window opens")
				suite.Wait(correctedTimeDistanceInSeconds)
			}
		}
	}

}
