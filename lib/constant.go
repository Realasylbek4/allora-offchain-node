package lib

const SECONDS_PER_BLOCK = 5   // each block is this many seconds
const ADDRESS_PREFIX = "allo" // each address prefixed by this
const DEFAULT_BOND_DENOM = "uallo"
const ALLORA_OFFCHAIN_NODE_CONFIG_JSON = "ALLORA_OFFCHAIN_NODE_CONFIG_JSON"
const ALLORA_OFFCHAIN_NODE_CONFIG_FILE_PATH = "ALLORA_OFFCHAIN_NODE_CONFIG_FILE_PATH"

const (
	InferenceRequestCount       string = "allora_worker_inference_request_count"
	ForecastRequestCount        string = "allora_worker_forecast_request_count"
	TruthRequestCount           string = "allora_reputer_truth_request_count"
	WorkerDataBuildCount        string = "allora_worker_data_build_count"
	ReputerDataBuildCount       string = "allora_reputer_data_build_count"
	WorkerChainSubmissionCount  string = "allora_worker_chain_submission_count"
	ReputerChainSubmissionCount string = "allora_reputer_chain_submission_count"
)

// A struct that holds the name and help text for a prometheus counter
var CounterData = []MetricsCounter{
	{InferenceRequestCount, "The total number of times worker requests inference from source"},
	{ForecastRequestCount, "The total number of times worker requests forecast from source"},
	{TruthRequestCount, "The total number of times reputer requests truth from source"},
	{WorkerDataBuildCount, "The total number of times worker built data successfully"},
	{ReputerDataBuildCount, "The total number of times worker built data successfully"},
	{WorkerChainSubmissionCount, "The total number of worker commits to the chain"},
	{ReputerChainSubmissionCount, "The total number of reputer commits to the chain"},
}
