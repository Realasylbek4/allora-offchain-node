Allora Off-Chain Node
Allora off-chain nodes enable publishing inferences, forecasts, and losses informed by configurable ground truth data and loss functions, interfacing directly with the Allora blockchain.

Table of Contents
Overview
Running with Docker
Running without Docker
Prometheus Metrics
Node Behavior and Submission Cycle
Configuration Options
Examples
License
Overview
Allora off-chain nodes perform the following tasks:

Publish inferences, forecasts, and loss calculations.
Use configurable ground truth data and loss functions.
Enable seamless interaction with the Allora blockchain via environment-driven configurations.
Running with Docker
Prerequisites
Ensure you have the following installed:

Docker: Installation Guide
Steps
Clone the repository:
Steps
Clone the repository:

bash
git clone <repository-url>
cd allora-offchain-node
Remove .env file:
If .env exists in the root directory, remove it to avoid conflicts with the automated environment variable setup.

Configure the node: Copy and populate the config.example.json file with your parameters.

Use your existing wallet, or leave the wallet field empty to auto-generate one.
bash
cp config.example.json config.json
Initialize the environment: Run the following commands to load the configuration:

bash
chmod +x init.config
./init.config
This script:

Loads config.json into the environment.
Creates Allora keys if wallet information is not provided.
Exports all necessary variables for the node to function.
Start the services: Use Docker Compose to build and start the services:

bash
docker compose up --build
This will:

Run the off-chain node and source services.
Enable inter-service communication through internal DNS.
Notes
Security: Ensure you remove sensitive secrets from config.json before committing to a public repository.
Tokens: If keys are auto-generated, request tokens from the faucet to register your worker and stake reputation.
Running without Docker
Prerequisites
Install Go 1.22.5: Installation Guide
Install dependencies:
bash
go mod download
Steps
Clone the repository:

bash
git clone <repository-url>
cd allora-offchain-node
Set up environment variables: Copy the example .env file and populate it with your details:

bash
cp .env.example .env
Role Configuration:

Workers: Configure your inference and/or forecast models in the adapters/ directory.
Reputers: Configure repute and loss models in the adapters/ directory.
Map topics to adapters in config.json.
Start the node:

bash
chmod +x start.local
./start.local
Prometheus Metrics
The node exposes metrics on port :2112/metrics. Available metrics include:

allora_worker_inference_request_count: Total worker inference requests.
allora_worker_forecast_request_count: Total worker forecast requests.
allora_reputer_truth_request_count: Total reputer truth requests.
allora_worker_data_build_count: Successful worker data builds.
allora_reputer_data_build_count: Successful reputer data builds.
allora_worker_chain_submission_count: Worker chain submissions.
allora_reputer_chain_submission_count: Reputer chain submissions.
Additional metrics may be added in future updates.

Node Behavior and Submission Cycle
Epoch and Submission Windows
Example configuration:

Epoch length: 100 blocks
Submission window: 10 blocks
Near zone: Last 20 blocks of an epoch
Timeline Overview:
sql
Epoch N Start       Submission Window Open       Near Zone               Epoch N+1 Start
|-------|----------------------------|------------------------|------------------------>
Block:   1000                        1010                    1080                     1100
Random Offset
To prevent mempool congestion, the node introduces randomized offsets to submission times.

Zones
Far Zone: Infrequent checks, optimized for efficiency.
Near Zone: Frequent checks, ensuring no missed submissions.
Configuration Options
Configuration Methods
Set the ALLORA_OFFCHAIN_NODE_CONFIG_JSON environment variable with a JSON string.
Set the ALLORA_OFFCHAIN_NODE_CONFIG_FILE_PATH to a configuration file path.
Note: Each method overrides the other.

Logging Configuration
LOG_LEVEL: Set the logging level (debug, info, warn, error, etc.). Default: info.
LOG_TIME_FORMAT: Set timestamp format (iso8601, unix, etc.). Default: iso8601.
Smart Window Detection
blockDurationEstimated: Estimated block time in seconds. Minimum: 1.
windowCorrectionFactor: Fine-tunes submission window detection. Minimum: 0.5.
Examples
Worker as an Inferer
json
{
  "worker": [
    {
      "topicId": 1,
      "inferenceEntrypointName": "apiAdapter",
      "parameters": {
        "InferenceEndpoint": "http://source:8000/inference/{Token}",
        "Token": "ETH"
      }
    }
  ]
}
Reputer Configuration
json
{
  "reputer": [
    {
      "topicId": 1,
      "groundTruthEntrypointName": "apiAdapter",
      "lossFunctionEntrypointName": "apiAdapter",
      "minStake": 100000,
      "groundTruthParameters": {
        "GroundTruthEndpoint": "http://localhost:8888/gt/{Token}/{BlockHeight}",
        "Token": "ETHUSD"
      },
      "lossFunctionParameters": {
        "LossFunctionService": "http://localhost:5000",
        "LossMethodOptions": {
          "loss_method": "sqe"
        }
      }
    }
  ]
}
License
This project is licensed under the Apache 2.0 License. See the LICENSE file for details.
