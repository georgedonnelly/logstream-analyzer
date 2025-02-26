# Self-Evolving Log Analysis Tool

A real-time log analysis tool that processes stdin log streams, detects patterns, and dynamically adapts to changing conditions.

## Features

- Real-time log processing with >1,000 entries/sec throughput
- Self-adjusting time window (30-120 seconds) based on processing rate
- Dynamic pattern detection and weighting
- Burst handling with adaptive buffer resizing
- Real-time terminal display updated every second
- Robust error handling for malformed logs

## Build and Run

### Requirements

- Go 1.16+

### Building the Tool

```bash
go build -o log_analyzer
```

3 versions of script

standard
./log_generator.sh | ./log_analyzer -debug  

max
./log_generator_max.sh | ./log_analyzer -buffer=500 -debug
