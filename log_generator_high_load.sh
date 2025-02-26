#!/bin/bash

# Configuration variables
MIN_BURST=5000
MAX_BURST=15000
SLOW_PERIOD=5    # seconds between mode changes
SPIKE_PERIOD=10  # seconds for error type spikes

# Array of possible error messages
declare -a ERROR_MESSAGES=(
    "Database connection failed"
    "Null pointer exception"
    "File not found"
    "Access denied"
    "Out of memory"
    "Network timeout occurred"
    "Illegal argument provided"
    "User authentication failed"
)

# Error index for spiking a specific error type
SPIKE_ERROR=0

# Function to generate a random IP address
generate_ip() {
    echo "192.168.$((RANDOM % 254 + 1)).$((RANDOM % 254 + 1))"
}

# Function to get current timestamp
get_timestamp() {
    date -u +"%Y-%m-%dT%H:%M:%SZ"
}

# Function to generate log level with bias
generate_level() {
    # During spike periods, generate more ERRORs to trigger concurrency bug
    if [ $SPIKE_MODE -eq 1 ]; then
        local rand=$((RANDOM % 10))
        if [ $rand -lt 8 ]; then  # 80% ERROR during spike
            echo "ERROR"
        elif [ $rand -lt 9 ]; then
            echo "INFO"
        else
            echo "DEBUG"
        fi
    else
        local rand=$((RANDOM % 3))
        case $rand in
            0) echo "ERROR" ;;
            1) echo "INFO" ;;
            2) echo "DEBUG" ;;
        esac
    fi
}

# Function to generate error message
generate_error_message() {
    local level=$1
    if [ "$level" = "ERROR" ]; then
        # During error type spike, use primarily one error type
        if [ $ERROR_SPIKE_MODE -eq 1 ]; then
            # 80% chance to use the spike error type
            if [ $((RANDOM % 10)) -lt 8 ]; then
                echo "Error 500 - ${ERROR_MESSAGES[$SPIKE_ERROR]}"
            else
                local index=$((RANDOM % ${#ERROR_MESSAGES[@]}))
                echo "Error 500 - ${ERROR_MESSAGES[$index]}"
            fi
        else
            local index=$((RANDOM % ${#ERROR_MESSAGES[@]}))
            echo "Error 500 - ${ERROR_MESSAGES[$index]}"
        fi
    else
        echo ""
    fi
}

# Function to generate a single log entry
generate_log_entry() {
    local level=$(generate_level)
    local ip=$(generate_ip)
    local timestamp=$(get_timestamp)
    local error_message=$(generate_error_message "$level")
    echo "[$timestamp] $level - IP:$ip $error_message"
}

# Initialize modes
SPIKE_MODE=0
ERROR_SPIKE_MODE=0
START_TIME=$(date +%s)

echo "Starting high-volume log generation. Press Ctrl+C to stop..." >&2

# Main loop
while true; do
    CURRENT_TIME=$(date +%s)
    ELAPSED=$((CURRENT_TIME - START_TIME))
    
    # Toggle spike mode every SLOW_PERIOD seconds
    if [ $((ELAPSED % (2*SLOW_PERIOD))) -lt $SLOW_PERIOD ]; then
        if [ $SPIKE_MODE -eq 0 ]; then
            echo "Entering high rate mode (>2500/sec) to demonstrate window shrinking and concurrency bug..." >&2
            SPIKE_MODE=1
        fi
    else
        if [ $SPIKE_MODE -eq 1 ]; then
            echo "Returning to normal rate mode..." >&2
            SPIKE_MODE=0
        fi
    fi
    
    # Toggle error type spike every SPIKE_PERIOD seconds
    if [ $((ELAPSED % (2*SPIKE_PERIOD))) -lt $SPIKE_PERIOD ]; then
        if [ $ERROR_SPIKE_MODE -eq 0 ]; then
            # Pick a random error to spike
            SPIKE_ERROR=$((RANDOM % ${#ERROR_MESSAGES[@]}))
            echo "Spiking error type: ${ERROR_MESSAGES[$SPIKE_ERROR]} to demonstrate emerging pattern detection..." >&2
            ERROR_SPIKE_MODE=1
        fi
    else
        if [ $ERROR_SPIKE_MODE -eq 1 ]; then
            echo "Returning to normal error distribution..." >&2
            ERROR_SPIKE_MODE=0
        fi
    fi
    
    # Generate a burst - larger during spike mode
    burst_size=$MIN_BURST
    if [ $SPIKE_MODE -eq 1 ]; then
        burst_size=$MAX_BURST
    fi
    
    # Generate burst of log entries
    for ((i=0; i<burst_size; i++)); do
        generate_log_entry
    done
    
    # Very short pause between bursts
    sleep 0.01
done
