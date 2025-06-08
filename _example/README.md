# Example Ponrunner Service with Full Observability Stack

This directory contains an example project showcasing `ponrunner`, a Go library for building robust services. The example demonstrates how to build a simple web service and integrate it with a comprehensive observability stack using Docker Compose. The stack includes Grafana, Loki, Prometheus, Jaeger, and Tempo for logging, metrics, and tracing.

## Features

- A simple Go HTTP service built with `ponrunner` and `chi`.
- Containerized application using Docker.
- Pre-configured observability stack with:
    - **Grafana**: For visualization of logs, metrics, and traces.
    - **Loki**: For log aggregation.
    - **Prometheus**: For metrics collection.
    - **Jaeger & Tempo**: For distributed tracing.
    - **OpenTelemetry Collector**: To receive, process, and export telemetry data.

## Directory Structure

- `cmd/main.go`: The main application source code.
- `configs/`: Configuration files for Grafana, Loki, Prometheus, etc.
- `Dockerfile`: To build the Go application container.
- `docker-compose.yaml`: Defines the multi-container application stack.
- `docker-compose-config.txt`: Environment variables for the Docker Compose setup.
- `go.mod`, `go.sum`: Go module files.

## Prerequisites

- [Docker](https://www.docker.com/get-started)
- [Docker Compose](https://docs.docker.com/compose/install/)

## Getting Started

1.  **Prepare Environment Variables**:
    Rename the `docker-compose-config.txt` file to `.env`. This file contains the environment variables used by the `docker-compose.yaml` file.

    ```bash
    mv docker-compose-config.txt .env
    ```

2.  **Build and Run the Services**:
    Use Docker Compose to build and start all the services in detached mode.

    ```bash
    docker-compose up --build -d
    ```

3.  **Check Service Status**:
    You can check the status of the running containers:

    ```bash
    docker-compose ps
    ```

## Accessing the Services

Once the stack is up and running, you can access the various components:

-   **Example Service**:
    -   API Endpoint: [http://localhost:8080/hello](http://localhost:8080/hello)
    -   This should return a JSON response like `{"message":"Hello from Ponrunner! Port: 8080"}`.

-   **Grafana**:
    -   URL: [http://localhost:3000](http://localhost:3000)
    -   Login: `admin` / `test` (as defined in `.env` and `docker-compose.yaml`).
    -   Data sources for Loki, Prometheus, Jaeger, and Tempo are pre-configured. You can explore logs, metrics, and traces for the `example-service`.

-   **Prometheus**:
    -   URL: [http://localhost:9090](http://localhost:9090)
    -   You can query metrics directly from the Prometheus UI. The OpenTelemetry Collector is configured as a scrape target.

-   **Jaeger**:
    -   URL: [http://localhost:16686](http://localhost:16686)
    -   You can view traces sent from the `example-service`.

## Observability in Action

-   **Logs**: The `example-service` is configured to send logs in JSON format. These logs are collected by the OpenTelemetry Collector and forwarded to Loki. You can view and query these logs in Grafana's "Explore" tab using the "Loki" data source.

-   **Metrics**: The application exposes metrics which are collected by the OpenTelemetry Collector and scraped by Prometheus. You can visualize these metrics in Grafana by creating dashboards or using the "Explore" tab with the "Prometheus" data source.

-   **Traces**: The service is instrumented for distributed tracing. Traces are sent to the OpenTelemetry Collector, which then exports them to Jaeger and Tempo. You can explore the request traces in the Jaeger UI or within Grafana using the "Jaeger" or "Tempo" data sources.

## Stopping the Stack

To stop and remove all the containers, networks, and volumes, run:

```bash
docker-compose down
```
