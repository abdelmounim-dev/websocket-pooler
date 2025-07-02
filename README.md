# WebSocket Pooler

This project provides a scalable, high-performance WebSocket connection pooler written in Go. It is designed to manage a large number of concurrent WebSocket connections for real-time applications. The pooler is built with a flexible architecture that allows for either Redis Pub/Sub or Kafka to be used as a message broker for backend communication.

## Features

- **High Concurrency:** Built with Go's concurrency model to handle thousands of simultaneous WebSocket connections.
- **Scalable Architecture:** Designed to be horizontally scalable. Multiple instances of the pooler can be run behind a load balancer.
- **Flexible Message Broker:** Supports both Redis Pub/Sub and Kafka as a message broker, configurable via environment variables.
- **Session Management:** Uses Redis for persistent session storage, allowing for connection recovery and stateful applications.
- **Authentication:** Includes optional JWT-based authentication for securing WebSocket connections.
- **Monitoring:** Integrated with Prometheus for metrics collection and Grafana for visualization with a pre-built dashboard.
- **Containerized:** Fully containerized with Docker and easily deployable with Docker Compose.

## Architecture

The system consists of several services working together:

- **Pooler:** The core WebSocket server that manages client connections.
- **Backend:** A sample backend service that can produce messages to be sent to clients.
- **Redis:** Used for session storage and can also be used as the message broker.
- **Kafka & Zookeeper:** Can be used as a more robust, high-throughput message broker.
- **Prometheus:** Scrapes metrics from the pooler instances.
- **Grafana:** Visualizes the metrics from Prometheus.

A client connects to a `pooler` instance. The `pooler` handles the connection and authentication. When the backend needs to send a message to a specific client, it publishes the message to the broker (Redis or Kafka). The `pooler` instances are subscribed to the broker and will forward the message to the correct client if the client is connected to that instance.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose](https://docs.docker.com/compose/install/)

## Getting Started

1.  **Clone the repository:**
    ```sh
    git clone https://github.com/abdelmounim-dev/websocket-pooler.git
    cd websocket-pooler
    ```

2.  **Run the application:**
    The project is configured to run with Docker Compose. The default configuration uses Redis as the message broker.

    ```sh
    docker-compose up --build
    ```

    To run in detached mode:
    ```sh
    docker-compose up --build -d
    ```

## Configuration

The primary configuration is managed within the `docker-compose.yml` file, specifically through environment variables for the `pooler` service.

### Broker Configuration

You can switch between Redis and Kafka as the message broker by modifying the `environment` section of the `pooler` service in `docker-compose.yml`.

**Using Redis (Default):**

```yaml
environment:
  WSGATEWAY_BROKER_TYPE: "redis"
  WSGATEWAY_REDIS_ADDRESS: "redis:6379"
```

**Using Kafka:**

To use Kafka, comment out the Redis variables and uncomment the Kafka variables.

```yaml
environment:
  # WSGATEWAY_BROKER_TYPE: "redis"
  # WSGATEWAY_REDIS_ADDRESS: "redis:6379"
  WSGATEWAY_BROKER_TYPE: "kafka"
  WSGATEWAY_KAFKA_BROKERS: "kafka:29092"
  WSGATEWAY_REDIS_ADDRESS: "redis:6379" # Still required for session store
```

Application-level configuration can be adjusted in `config.dev.yaml` and `config.prod.yaml`.

## Services and Ports

The `docker-compose` environment exposes the following services:

- **WebSocket Pooler:** `localhost:8080`
- **Prometheus:** `localhost:9090`
- **Grafana:** `localhost:3000` (Default credentials: `admin`/`admin`)
- **Redis:** `localhost:6379`
- **Kafka:** `localhost:9092`

The Grafana service is pre-configured with a Prometheus data source and a dashboard for monitoring the WebSocket pooler.
