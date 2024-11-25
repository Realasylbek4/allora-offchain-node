# Offchain API Source (Server)

This is a small Python Flask API that can be run locally or within a Docker container. It's meant to demonstrate what your server will look like generating model inference and forecast to be called by the adapter

## Prerequisites

* Python 3.x
* Flask
* Docker (if you want to run the API in a Docker container)

## Running the API Locally

### Installing Dependencies

Before running the API, you need to install the required dependencies.

```bash
pip install -r requirements.txt
```

To run the Flask API locally, use the following command:

```bash
python main.py
```

This will start the Flask development server on http://127.0.0.1:8000/

## Running the API with Docker

Alternatively, you can run the API using Docker.

```bash
docker build -t offchain-api-source .
```

Run the Docker container

```bash
docker run -p 8000:8000 offchain-api-source
```

This will start the Flask API server on http://localhost:8000/
