# /home/ab/ws-pooler/tests/README.md
# Testing Strategies for WebSocket Pooler

This directory contains scripts and code for testing the WebSocket Pooler application. The tests are divided into several categories as described below.

## Prerequisites

Before running these tests, ensure you have the following installed:
- Go (version 1.23+)
- Docker and Docker Compose
- [k6](httpss://k6.io/docs/getting-started/installation/) for load and chaos testing.

## 1. Unit Tests

Unit tests verify individual components in isolation. They are located alongside the code they test (e.g., `auth_test.go` in the `websocket` package).

**How to Run:**
From the root directory of the project (`/home/ab/ws-pooler`), run:
```sh
go test ./...
