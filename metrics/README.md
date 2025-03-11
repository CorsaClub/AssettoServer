Let me format the entire README content in a single code block:

```markdown:metrics/README.md
# Assetto Corsa Server Metrics Wrapper

A high-performance metrics collection and monitoring wrapper for Assetto Corsa Server, with support for VictoriaMetrics and structured logging.

## Overview

This wrapper provides comprehensive monitoring, metrics collection, and logging capabilities for Assetto Corsa servers. It collects various metrics including:

- Server performance metrics (FPS, tick rate, memory usage)
- Player statistics (connections, latency, packet loss)
- Session information (type, duration, player count)
- Network metrics (bytes sent/received, connection quality)
- System resources (CPU, memory, disk usage)

## Features

- Real-time metrics collection and monitoring
- Structured logging with multiple severity levels
- VictoriaMetrics integration for metrics storage
- Circuit breaker pattern for fault tolerance
- Configurable rate limiting
- Health check endpoints
- Session management and tracking
- Player activity monitoring
- CSP version tracking
- Performance metrics collection

## Recent Improvements

The codebase has undergone significant refactoring to improve:

1. **Code Structure**
   - Better separation of concerns
   - Improved modularity
   - Enhanced readability with consistent naming conventions
   - Translated all French comments to English
   - **Consolidated all metrics definitions into a single file** for better maintainability

2. **Error Handling**
   - More robust error handling with structured error types
   - Improved error reporting with context
   - Circuit breaker pattern for fault tolerance
   - Graceful degradation when services are unavailable

3. **Performance**
   - Optimized metrics batching
   - Reduced memory allocations with object pooling
   - Improved concurrency handling
   - Better resource management

4. **Security**
   - Enhanced input validation
   - Improved authentication handling
   - Better protection against potential vulnerabilities

5. **Maintainability**
   - Comprehensive documentation
   - Consistent coding style
   - Improved logging with context
   - Better configuration management
   - **Centralized metrics definitions** for easier updates and consistency

## Installation

1. Clone the repository:
```bash
git clone https://github.com/CorsaClub/AssettoServer
cd metrics
```

2. Build the wrapper:
```bash
go build -o metrics
```

## Usage

Run the wrapper by providing the path to your Assetto Corsa server start script:

```bash
/app/metrics -i /app/start-server.sh [options]
```

### Command Line Options

- `-i`: Path to server start script (default: "./start-server.sh")
- `-args`: Additional arguments for the server

## Configuration

The wrapper can be configured using environment variables:

### Authentication Configuration
- `AUTH_STEAM_ID`: Steam ID for WebSocket authentication
- `AUTH_USER_ID`: User ID for WebSocket authentication

### Server Configuration
- `GAMESERVER_ID`: Unique identifier for the server
- `GAMESERVER_REGION`: Server region
- `SERVER_NAME`: Display name of the server
- `SERVER_TYPE`: Type of server (Public/Official/Private)

### VictoriaMetrics Configuration
- `VICTORIA_METRICS_URL`: VictoriaMetrics server URL
- `VICTORIA_METRICS_PORT`: VictoriaMetrics server port (default: 8428)
- `VICTORIA_METRICS_USERNAME`: Authentication username
- `VICTORIA_METRICS_PASSWORD`: Authentication password
- `VICTORIA_METRICS_REQUEST_TIMEOUT`: Request timeout in seconds
- `VICTORIA_METRICS_CONNECT_TIMEOUT`: Connection timeout in seconds
- `VICTORIA_METRICS_MAX_RETRIES`: Maximum retry attempts
- `VICTORIA_METRICS_RETRY_BACKOFF`: Retry backoff duration

### VictoriaLogs Configuration
- `VICTORIA_LOGS_URL`: VictoriaLogs server URL
- `VICTORIA_LOGS_PORT`: VictoriaLogs server port (default: 9428)
- `VICTORIA_LOGS_USERNAME`: Authentication username
- `VICTORIA_LOGS_PASSWORD`: Authentication password
- `VICTORIA_LOGS_TIMEOUT`: Request timeout in seconds
- `VICTORIA_LOGS_COMPRESSION`: Enable/disable compression (default: true)

### Metrics Configuration
- `METRICS_BATCH_SIZE`: Number of metrics per batch (default: 100)
- `METRICS_FLUSH_INTERVAL`: Interval between metric flushes (default: "5s")
- `METRICS_BUFFER_SIZE`: Size of metrics buffer (default: 1000)
- `METRICS_RETENTION_TIME`: Metrics retention period (default: "24h")
- `METRICS_COMPRESSION`: Enable/disable compression (default: true)

## Metrics

### Server Metrics
- `assetto_server_state`: Current server state
- `assetto_server_players`: Number of connected players
- `assetto_server_fps`: Server FPS
- `assetto_server_tick_rate`: Server tick rate
- `assetto_server_memory_usage_bytes`: Memory usage
- `assetto_server_cpu_usage`: CPU usage

### Player Metrics
- `assetto_server_player_latency_ms`: Player latency
- `assetto_server_player_packet_loss`: Player packet loss
- `assetto_server_player_connects_total`: Total player connections
- `assetto_server_player_disconnects_total`: Total player disconnections

### Session Metrics
- `assetto_server_session_duration_seconds`: Session duration
- `assetto_server_session_players_total`: Players per session
- `assetto_server_session_type`: Current session type
- `assetto_server_session_remaining_seconds`: Time remaining in session
- `assetto_server_session_state_changes_total`: Session state changes

### Network Metrics
- `assetto_server_network_bytes_received_total`: Total bytes received
- `assetto_server_network_bytes_sent_total`: Total bytes sent
- `assetto_server_network_latency_ms`: Network latency
- `assetto_server_network_packet_loss_ratio`: Packet loss ratio

### CSP Metrics
- `assetto_server_csp_version`: CSP version of connected players
- `assetto_server_csp_features_enabled`: Enabled CSP features per player

### System Resource Metrics
- `assetto_server_cpu_usage_per_thread`: CPU usage per thread
- `assetto_server_memory_detailed_bytes`: Detailed memory usage
- `assetto_server_disk_operations_total`: Disk operations count

## Health Checks

The wrapper exposes a health check endpoint at:
```bash
http://localhost:9001/health
```

Returns:
- 200 OK: Server is healthy
- 503 Service Unavailable: Server is unhealthy

## Logging

Logs are structured and include:
- Timestamp
- Log level (INFO, WARNING, ERROR, DEBUG)
- Component
- Message
- Context (server ID, session ID, player info)

Log levels:
- INFO: Normal operational messages
- WARNING: Warning conditions
- ERROR: Error conditions that should be investigated
- DEBUG: Detailed debug information

## Development

### Prerequisites
- Go 1.21 or higher
- VictoriaMetrics server
- VictoriaLogs server (optional)

### Building
```bash
go build
```

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a new Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For support, please open an issue in the GitHub repository or contact the maintainers.

## Acknowledgments

- AssettoServer team for their excellent server
- VictoriaMetrics team for their excellent time series database
- Assetto Corsa community for their continued support
```
