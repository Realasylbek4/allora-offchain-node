# allora-offchain-node

Allora off-chain nodes publish inferences, forecasts, and losses informed by a configurable ground truth and applying a configurable loss function to the Allora chain.

## How to run with docker
1. Clone the repository
2. Make sure to remove any .env file so it doesn't clash with the automated environment variables
3. Copy config.example.json and populate with your variables. You can either populate with your existing wallet or leave it empty for it to be autocreated

```shell
cp config.example.json config.json
```
4. Run command below to load your config.json file to environment

```shell
chmod +x init.config
./init.config
```

from the root directory. This will:
   - Load your config.json file into the environment. Depending on whether you provided your wallet details or not it will also do the following:
      - Automatically create allora keys for you. You will have to request for some tokens from faucet to be able to register your worker and stake your reputer. You can find your address in ./data/env_file
      - Automatically export the needed variables from the account created to be used by the offchain node and bundles it with the your provided config.json and then pass them to the node as environemnt variable

5. Run `docker compose up --build`. This will:
   - Run the both the offchain node and the source services, communicating through endpoints attached to the internal dns

Please note that the environment variable will be created as bumdle of your config.json and allora account secrets, please make sure to remove every sectrets before commiting to remote git repository


## How to run without docker

1. Clone the repository
2. Install Go 1.22.5
3. Install the dependencies:

```shell
go mod download
```

4. Copy environment variables:

```shell
cp .env.example .env
```

5. Fill in the environment variables in `.env` with your own values
6. If you're a worker...
   1. Configure your inference and/or forecast models in `adapters/` and/or another source linked to by adapters in `adapters/` directory.
7. If you're a reputer...
   1. Configure your repute and/or loss models in `adapters/` and/or another source linked to by adapters in `adapters/` directory.
8. Map each topic to the appropriate adapter in `config.json`.
9. Run the following commands:

```shell
chmod +x start.local
./start.local
```

## Prometheus Metrics
Some metrics has been provided for in the node. You can access them with port `:2112/metrics`. Here are the following list of existing metrics: 
- `allora_worker_inference_request_count`: The total number of times worker requests inference from source
- `allora_worker_forecast_request_count`: The total number of times worker requests forecast from source
- `allora_reputer_truth_request_count`: The total number of times reputer requests truth from source
- `allora_worker_data_build_count`: The total number of times worker built data successfully
- `allora_reputer_data_build_count`: The total number of times reputer built data successfully
- `allora_worker_chain_submission_count`: The total number of worker commits to the chain
- `allora_reputer_chain_submission_count`: The total number of reputer commits to the chain

> Please note that we will keep updating the list as more metrics are being added

## How to configure

There are several ways to configure the node. In order of preference, you can do any of these: 
* Set the `ALLORA_OFFCHAIN_NODE_CONFIG_JSON` env var with a configuration as a JSON string.
* Set the `ALLORA_OFFCHAIN_NODE_CONFIG_FILE_PATH` env var pointing to a file, which contains configuration as JSON. An example if provided in `config.example.json`.

Each option completely overwrites the other options.

This is the entrypoint for the application that simply builds and runs the Go program.

It spins off a distinct processes per role worker, reputer per topic configered in `config.json`.

## Logging env vars

* LOG_LEVEL: Set the logging level. Valid values are `debug`, `info`, `warn`, `error`, `fatal`, `panic`. Defaults to `info`.
* LOG_TIME_FORMAT: Sets the format of the timestamp in the log. Valid values are `unix`, `unixms`, `unixmicro`, `iso8601`. Defaults to `iso8601`.

## Wallet configuration

The wallet configuration is done in `config.json` under the `wallet` field.

### Gas and Fees

- `gas` can be set to `auto` or a specific gas value. If set to `auto`, the node will automatically calculate the gas limit based on the estimated gas used by the transactions.
- `gasAdjustment` is used to adjust the gas limit.
- `gasPrices` and `maxFees` fields are used to set the gas prices and max fees for the wallet. They are expressed in `uallo`.

### Error handling

Error handling is done differently for different types of errors.

#### Retries 
- `accounSequenceRetryDelay`: For the "account sequence mismatch" error. 
- `retryDelay`: For all other errors that need retry delays.




## Configuration examples

A complete example is provided in `config.example.json`. 
These below are excerpts of the configuration (with some parts omitted for brevity) for different setups:

### 1 workers as inferer 

```json
{
   "worker": [
      {
        "topicId": 1,
        "inferenceEntrypointName": "api-worker-reputer",
        "loopSeconds": 10,
        "parameters": {
          "InferenceEndpoint": "http://source:8000/inference/{Token}",
          "Token": "ETH"
        }
      }
   ]
}
```

###  1 worker as forecaster
```json
{
   "worker": [
      {
        "topicId": 1,
        "forecastEntrypointName": "api-worker-reputer",
        "loopSeconds": 10,
        "parameters": {
          "ForecastEndpoint": "http://source:8000/forecasts/{TopicId}/{BlockHeight}"
        }
      }
   ]
}

```

###  1 worker as inferer and forecaster

```json
{
   "worker": [
      {
        "topicId": 1,
        "inferenceEntrypointName": "api-worker-reputer",
        "forecastEntrypointName": "api-worker-reputer",
        "loopSeconds": 10,
        "parameters": {
          "InferenceEndpoint": "http://source:8000/inference/{Token}",
          "ForecastEndpoint": "http://source:8000/forecasts/{TopicId}/{BlockHeight}",
          "Token": "ETH"
        }
      }
   ]
}
```

### 1 reputer

```json
{
"reputer": [
      {
        "topicId": 1,
        "groundTruthEntrypointName": "api-worker-reputer",
        "lossFunctionEntrypointName": "api-worker-reputer",
        "loopSeconds": 30,
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
```

### 1 worker as inferer and forecaster, and 1 reputer

```json
{
"worker": [
      {
        "topicId": 1,
        "inferenceEntrypointName": "api-worker-reputer",
        "forecastEntrypointName": "api-worker-reputer",
        "loopSeconds": 10,
        "parameters": {
          "InferenceEndpoint": "http://source:8000/inference/{Token}",
          "ForecastEndpoint": "http://source:8000/forecasts/{TopicId}/{BlockHeight}",
          "Token": "ETH"
        }
      }
    ],
"reputer": [
      {
        "topicId": 1,
        "groundTruthEntrypointName": "api-worker-reputer",
        "lossFunctionEntrypointName": "api-worker-reputer",
        "loopSeconds": 30,
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
```

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.
