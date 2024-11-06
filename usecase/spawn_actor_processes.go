package usecase

import (
	"allora_offchain_node/lib"
	"sync"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/rs/zerolog/log"
)

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

func (suite *UseCaseSuite) runWorkerProcess(worker lib.WorkerConfig) {
	log.Info().Uint64("topicId", worker.TopicId).Msg("Running worker process for topic")

	registered := suite.Node.RegisterWorkerIdempotently(worker)
	if !registered {
		log.Error().Uint64("topicId", worker.TopicId).Msg("Failed to register worker for topic")
		return
	}

	latestNonceHeightActedUpon := int64(0)
	for {
		latestOpenWorkerNonce, err := suite.Node.GetLatestOpenWorkerNonceByTopicId(worker.TopicId)
		if err != nil {
			log.Warn().Err(err).Uint64("topicId", worker.TopicId).Msg("Error getting latest open worker nonce on topic - node availability issue?")
		} else {
			if latestOpenWorkerNonce.BlockHeight > latestNonceHeightActedUpon {
				log.Debug().Uint64("topicId", worker.TopicId).Int64("BlockHeight", latestOpenWorkerNonce.BlockHeight).Msg("Building and committing worker payload for topic")

				err := suite.BuildCommitWorkerPayload(worker, latestOpenWorkerNonce)
				if err != nil {
					log.Error().Err(err).Uint64("topicId", worker.TopicId).Int64("BlockHeight", latestOpenWorkerNonce.BlockHeight).Msg("Error building and committing worker payload for topic")
				}
				latestNonceHeightActedUpon = latestOpenWorkerNonce.BlockHeight
			} else {
				log.Debug().Uint64("topicId", worker.TopicId).Msg("No new worker nonce found")
			}
		}
		suite.Wait(worker.LoopSeconds)
	}
}

func (suite *UseCaseSuite) runReputerProcess(reputer lib.ReputerConfig) {
	log.Debug().Uint64("topicId", reputer.TopicId).Msg("Running reputer process for topic")

	registeredAndStaked := suite.Node.RegisterAndStakeReputerIdempotently(reputer)
	if !registeredAndStaked {
		log.Error().Uint64("topicId", reputer.TopicId).Msg("Failed to register or sufficiently stake reputer for topic")
		return
	}

	latestNonceHeightActedUpon := int64(0)
	for {
		latestOpenReputerNonce, err := suite.Node.GetOldestReputerNonceByTopicId(reputer.TopicId)
		if err != nil {
			log.Warn().Err(err).Uint64("topicId", reputer.TopicId).Int64("BlockHeight", latestOpenReputerNonce).Msg("Error getting latest open reputer nonce on topic - node availability issue?")
		} else {
			if latestOpenReputerNonce > latestNonceHeightActedUpon {
				log.Debug().Uint64("topicId", reputer.TopicId).Int64("BlockHeight", latestOpenReputerNonce).Msg("Building and committing reputer payload for topic")

				err := suite.BuildCommitReputerPayload(reputer, latestOpenReputerNonce)
				if err != nil {
					log.Error().Err(err).Uint64("topicId", reputer.TopicId).Msg("Error building and committing reputer payload for topic")
				}
				latestNonceHeightActedUpon = latestOpenReputerNonce
			} else {
				log.Debug().Uint64("topicId", reputer.TopicId).Msg("No new reputer nonce found")
			}
		}
		suite.Wait(reputer.LoopSeconds)
	}
}
