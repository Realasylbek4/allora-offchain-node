package lib

import (
	"errors"
	"fmt"

	emissions "github.com/allora-network/allora-chain/x/emissions/types"
	bank "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
)

const (
	WindowCorrectionFactorSuggestedMin = 0.5
	BlockDurationEstimatedMin          = 1.0
	RetryDelayMin                      = 1
	AccountSequenceRetryDelayMin       = 1
)

// Properties manually provided by the user as part of UserConfig
type WalletConfig struct {
	Address                   string // will be overwritten by the keystore. This is the 1 value that is auto-generated in this struct
	AddressKeyName            string // load a address by key from the keystore
	AddressRestoreMnemonic    string
	AlloraHomeDir             string  // home directory for the allora keystore
	Gas                       string  // gas to use for the allora client
	GasAdjustment             float64 // gas adjustment to use for the allora client
	GasPrices                 float64 // gas prices to use for the allora client - 0 for no fees
	MaxFees                   uint64  // max gas to use for the allora client
	NodeRpc                   string  // rpc node for allora chain
	MaxRetries                int64   // retry to get data from chain up to this many times per query or tx
	RetryDelay                int64   // number of seconds to wait between retries (general case)
	AccountSequenceRetryDelay int64   // number of seconds to wait between retries in case of account sequence error
	SubmitTx                  bool    // useful for dev/testing. set to false to run in dry-run processes without committing to the chain
	BlockDurationEstimated    float64 // estimated average block duration in seconds
	WindowCorrectionFactor    float64 // correction factor for the time estimation, suggested range 0.7-0.9.
}

// Properties auto-generated based on what the user has provided in WalletConfig fields of UserConfig
type ChainConfig struct {
	Address              string // will be auto-generated based on the keystore
	Account              cosmosaccount.Account
	Client               *cosmosclient.Client
	EmissionsQueryClient emissions.QueryServiceClient
	BankQueryClient      bank.QueryClient
	DefaultBondDenom     string
	AddressPrefix        string // prefix for the allora addresses
}

type WorkerConfig struct {
	TopicId                 emissions.TopicId
	InferenceEntrypointName string
	InferenceEntrypoint     AlloraAdapter
	ForecastEntrypointName  string
	ForecastEntrypoint      AlloraAdapter
	LoopSeconds             int64             // seconds to wait between attempts to get next worker nonce
	Parameters              map[string]string // Map for variable configuration values
}

type ReputerConfig struct {
	TopicId                    emissions.TopicId
	GroundTruthEntrypointName  string
	GroundTruthEntrypoint      AlloraAdapter
	LossFunctionEntrypointName string
	LossFunctionEntrypoint     AlloraAdapter
	// Minimum stake to repute. will try to add stake from wallet if current stake is less than this.
	// Will not repute if current stake is less than this, after trying to add any necessary stake.
	// This is idempotent in that it will not add more stake than specified here.
	// Set to 0 to effectively disable this feature and use whatever stake has already been added.
	MinStake               int64
	LoopSeconds            int64                  // seconds to wait between attempts to get next reptuer nonces
	GroundTruthParameters  map[string]string      // Map for variable configuration values
	LossFunctionParameters LossFunctionParameters // Map for variable configuration values
}

type LossFunctionParameters struct {
	LossFunctionService string
	LossMethodOptions   map[string]string
	IsNeverNegative     *bool // Cached result of whether the loss function is never negative
}

type UserConfig struct {
	Wallet  WalletConfig
	Worker  []WorkerConfig
	Reputer []ReputerConfig
}

type NodeConfig struct {
	Chain   ChainConfig
	Wallet  WalletConfig
	Worker  []WorkerConfig
	Reputer []ReputerConfig
}

type WorkerResponse struct {
	WorkerConfig
	InfererValue     string      `json:"infererValue,omitempty"`
	ForecasterValues []NodeValue `json:"forecasterValue,omitempty"`
}

type SignedWorkerResponse struct {
	*emissions.WorkerDataBundle
	BlockHeight int64 `json:"blockHeight,omitempty"`
	TopicId     int64 `json:"topicId,omitempty"`
}

type ValueBundle struct {
	CombinedValue          string      `json:"combinedValue,omitempty"`
	NaiveValue             string      `json:"naiveValue,omitempty"`
	InfererValues          []NodeValue `json:"infererValues,omitempty"`
	ForecasterValues       []NodeValue `json:"forecasterValues,omitempty"`
	OneOutInfererValues    []NodeValue `json:"oneOutInfererValues,omitempty"`
	OneOutForecasterValues []NodeValue `json:"oneOutForecasterValues,omitempty"`
	OneInForecasterValues  []NodeValue `json:"oneInForecasterValues,omitempty"`
}

// Check that each assigned entrypoint in the user config actually can be used
// for the intended purpose, else throw error
func (c *UserConfig) ValidateConfigAdapters() error {
	// Validate wallet config
	err := c.ValidateWalletConfig()
	if err != nil {
		return err
	}
	// Validate worker configs
	for _, workerConfig := range c.Worker {
		err := workerConfig.ValidateWorkerConfig()
		if err != nil {
			return err
		}
	}
	// Validate reputer configs
	for _, reputerConfig := range c.Reputer {
		err := reputerConfig.ValidateReputerConfig()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *UserConfig) ValidateWalletConfig() error {
	if c.Wallet.WindowCorrectionFactor < WindowCorrectionFactorSuggestedMin {
		return errors.New(fmt.Sprintf("window correction factor lower than suggested minimum: %f < %f", c.Wallet.WindowCorrectionFactor, WindowCorrectionFactorSuggestedMin))
	}
	if c.Wallet.BlockDurationEstimated < BlockDurationEstimatedMin {
		return errors.New(fmt.Sprintf("block duration estimated lower than the minimum: %f < %f", c.Wallet.BlockDurationEstimated, BlockDurationEstimatedMin))
	}
	if c.Wallet.RetryDelay < RetryDelayMin {
		return errors.New(fmt.Sprintf("retry delay lower than the minimum: %f < %f", c.Wallet.RetryDelay, RetryDelayMin))
	}
	if c.Wallet.AccountSequenceRetryDelay < AccountSequenceRetryDelayMin {
		return errors.New(fmt.Sprintf("account sequence retry delay lower than the minimum: %f < %f", c.Wallet.AccountSequenceRetryDelay, AccountSequenceRetryDelayMin))
	}

	return nil
}

func (reputerConfig *ReputerConfig) ValidateReputerConfig() error {
	if reputerConfig.GroundTruthEntrypoint != nil && !reputerConfig.GroundTruthEntrypoint.CanSourceGroundTruthAndComputeLoss() {
		return errors.New("invalid loss entrypoint")
	}
	return nil
}

func (workerConfig *WorkerConfig) ValidateWorkerConfig() error {
	if workerConfig.InferenceEntrypoint != nil && !workerConfig.InferenceEntrypoint.CanInfer() {
		return errors.New("invalid inference entrypoint")
	}
	if workerConfig.ForecastEntrypoint != nil && !workerConfig.ForecastEntrypoint.CanForecast() {
		return errors.New("invalid forecast entrypoint")
	}
	return nil
}
