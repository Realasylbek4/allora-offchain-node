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
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/rand"
)

// Number of submission windows considered to be "near" the next window
// When time is near, the control is more accurate
const NUM_SUBMISSION_WINDOWS_FOR_SUBMISSION_NEARNESS int64 = 2

// Correction factor used when calculating time distances near window
// Waiting times under nearness circumstances are adjusted by this factor
const NEARNESS_CORRECTION_FACTOR float64 = 1.0

// Minimum wait time between status checks
const WAIT_TIME_STATUS_CHECKS int64 = 1

// ActorProcessParams encapsulates the configuration needed for running actor processes
type ActorProcessParams[T lib.TopicActor] struct {
	// Configuration for the actor (Worker or Reputer)
	Config T
	// Function to process payloads (processWorkerPayload or processReputerPayload)
	ProcessPayload func(T, int64, uint64) (int64, error)
	// Function to get nonces (GetLatestOpenWorkerNonceByTopicId or GetOldestReputerNonceByTopicId)
	GetNonce func(emissionstypes.TopicId) (*emissionstypes.Nonce, error)
	// Window length used to determine when we're near submission time
	NearWindowLength int64
	// Actual submission window length
	SubmissionWindowLength int64
	// Actor type for logging ("worker" or "reputer")
	ActorType string
}

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
func (suite *UseCaseSuite) processWorkerPayload(worker lib.WorkerConfig, latestNonceHeightActedUpon int64, timeoutHeight uint64) (int64, error) {
	latestOpenWorkerNonce, err := suite.Node.GetLatestOpenWorkerNonceByTopicId(worker.TopicId)

	if err != nil {
		log.Warn().Err(err).Uint64("topicId", worker.TopicId).Msg("Error getting latest open worker nonce on topic - node availability issue?")
		return latestNonceHeightActedUpon, err
	}

	if latestOpenWorkerNonce.BlockHeight > latestNonceHeightActedUpon {
		log.Debug().Uint64("topicId", worker.TopicId).Int64("BlockHeight", latestOpenWorkerNonce.BlockHeight).
			Msg("Building and committing worker payload for topic")

		err := suite.BuildCommitWorkerPayload(worker, latestOpenWorkerNonce, timeoutHeight)
		if err != nil {
			return latestNonceHeightActedUpon, errorsmod.Wrapf(err, "error building and committing worker payload for topic")
		}
		log.Debug().Uint64("topicId", uint64(worker.TopicId)).
			Str("actorType", "worker").
			Msg("Successfully finished processing payload")
		return latestOpenWorkerNonce.BlockHeight, nil
	} else {
		log.Debug().Uint64("topicId", worker.TopicId).
			Int64("LastOpenNonceBlockHeight", latestOpenWorkerNonce.BlockHeight).
			Int64("latestNonceHeightActedUpon", latestNonceHeightActedUpon).Msg("No new worker nonce found")
		return latestNonceHeightActedUpon, nil
	}
}

func (suite *UseCaseSuite) processReputerPayload(reputer lib.ReputerConfig, latestNonceHeightActedUpon int64, timeoutHeight uint64) (int64, error) {
	nonce, err := suite.Node.GetOldestReputerNonceByTopicId(reputer.TopicId)

	if err != nil {
		log.Warn().Err(err).Uint64("topicId", reputer.TopicId).Msg("Error getting latest open reputer nonce on topic - node availability issue?")
		return latestNonceHeightActedUpon, err
	}

	if nonce.BlockHeight > latestNonceHeightActedUpon {
		log.Debug().Uint64("topicId", reputer.TopicId).Int64("BlockHeight", nonce.BlockHeight).
			Msg("Building and committing reputer payload for topic")

		err := suite.BuildCommitReputerPayload(reputer, nonce.BlockHeight, timeoutHeight)
		if err != nil {
			return latestNonceHeightActedUpon, errorsmod.Wrapf(err, "error building and committing reputer payload for topic")
		}
		log.Debug().Uint64("topicId", reputer.TopicId).
			Str("actorType", "reputer").
			Msg("Successfully finished processing payload")
		return nonce.BlockHeight, nil
	} else {
		log.Debug().Uint64("topicId", reputer.TopicId).
			Int64("LastOpenNonceBlockHeight", nonce.BlockHeight).
			Int64("latestNonceHeightActedUpon", latestNonceHeightActedUpon).Msg("No new reputer nonce found")
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

// Generates a conservative random offset within the submission window
func generateFairOffset(submissionWindow int64) int64 {
	// Ensure the random number generator is seeded
	source := rand.NewSource(uint64(time.Now().UnixNano()))
	rng := rand.New(source)

	// Calculate the center of the window
	center := submissionWindow / 2

	// Generate a random number between start and window center
	offset := rng.Int63n(center + 1)

	return offset
}

func (suite *UseCaseSuite) runWorkerProcess(worker lib.WorkerConfig) {
	log.Info().Uint64("topicId", worker.TopicId).Msg("Running worker process for topic")

	// Handle registration
	registered := suite.Node.RegisterWorkerIdempotently(worker)
	if !registered {
		log.Fatal().Uint64("topicId", worker.TopicId).Msg("Failed to register worker for topic, exiting")
		return
	}
	log.Debug().Uint64("topicId", worker.TopicId).Msg("Worker registered")

	// Using the helper function
	topicInfo, err := queryTopicInfo(suite, worker, "worker", "topic info: worker")
	if err != nil {
		log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Failed to get topic info for worker")
		return
	}

	params := ActorProcessParams[lib.WorkerConfig]{
		Config:                 worker,
		ProcessPayload:         suite.processWorkerPayload,
		GetNonce:               suite.Node.GetLatestOpenWorkerNonceByTopicId,
		NearWindowLength:       topicInfo.WorkerSubmissionWindow, // Use worker window to determine "nearness"
		SubmissionWindowLength: topicInfo.WorkerSubmissionWindow, // Use worker window for actual submission window
		ActorType:              "worker",
	}

	runActorProcess(suite, params)
}

func (suite *UseCaseSuite) runReputerProcess(reputer lib.ReputerConfig) {
	log.Debug().Uint64("topicId", reputer.TopicId).Msg("Running reputer process for topic")

	// Handle registration and staking
	registeredAndStaked := suite.Node.RegisterAndStakeReputerIdempotently(reputer)
	if !registeredAndStaked {
		log.Fatal().Uint64("topicId", reputer.TopicId).Msg("Failed to register or sufficiently stake reputer for topic")
		return
	}
	log.Debug().Uint64("topicId", reputer.TopicId).Msg("Reputer registered and staked")

	// Using the helper function
	topicInfo, err := queryTopicInfo(suite, reputer, "reputer", "topic info: reputer")
	if err != nil {
		log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Failed to get topic info for reputer")
		return
	}

	params := ActorProcessParams[lib.ReputerConfig]{
		Config:                 reputer,
		ProcessPayload:         suite.processReputerPayload,
		GetNonce:               suite.Node.GetOldestReputerNonceByTopicId,
		NearWindowLength:       topicInfo.WorkerSubmissionWindow, // Use worker window to determine "nearness"
		SubmissionWindowLength: topicInfo.EpochLength,            // Use epoch length for actual submission window
		ActorType:              "reputer",
	}

	runActorProcess(suite, params)
}

// Function that runs the actor process for a given topic and actor type
// This mechanism is used to handle the submission of payloads for both workers and reputers,
// using ActorProcessParams to handle the different configurations and functions needed for each actor type
func runActorProcess[T lib.TopicActor](suite *UseCaseSuite, params ActorProcessParams[T]) {
	log.Debug().
		Uint64("topicId", uint64(params.Config.GetTopicId())).
		Str("actorType", params.ActorType).
		Msg("Running actor process for topic")

	topicInfo, err := queryTopicInfo(suite, params.Config, params.ActorType, "topic info: actor process")
	if err != nil {
		log.Error().
			Err(err).
			Uint64("topicId", uint64(params.Config.GetTopicId())).
			Str("actorType", params.ActorType).
			Msg("Failed to get topic info after retries")
		return
	}

	epochLength := topicInfo.EpochLength
	minBlocksToCheck := params.NearWindowLength * NUM_SUBMISSION_WINDOWS_FOR_SUBMISSION_NEARNESS
	latestNonceHeightSentTxFor := int64(0)
	var currentBlockHeight int64

	for {
		log.Trace().Msg("Start iteration, querying latest block")
		// Query the latest block
		status, err := suite.Node.Chain.Client.Status(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("Failed to get status")
			suite.Wait(WAIT_TIME_STATUS_CHECKS)
			continue
		}
		currentBlockHeight = status.SyncInfo.LatestBlockHeight

		topicInfo, err := queryTopicInfo(suite, params.Config, params.ActorType, "topic info: actor process")
		if err != nil {
			log.Error().
				Err(err).
				Uint64("topicId", uint64(params.Config.GetTopicId())).
				Str("actorType", params.ActorType).
				Msg("Error getting topic info")
			return
		}
		log.Trace().
			Int64("currentBlockHeight", currentBlockHeight).
			Int64("EpochLastEnded", topicInfo.EpochLastEnded).
			Int64("EpochLength", epochLength).
			Msg("Info from topic")

		epochLastEnded := topicInfo.EpochLastEnded
		epochEnd := epochLastEnded + epochLength
		timeoutHeight := epochLastEnded + params.SubmissionWindowLength

		// Check if block is within the submission window
		if currentBlockHeight-epochLastEnded <= params.SubmissionWindowLength {
			// Within the submission window, attempt to process payload
			latestNonceHeightSentTxFor, err = params.ProcessPayload(params.Config, latestNonceHeightSentTxFor, uint64(timeoutHeight))
			if err != nil {
				log.Error().
					Err(err).
					Uint64("topicId", uint64(params.Config.GetTopicId())).
					Str("actorType", params.ActorType).
					Msg("Error processing payload - could not complete transaction")
			}

			distanceUntilNextEpoch := epochEnd - currentBlockHeight
			waitingTimeInSeconds, err := calculateTimeDistanceInSeconds(
				distanceUntilNextEpoch,
				suite.Node.Wallet.BlockDurationEstimated,
				suite.Node.Wallet.WindowCorrectionFactor,
			)
			if err != nil {
				log.Error().
					Err(err).
					Uint64("topicId", uint64(params.Config.GetTopicId())).
					Str("actorType", params.ActorType).
					Msg("Error calculating time distance to next epoch after sending tx")
				return
			}

			log.Info().
				Uint64("topicId", uint64(params.Config.GetTopicId())).
				Str("actorType", params.ActorType).
				Int64("currentBlockHeight", currentBlockHeight).
				Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
				Int64("waitingTimeInSeconds", waitingTimeInSeconds).
				Msg("Waiting until the submission window opens after sending")
			suite.Wait(waitingTimeInSeconds)
		} else if currentBlockHeight > epochEnd {
			// Inconsistent topic data, wait until the next epoch
			waitingTimeInSeconds, err := calculateTimeDistanceInSeconds(
				epochLength,
				suite.Node.Wallet.BlockDurationEstimated,
				NEARNESS_CORRECTION_FACTOR,
			)
			if err != nil {
				log.Error().
					Err(err).
					Uint64("topicId", uint64(params.Config.GetTopicId())).
					Str("actorType", params.ActorType).
					Msg("Error calculating time distance to next epoch after sending tx")
				return
			}
			log.Warn().
				Uint64("topicId", uint64(params.Config.GetTopicId())).
				Str("actorType", params.ActorType).
				Int64("waitingTimeInSeconds", waitingTimeInSeconds).
				Msg("Current block height is greater than next epoch length, inactive topic? Waiting seconds...")
			suite.Wait(waitingTimeInSeconds)
		} else {
			distanceUntilNextEpoch := epochEnd - currentBlockHeight
			if distanceUntilNextEpoch <= minBlocksToCheck {
				// Close distance, check more closely until the submission window opens
				offset := generateFairOffset(params.SubmissionWindowLength)
				closeBlockDistance := distanceUntilNextEpoch + offset
				waitingTimeInSeconds, err := calculateTimeDistanceInSeconds(
					closeBlockDistance,
					suite.Node.Wallet.BlockDurationEstimated,
					NEARNESS_CORRECTION_FACTOR,
				)
				if err != nil {
					log.Error().
						Err(err).
						Uint64("topicId", uint64(params.Config.GetTopicId())).
						Str("actorType", params.ActorType).
						Msg("Error calculating close distance to epochLength")
					return
				}
				log.Info().
					Uint64("topicId", uint64(params.Config.GetTopicId())).
					Str("actorType", params.ActorType).
					Int64("SubmissionWindowLength", params.SubmissionWindowLength).
					Int64("offset", offset).
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("closeBlockDistance", closeBlockDistance).
					Int64("waitingTimeInSeconds", waitingTimeInSeconds).
					Msg("Close to the window, waiting until next submission window")
				suite.Wait(waitingTimeInSeconds)
			} else {
				// Far distance, bigger waits until the submission window opens
				waitingTimeInSeconds, err := calculateTimeDistanceInSeconds(
					distanceUntilNextEpoch,
					suite.Node.Wallet.BlockDurationEstimated,
					suite.Node.Wallet.WindowCorrectionFactor,
				)
				if err != nil {
					log.Error().
						Err(err).
						Uint64("topicId", uint64(params.Config.GetTopicId())).
						Str("actorType", params.ActorType).
						Msg("Error calculating far distance to epochLength")
					return
				}
				log.Info().
					Uint64("topicId", uint64(params.Config.GetTopicId())).
					Str("actorType", params.ActorType).
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("waitingTimeInSeconds", waitingTimeInSeconds).
					Msg("Waiting until the submission window opens - far distance")
				suite.Wait(waitingTimeInSeconds)
			}
		}
	}
}

// Queries the topic info for a given actor type and wallet params from suite
// Wrapper over NodeConfig.GetTopicInfo() with generic config type
func queryTopicInfo[T lib.TopicActor](
	suite *UseCaseSuite,
	config T,
	actorType string,
	infoMsg string,
) (*emissionstypes.Topic, error) {
	topicInfo, err := suite.Node.GetTopicInfo(config.GetTopicId())
	if err != nil {
		return nil, errorsmod.Wrapf(err, "failed to get topic info")
	}
	return topicInfo, nil
}
