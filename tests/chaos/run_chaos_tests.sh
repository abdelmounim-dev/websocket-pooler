#!/usr/bin/env bash

# This script runs a simple chaos experiment against the WebSocket Pooler stack.
# It uses k6 to run a continuous, low-level load test as a "monitor" to
# check for system availability and correctness during the experiment.

# --- Configuration ---
MONITOR_VUS=10
MONITOR_DURATION="60s"
FAULT_DURATION=15 # seconds to keep Redis down

# --- Colors for logging ---
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

function log_info() {
    echo -e "${GREEN}[INFO] $1${NC}"
}

function log_warn() {
    echo -e "${YELLOW}[WARN] $1${NC}"
}

function log_error() {
    echo -e "${RED}[ERROR] $1${NC}"
}

# --- Cleanup function ---
function cleanup() {
    log_info "Cleaning up and shutting down Docker Compose stack..."
    docker compose down -v --remove-orphans
}
trap cleanup EXIT

# --- Main Script ---
log_info "Starting Chaos Test..."

# 1. Build and start the full stack in detached mode
log_info "Starting application stack with docker-compose..."
docker compose up -d --build
if [ $? -ne 0 ]; then
    log_error "Docker Compose failed to start. Aborting."
    exit 1
fi
log_info "Stack is up. Waiting a few seconds for services to stabilize..."
sleep 10

# 2. Run k6 monitor in the background
log_info "Starting k6 monitor in the background (${MONITOR_VUS} VUs for ${MONITOR_DURATION})..."
k6 run --vus $MONITOR_VUS --duration $MONITOR_DURATION tests/load/load_test.js > k6_monitor.log 2>&1 &
K6_PID=$!

# Give the monitor a moment to start running
sleep 5

# 3. Inject Fault: Stop the Redis container
log_warn "--- INJECTING FAULT: Stopping Redis container ---"
docker-compose stop redis
log_warn "Redis is down. The system will run without it for ${FAULT_DURATION} seconds."

# 4. Wait while the fault is active
sleep $FAULT_DURATION

# 5. Restore Service: Start Redis again
log_info "--- RESTORING SERVICE: Starting Redis container ---"
docker-compose start redis
log_info "Redis has been restored. System should recover."

# 6. Wait for the k6 monitor to complete
log_info "Waiting for k6 monitor to finish..."
wait $K6_PID
log_info "k6 monitor finished."

# 7. Analyze Results
log_info "--- CHAOS TEST COMPLETE: Analyzing Results ---"
echo "Displaying the last 20 lines of the k6 log:"
tail -n 20 k6_monitor.log
echo "------------------------------------------------"

# Check for connection errors in the log
if grep -q "ws_connection_errors" k6_monitor.log | grep -v "count=0"; then
    log_error "Chaos test FAILED: 'ws_connection_errors' were detected. The system may not have recovered gracefully."
    cat k6_monitor.log | grep "ws_connection_errors"
    exit 1
else
    log_info "Chaos test PASSED: No WebSocket connection errors were reported by k6. The system appears resilient to Redis failure."
fi

exit 0
