package usecase

import (
	"allora_offchain_node/lib"
	"context"
	"errors"
	"math"
	"strconv"
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

// Correction factor used when calculating time distances for new topics
const NEW_TOPIC_CORRECTION_FACTOR float64 = 0.5

// Minimum wait time between status checks
const WAIT_TIME_STATUS_CHECKS int64 = 2

// ActorProcessParams encapsulates the configuration needed for running actor processes
type ActorProcessParams[T lib.TopicActor] struct {
	// Configuration for the actor (Worker or Reputer)
	Config T
	// Function to process payloads (processWorkerPayload or processReputerPayload)
	ProcessPayload func(context.Context, T, int64, uint64) (int64, error)
	// Function to get nonces (GetLatestOpenWorkerNonceByTopicId or GetOldestReputerNonceByTopicId)
	GetNonce func(context.Context, emissionstypes.TopicId) (*emissionstypes.Nonce, error)
	// Window length used to determine when we're near submission time
	NearWindowLength int64
	// Actual submission window length
	SubmissionWindowLength int64
	// Actor type for logging ("worker" or "reputer")
	ActorType string
}

// UpdateGasPriceRoutine continuously updates the gas price at a specified interval
func (suite *UseCaseSuite) UpdateGasPriceRoutine(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Updating fee price routine: terminating.")
			return
		default:
			price, err := RunWithNodeRetry(
				ctx,
				suite.RPCManager,
				func(node *lib.NodeConfig) (float64, error) {
					return node.GetBaseFee(ctx)
				},
				"get base fee",
			)
			if err != nil {
				log.Error().Err(err).Msg("Error updating gas prices")
			}
			lib.SetGasPrice(price)
			log.Debug().Float64("gasPrice", lib.GetGasPrice()).Msg("Updating fee price routine: updating value.")
			time.Sleep(time.Duration(suite.RPCManager.GetCurrentNode().Wallet.GasPriceUpdateInterval) * time.Second)
		}
	}
}

// Spawns the actor processes and any associated non-essential routines
func (suite *UseCaseSuite) Spawn(ctx context.Context) {
	if suite.RPCManager.GetCurrentNode().Wallet.GasPrices == lib.AutoGasPrices {
		log.Info().Msg("auto gas prices. Updating fee price routine: starting.")
		price, err := RunWithNodeRetry(
			ctx,
			suite.RPCManager,
			func(node *lib.NodeConfig) (float64, error) {
				return node.GetBaseFee(ctx)
			},
			"get base fee",
		)
		if err != nil {
			log.Error().Err(err).Msg("Error updating gas prices in auto mode - RPC availability issue?")
			return
		}
		lib.SetGasPrice(price)
		// After intialization, start auto-update routine
		go suite.UpdateGasPriceRoutine(ctx)
	} else {
		price, err := strconv.ParseFloat(suite.RPCManager.GetCurrentNode().Wallet.GasPrices, 64)
		if err != nil {
			log.Error().Err(err).Msg("Invalid gas prices format")
			return
		} else {
			log.Debug().Float64("gasPrice", price).Msg("Setting gas prices manually")
			lib.SetGasPrice(price)
		}
	}

	var wg sync.WaitGroup
	essentialDone := make(chan struct{}) // Channel for essential routines to signal when they are done

	// Run worker process per topic
	alreadyStartedWorkerForTopic := make(map[emissionstypes.TopicId]bool)
	for _, worker := range suite.RPCManager.GetCurrentNode().Worker {
		if _, ok := alreadyStartedWorkerForTopic[worker.TopicId]; ok {
			log.Debug().Uint64("topicId", worker.TopicId).Msg("Worker already started for topicId")
			continue
		}
		alreadyStartedWorkerForTopic[worker.TopicId] = true

		wg.Add(1)
		go func(worker lib.WorkerConfig) {
			defer wg.Done()
			suite.runWorkerProcess(ctx, worker)
			log.Error().Uint64("topicId", worker.TopicId).Msg("Worker process finished")
		}(worker)
	}

	// Run reputer process per topic
	alreadyStartedReputerForTopic := make(map[emissionstypes.TopicId]bool)
	for _, reputer := range suite.RPCManager.GetCurrentNode().Reputer {
		if _, ok := alreadyStartedReputerForTopic[reputer.TopicId]; ok {
			log.Debug().Uint64("topicId", reputer.TopicId).Msg("Reputer already started for topicId")
			continue
		}
		alreadyStartedReputerForTopic[reputer.TopicId] = true

		wg.Add(1)
		go func(reputer lib.ReputerConfig) {
			defer wg.Done()
			suite.runReputerProcess(ctx, reputer)
			log.Error().Uint64("topicId", reputer.TopicId).Msg("Reputer process finished")
		}(reputer)
	}

	// Wait for all essential routines to finish
	go func() {
		wg.Wait()
		close(essentialDone)
	}()

	<-essentialDone // Block until all essential routines are done
}

// Attempts to build and commit a worker payload for a given nonce
// Returns the nonce height acted upon (the received one or the new one if any)
func (suite *UseCaseSuite) processWorkerPayload(ctx context.Context, worker lib.WorkerConfig, latestNonceHeightActedUpon int64, timeoutHeight uint64) (int64, error) {
	latestOpenWorkerNonce, err := RunWithNodeRetry(
		ctx,
		suite.RPCManager,
		func(node *lib.NodeConfig) (*emissionstypes.Nonce, error) {
			return node.GetLatestOpenWorkerNonceByTopicId(ctx, worker.TopicId)
		},
		"get latest open worker nonce",
	)
	if err != nil {
		log.Warn().Err(err).Uint64("topicId", worker.TopicId).Msg("Error getting latest open worker nonce on topic - node availability issue?")
		return latestNonceHeightActedUpon, err
	}

	if latestOpenWorkerNonce.BlockHeight > latestNonceHeightActedUpon {
		log.Debug().Uint64("topicId", worker.TopicId).Int64("BlockHeight", latestOpenWorkerNonce.BlockHeight).
			Msg("Building and committing worker payload for topic")
		err = suite.BuildCommitWorkerPayload(ctx, worker, latestOpenWorkerNonce, timeoutHeight)
		if err != nil {
			return latestNonceHeightActedUpon, errorsmod.Wrapf(err, "error building and committing worker payload for topic")
		}
		log.Debug().Uint64("topicId", worker.TopicId).
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

func (suite *UseCaseSuite) processReputerPayload(ctx context.Context, reputer lib.ReputerConfig, latestNonceHeightActedUpon int64, timeoutHeight uint64) (int64, error) {
	nonce, err := RunWithNodeRetry(
		ctx,
		suite.RPCManager,
		func(node *lib.NodeConfig) (*emissionstypes.Nonce, error) {
			return node.GetOldestReputerNonceByTopicId(ctx, reputer.TopicId)
		},
		"get oldest reputer nonce",
	)
	if err != nil {
		log.Warn().Err(err).Uint64("topicId", reputer.TopicId).Msg("Error getting latest open reputer nonce on topic - node availability issue?")
		return latestNonceHeightActedUpon, err
	}

	if nonce.BlockHeight > latestNonceHeightActedUpon {
		// Check if reputer can submit
		isWhitelisted, err := RunWithNodeRetry(
			ctx,
			suite.RPCManager,
			func(node *lib.NodeConfig) (bool, error) {
				return node.CanSubmitReputer(ctx, reputer.TopicId, node.Wallet.Address)
			},
			"check reputer whitelist",
		)
		if err != nil {
			log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Failed to check if reputer is whitelisted")
			return latestNonceHeightActedUpon, err
		}
		if !isWhitelisted {
			log.Error().Uint64("topicId", reputer.TopicId).Msg("Reputer is not whitelisted in topic, not submitting payload")
			return nonce.BlockHeight, nil
		}

		log.Debug().Uint64("topicId", reputer.TopicId).Int64("BlockHeight", nonce.BlockHeight).
			Msg("Building and committing reputer payload for topic")
		err = suite.BuildCommitReputerPayload(ctx, reputer, nonce.BlockHeight, timeoutHeight)
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
func generateRandomOffset(submissionWindow int64) int64 {
	// Ensure the random number generator is seeded
	source := rand.NewSource(uint64(time.Now().UnixNano())) // nolint: gosec
	rng := rand.New(source)

	// Calculate the center of the window
	center := submissionWindow / 2

	// Generate a random number between start and window center
	offset := rng.Int63n(center + 1)

	return offset
}

// Runs the worker process for a given worker config
func (suite *UseCaseSuite) runWorkerProcess(ctx context.Context, worker lib.WorkerConfig) {
	log.Info().Uint64("topicId", worker.TopicId).Msg("Running worker process for topic")

	// Handle registration
	registered, err := RunWithNodeRetry(
		ctx,
		suite.RPCManager,
		func(node *lib.NodeConfig) (bool, error) {
			return node.RegisterWorkerIdempotently(ctx, worker)
		},
		"RegisterWorkerIdempotently",
	)
	if err != nil {
		log.Fatal().Err(err).Uint64("topicId", worker.TopicId).Msg("Failed to register worker for topic, exiting")
		return
	}

	if !registered {
		log.Fatal().Uint64("topicId", worker.TopicId).Msg("Failed to register worker for topic, exiting")
		return
	}
	log.Debug().Uint64("topicId", worker.TopicId).Msg("Worker registered")

	// Using the helper function
	topicInfo, err := queryTopicInfo(ctx, suite, worker)
	if err != nil {
		log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Failed to get topic info for worker")
		return
	}

	getNonce := func(ctx context.Context, topicId emissionstypes.TopicId) (*emissionstypes.Nonce, error) {
		return RunWithNodeRetry(
			ctx,
			suite.RPCManager,
			func(node *lib.NodeConfig) (*emissionstypes.Nonce, error) {
				return node.GetLatestOpenWorkerNonceByTopicId(ctx, topicId)
			},
			"get latest open worker nonce",
		)
	}
	params := ActorProcessParams[lib.WorkerConfig]{
		Config:                 worker,
		ProcessPayload:         suite.processWorkerPayload,
		GetNonce:               getNonce,
		NearWindowLength:       topicInfo.WorkerSubmissionWindow, // Use worker window to determine "nearness"
		SubmissionWindowLength: topicInfo.WorkerSubmissionWindow, // Use worker window for actual submission window
		ActorType:              "worker",
	}

	// Check if worker is isWhitelisted
	isWhitelisted, err := RunWithNodeRetry(
		ctx,
		suite.RPCManager,
		func(node *lib.NodeConfig) (bool, error) {
			return node.CanSubmitWorker(ctx, worker.TopicId, node.Wallet.Address)
		},
		"check worker whitelist",
	)
	if err != nil {
		log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Failed to check if worker is whitelisted")
		return
	}
	if !isWhitelisted {
		log.Error().Uint64("topicId", worker.TopicId).Msg("Worker is not whitelisted in topic, exiting worker process")
		return
	}

	// Run the actor process
	runActorProcess(ctx, suite, params)
}

// Runs the reputer process for a given reputer config
func (suite *UseCaseSuite) runReputerProcess(ctx context.Context, reputer lib.ReputerConfig) {
	log.Debug().Uint64("topicId", reputer.TopicId).Msg("Running reputer process for topic")

	// Handle registration and staking
	registeredAndStaked, err := RunWithNodeRetry(
		ctx,
		suite.RPCManager,
		func(node *lib.NodeConfig) (bool, error) {
			return node.RegisterAndStakeReputerIdempotently(ctx, reputer)
		},
		"RegisterAndStakeReputerIdempotently",
	)
	if err != nil {
		log.Fatal().Uint64("topicId", reputer.TopicId).Msg("Failed to register or sufficiently stake reputer for topic")
		return
	}
	if !registeredAndStaked {
		log.Fatal().Uint64("topicId", reputer.TopicId).Msg("Failed to register or sufficiently stake reputer for topic")
		return
	}
	log.Debug().Uint64("topicId", reputer.TopicId).Msg("Reputer registered and staked")

	// Using the helper function
	topicInfo, err := queryTopicInfo(ctx, suite, reputer)
	if err != nil {
		log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Failed to get topic info for reputer")
		return
	}

	getNonce := func(ctx context.Context, topicId emissionstypes.TopicId) (*emissionstypes.Nonce, error) {
		return RunWithNodeRetry(
			ctx,
			suite.RPCManager,
			func(node *lib.NodeConfig) (*emissionstypes.Nonce, error) {
				return node.GetOldestReputerNonceByTopicId(ctx, topicId)
			},
			"get oldest reputer nonce",
		)
	}
	params := ActorProcessParams[lib.ReputerConfig]{
		Config:                 reputer,
		ProcessPayload:         suite.processReputerPayload,
		GetNonce:               getNonce,
		NearWindowLength:       topicInfo.WorkerSubmissionWindow, // Use worker window to determine "nearness"
		SubmissionWindowLength: topicInfo.EpochLength,            // Use epoch length for actual submission window
		ActorType:              "reputer",
	}

	// Check if reputer is isWhitelisted
	isWhitelisted, err := RunWithNodeRetry(
		ctx,
		suite.RPCManager,
		func(node *lib.NodeConfig) (bool, error) {
			return node.CanSubmitReputer(ctx, reputer.TopicId, node.Wallet.Address)
		},
		"check reputer whitelist",
	)
	if err != nil {
		log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Failed to check if reputer is whitelisted")
		return
	}
	if !isWhitelisted {
		log.Error().Uint64("topicId", reputer.TopicId).Msg("Reputer is not whitelisted in topic, exiting reputer process")
		return
	}

	// Run the actor process
	runActorProcess(ctx, suite, params)
}

// Function that runs the actor process for a given topic and actor type
// This mechanism is used to handle the submission of payloads for both workers and reputers,
// using ActorProcessParams to handle the different configurations and functions needed for each actor type
func runActorProcess[T lib.TopicActor](ctx context.Context, suite *UseCaseSuite, params ActorProcessParams[T]) {
	log.Debug().
		Uint64("topicId", params.Config.GetTopicId()).
		Str("actorType", params.ActorType).
		Msg("Running actor process for topic")

	topicInfo, err := queryTopicInfo(ctx, suite, params.Config)
	if err != nil {
		log.Error().
			Err(err).
			Uint64("topicId", params.Config.GetTopicId()).
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
		status, err := suite.RPCManager.GetCurrentNode().Chain.Client.Status(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get status")
			if lib.DoneOrWait(ctx, WAIT_TIME_STATUS_CHECKS) {
				return
			}
			continue
		}
		currentBlockHeight = status.SyncInfo.LatestBlockHeight

		topicInfo, err := queryTopicInfo(ctx, suite, params.Config)
		if err != nil {
			log.Error().
				Err(err).
				Uint64("topicId", params.Config.GetTopicId()).
				Str("actorType", params.ActorType).
				Msg("Error getting topic info")
			return
		}
		log.Trace().
			Int64("currentBlockHeight", currentBlockHeight).
			Int64("EpochLastEnded", topicInfo.EpochLastEnded).
			Int64("EpochLength", epochLength).
			Msg("Info from topic")

		// Special case: new topic
		if topicInfo.EpochLastEnded == 0 {
			log.Debug().Msg("New topic, processing payload")
			// timeoutHeight is one epoch length away
			timeoutHeight := currentBlockHeight + epochLength

			latestNonceHeightSentTxFor, err = params.ProcessPayload(ctx, params.Config, latestNonceHeightSentTxFor, uint64(timeoutHeight)) // nolint: gosec
			if err != nil {
				log.Error().
					Err(err).
					Uint64("topicId", params.Config.GetTopicId()).
					Str("actorType", params.ActorType).
					Msg("Error processing payload - could not complete transaction")
			}
			// Wait for an epochLength with a correction factor, it will self-adjust from there
			waitingTimeInSeconds, err := calculateTimeDistanceInSeconds(
				epochLength,
				suite.RPCManager.GetCurrentNode().Wallet.BlockDurationEstimated,
				NEW_TOPIC_CORRECTION_FACTOR,
			)
			if err != nil {
				log.Error().
					Err(err).
					Uint64("topicId", params.Config.GetTopicId()).
					Str("actorType", params.ActorType).
					Int64("waitingTimeInSeconds", waitingTimeInSeconds).
					Msg("Error calculating time distance to next epoch after sending tx - wait epochLength")
				return
			}
			if lib.DoneOrWait(ctx, waitingTimeInSeconds) {
				return
			}
			continue
		}

		epochLastEnded := topicInfo.EpochLastEnded
		epochEnd := epochLastEnded + epochLength
		timeoutHeight := epochLastEnded + params.SubmissionWindowLength
		log.Trace().
			Int64("epochLastEnded", epochLastEnded).
			Int64("epochEnd", epochEnd).
			Int64("timeoutHeight", timeoutHeight).
			Msg("Epoch info")

		var waitingTimeInSeconds int64

		// Check if block is within the submission window
		if currentBlockHeight-epochLastEnded <= params.SubmissionWindowLength {
			// Within the submission window, attempt to process payload
			latestNonceHeightSentTxFor, err = params.ProcessPayload(ctx, params.Config, latestNonceHeightSentTxFor, uint64(timeoutHeight)) // nolint: gosec
			if err != nil {
				log.Error().
					Err(err).
					Uint64("topicId", params.Config.GetTopicId()).
					Str("actorType", params.ActorType).
					Msg("Error processing payload - could not complete transaction")
			}

			distanceUntilNextEpoch := epochEnd - currentBlockHeight
			if distanceUntilNextEpoch < 0 {
				log.Warn().
					Uint64("topicId", params.Config.GetTopicId()).
					Str("actorType", params.ActorType).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("submissionWindowLength", params.SubmissionWindowLength).
					Msg("Distance until next epoch is less than 0, setting to submissionWindowLength")
				distanceUntilNextEpoch = params.SubmissionWindowLength
			}

			waitingTimeInSeconds, err = calculateTimeDistanceInSeconds(
				distanceUntilNextEpoch,
				suite.RPCManager.GetCurrentNode().Wallet.BlockDurationEstimated,
				suite.RPCManager.GetCurrentNode().Wallet.WindowCorrectionFactor,
			)
			if err != nil {
				log.Error().
					Err(err).
					Uint64("topicId", params.Config.GetTopicId()).
					Str("actorType", params.ActorType).
					Msg("Error calculating time distance to next epoch after sending tx")
				return
			}

			log.Info().
				Uint64("topicId", params.Config.GetTopicId()).
				Str("actorType", params.ActorType).
				Int64("currentBlockHeight", currentBlockHeight).
				Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
				Int64("waitingTimeInSeconds", waitingTimeInSeconds).
				Msg("Waiting until the submission window opens after sending")
		} else if currentBlockHeight > epochEnd {
			// Inconsistent topic data, wait until the next epoch
			waitingTimeInSeconds, err = calculateTimeDistanceInSeconds(
				epochLength,
				suite.RPCManager.GetCurrentNode().Wallet.BlockDurationEstimated,
				NEARNESS_CORRECTION_FACTOR,
			)
			if err != nil {
				log.Error().
					Err(err).
					Uint64("topicId", params.Config.GetTopicId()).
					Str("actorType", params.ActorType).
					Msg("Error calculating time distance to next epoch after sending tx")
				return
			}
			log.Warn().
				Uint64("topicId", params.Config.GetTopicId()).
				Str("actorType", params.ActorType).
				Int64("waitingTimeInSeconds", waitingTimeInSeconds).
				Int64("currentBlockHeight", currentBlockHeight).
				Int64("epochEnd", epochEnd).
				Msg("Current block height is greater than next epoch length, is topic inactive? Waiting seconds...")
		} else {
			distanceUntilNextEpoch := epochEnd - currentBlockHeight
			if distanceUntilNextEpoch <= minBlocksToCheck {
				// Close distance, check more closely until the submission window opens
				// Introduces a random offset to avoid thundering herd problem
				offset := generateRandomOffset(params.SubmissionWindowLength)
				closeBlockDistance := distanceUntilNextEpoch + offset
				waitingTimeInSeconds, err = calculateTimeDistanceInSeconds(
					closeBlockDistance,
					suite.RPCManager.GetCurrentNode().Wallet.BlockDurationEstimated,
					NEARNESS_CORRECTION_FACTOR,
				)
				if err != nil {
					log.Error().
						Err(err).
						Uint64("topicId", params.Config.GetTopicId()).
						Str("actorType", params.ActorType).
						Msg("Error calculating close distance to epochLength")
					return
				}
				log.Info().
					Uint64("topicId", params.Config.GetTopicId()).
					Str("actorType", params.ActorType).
					Int64("SubmissionWindowLength", params.SubmissionWindowLength).
					Int64("offset", offset).
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("closeBlockDistance", closeBlockDistance).
					Int64("waitingTimeInSeconds", waitingTimeInSeconds).
					Msg("Close to the window, waiting until next submission window")
			} else {
				// Far distance, bigger waits until the submission window opens
				waitingTimeInSeconds, err = calculateTimeDistanceInSeconds(
					distanceUntilNextEpoch,
					suite.RPCManager.GetCurrentNode().Wallet.BlockDurationEstimated,
					suite.RPCManager.GetCurrentNode().Wallet.WindowCorrectionFactor,
				)
				if err != nil {
					log.Error().
						Err(err).
						Uint64("topicId", params.Config.GetTopicId()).
						Str("actorType", params.ActorType).
						Msg("Error calculating far distance to epochLength")
					return
				}
				log.Info().
					Uint64("topicId", params.Config.GetTopicId()).
					Str("actorType", params.ActorType).
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("waitingTimeInSeconds", waitingTimeInSeconds).
					Msg("Waiting until the submission window opens - far distance")
			}
		}
		if lib.DoneOrWait(ctx, waitingTimeInSeconds) {
			return
		}
	}
}

// Queries the topic info for a given actor type and wallet params from suite
// Wrapper over NodeConfig.GetTopicInfo() with generic config type
func queryTopicInfo[T lib.TopicActor](
	ctx context.Context,
	suite *UseCaseSuite,
	config T,
) (*emissionstypes.Topic, error) {
	topicInfo, err := suite.RPCManager.GetCurrentNode().GetTopicInfo(ctx, config.GetTopicId())
	if err != nil {
		return nil, errorsmod.Wrapf(err, "failed to get topic info")
	}
	return topicInfo, nil
}
