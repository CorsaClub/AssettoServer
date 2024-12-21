# CcLogSessionPlugin

A plugin for AssettoServer that logs and sends detailed session data to an API endpoint. It tracks player activities, lap times, collisions, and other session-related information.

Plugin ready for 0.0.55-pre23

## Features

- Tracks player session data including:
  - Distance traveled
  - Maximum speed
  - Session duration
  - Collisions (both with environment and other players)
  - Lap times and sectors
  - Race positions
- Sends data to configured API endpoints when:
  - Players disconnect
  - Sessions end
- Supports mTLS authentication
- Configurable data sending frequency for disconnected players

## Configuration

Create a `plugin_cc_log_session_cfg.yml` file with the following parameters:

```yaml
# Required: Unique identifier for this server
ServerId: "your-server-id"

# Required: API endpoint for player disconnection events
ApiUrlPlayerDisconnect: "https://api.example.com/player-disconnect"

# Required: API endpoint for session end events
ApiUrlSessionEnd: "https://api.example.com/session-end"

# Optional: mTLS certificate paths
CrtPath: "/path/to/cert.crt"  # Optional: Path to certificate file
KeyPath: "/path/to/key.key"   # Optional: Path to key file

# Optional: How often to send disconnected player data (default: 15)
SendDisconnectedFrequencyMinutes: 15
```

## API Data Format

### Request Body

The plugin sends JSON data in the following format:

```json
{
  "serverId": "string",
  "track": "string",
  "trackConfig": "string",
  "minCSPVersion": "number?",
  "sessionType": "number",
  "reverseGrid": "number",
  "reason": "string",
  "grid": [
    {
      "sessionId": "number",
      "steamId": "number"
    }
  ],
  "players": [
    {
      "sessionId": "number",
      "steamId": "number",
      "model": "string",
      "cspVersion": "number?",
      "finalRacePosition": "number",
      "dnf": "boolean",
      "distance": "number",
      "maxSpeed": "number",
      "startTime": "number",
      "endTime": "number",
      "playerCollisions": "number",
      "environmentCollisions": "number",
      "laps": {
        "lapNumber": {
          "time": "number",
          "sectors": {
            "sectorNumber": "time"
          },
          "cuts": "number",
          "position": "number"
        }
      }
    }
  ]
}
```

### Data Fields Explanation

#### Session Data
- `serverId`: Unique identifier for the server
- `track`: Track name
- `trackConfig`: Track configuration/layout
- `minCSPVersion`: Minimum required CSP version (if set)
- `sessionType`: Type of session (Practice = 1, Qualify = 2, Race = 3)
- `reverseGrid`: Number of positions reversed for race grid
- `reason`: Reason for data send ("SessionEnd" or "PlayerLeave")

#### Grid Data (Race sessions only)
- `sessionId`: Car's session ID
- `steamId`: Player's Steam ID

#### Player Data
- `sessionId`: Car's session ID
- `steamId`: Player's Steam ID
- `model`: Car model
- `cspVersion`: Player's CSP version
- `finalRacePosition`: Final position in race (-1 if DNF)
- `dnf`: Whether player Did Not Finish
- `distance`: Total distance traveled (meters)
- `maxSpeed`: Maximum speed reached (km/h)
- `startTime`: Session start time (milliseconds)
- `endTime`: Session end time (milliseconds)
- `playerCollisions`: Number of collisions with other players
- `environmentCollisions`: Number of collisions with environment

#### Lap Data
- `time`: Lap time (milliseconds)
- `sectors`: Sector times (milliseconds)
- `cuts`: Number of track cuts
- `position`: Position when lap was completed

## Security

When using mTLS:
- Both `CrtPath` and `KeyPath` must be provided
- Files must exist at the specified paths
- Supports TLS 1.2 and 1.3
- Server certificate is validated

## Error Handling

The plugin:
- Logs all API communication attempts
- Continues operating if API calls fail
- Retries sending disconnected player data at configured intervals
- Validates all configuration at startup

## Dependencies

- AssettoServer
- .NET 9.0
- System.Net.Http for API communication
- X509 certificates for mTLS (optional)
```
