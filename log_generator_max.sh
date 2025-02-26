#!/bin/bash
# log_generator_max.sh - A high-performance log generator script

# Maximum performance settings
BURST_SIZE=1000 #smaller burst size for testing
SPIKE_PERIOD=5

# Array of error messages (shortened for efficiency)
declare -a ERROR_MESSAGES=(
    "Database connection failed"
    "Null pointer exception"
    "Out of memory"
    "User authentication failed"
)

# Function for fast log generation
generate_log_entry() {
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local ip="192.168.1.1"
    
    # During spike, use 95% ERROR logs
    if [ $SPIKE_MODE -eq 1 ]; then
        if [ $((RANDOM % 100)) -lt 95 ]; then
            echo "[$timestamp] ERROR - IP:$ip Error 500 - ${ERROR_MESSAGES[$SPIKE_ERROR]}"
        else
            echo "[$timestamp] INFO - IP:$ip"
        fi
    else
        # Normal mode - equal distribution
        local type=$((RANDOM % 3))
        case $type in
            0) echo "[$timestamp] ERROR - IP:$ip Error 500 - ${ERROR_MESSAGES[$((RANDOM % 4))]}" ;;
            1) echo "[$timestamp] INFO - IP:$ip" ;;
            2) echo "[$timestamp] DEBUG - IP:$ip" ;;
        esac
    fi
}

# Initialize
SPIKE_MODE=0
SPIKE_ERROR=0
START_TIME=$(date +%s)

echo "Starting MAXIMUM THROUGHPUT log generation..." >&2

# Main loop - optimized for speed
while true; do
    CURRENT_TIME=$(date +%s)
    ELAPSED=$((CURRENT_TIME - START_TIME))
    
    # Toggle spike mode every 5 seconds
    if [ $((ELAPSED % 10)) -lt 5 ]; then
        if [ $SPIKE_MODE -eq 0 ]; then
            SPIKE_MODE=1
            SPIKE_ERROR=$((RANDOM % 4))
            echo "!!! EXTREME RATE MODE ACTIVATED - WATCH FOR PATTERNS: ${ERROR_MESSAGES[$SPIKE_ERROR]} !!!" >&2
        fi
    else
        if [ $SPIKE_MODE -eq 1 ]; then
            SPIKE_MODE=0
            echo "Normal rate mode" >&2
        fi
    fi
    
    # Generate massive burst
    for ((i=0; i<BURST_SIZE; i++)); do
        generate_log_entry
    done
    sleep 0.05  # add a small sleep to avoid CPU overload
    
    # No sleep at all - maximum throughput
done
