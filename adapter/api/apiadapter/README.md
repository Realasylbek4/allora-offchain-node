# Allora Offchain API Adapter

This repository contains the adapter module for the Allora Offchain Node. The adapter is responsible for connecting the offchain node to external systems via hitting an URL, for inferences, forecasts or source of truth.

It is intended to be used by configuration.

This Adapter is intended to be used to send inferences and/or forecasts and/or source_truth from external services which provide an API endpoint.

## Config

To use and configure this adapter, please use the WorkerConfig object. 
Example as Worker:

```
Worker: []lib.WorkerConfig{
    TopicId:             1,
    InferenceEntrypoint: apiAdapter.NewAlloraAdapter(),
    ForecastEntrypoint:  apiAdapter.NewAlloraAdapter(),
    LoopSeconds:         5,
    Parameters: map[string]string{
        "Token":             "ETH",
        "InferenceEndpoint": "http://localhost:8000/inference/{Token}",
        "ForecastEndpoint":  "http://localhost:8000/forecast/{TopicId}/{BlockHeight}",
    },
},
```

Example as Reputer ("gt" in this context means "ground truth"): 
```
Reputer: []lib.ReputerConfig{
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
            "loss_method": "huber",
            "delta": "1.0"
          }
        }
    }
},
```

## Parameters

The parameters section contains additional properties the user wants to use to configure their URLs to hit.
In the case of the reputer, there are two parameters sections, one for the ground truth and one for the loss function.
In particular, the `LossMethodOptions` are specific to the loss function and passed unconverted to the loss function service.
They can be used to pass additional parameters to the loss function service. For example, the `delta` parameter is passed to the huber loss function like this (or as per defined in the loss function service of choice): 

```
"LossMethodOptions": {
    "loss_method": "huber",
    "delta": "1.0"
}
```


### Worker

`InferenceEndpoint` is required if `InferenceEntrypoint` is defined.
`ForecastEndpoint` is required if `ForecastEntrypoint` is defined.

`InferenceEndpoint`: provides the inference endpoint to hit. It supports URL template variables.
`ForecastEndpoint`: provides the forecast endpoint to hit. It supports URL template variables.

If it is not desired to send inferences or forecasts, it can be configured by setting that specific entrypoint to nil. Example, for not sending inferences:
```
InferenceEntrypoint: nil
```

### Reputer

Two endpoints are required:
* `GroundTruthEndpoint`: provides the ground truth endpoint to hit. It does support template variables.
* `LossFunctionService`: provides the loss function service to hit on loss calculation and the endpoint to know whether the loss function is never negative. These are appended to create `/calculate` and `/is_never_negative` endpoints respectively. They do not support template variables.


### Additional Parameters 

Any additional parameter can be defined freely, like `Token` in the example, and be used in the endpoint templates.
Additional parameters do not support template variables.


## Template variables

The URLs support template variables as defined from the Parameters section. 

In addition, it supports two special variables: 
* TopicId: as defined in WorkerConfig object
* BlockHeight: the blockheight at which the operation happens


## Usage

* Set up your inference and/or forecast models and serve results via an API. 
* Add a Worker configuration like the above in your config.go, configuring your endpoints appropriately.
* Configure the rest of the Allora Offchain Node (e.g. wallet)
* Run the Allora Offchain Node

