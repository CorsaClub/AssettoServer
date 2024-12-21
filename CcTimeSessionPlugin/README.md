# CcTimeSessionPlugin

A plugin for AssettoServer that manages session duration and broadcasts time-related notifications to players. It provides automated session management with configurable durations and multi-language support.

Plugin ready for 0.0.55-pre23

## Features

- Configurable session duration
- Automated session end handling
- Time remaining notifications with dynamic intervals:
  - Every 10 minutes when > 30 minutes remaining
  - Every 5 minutes when between 15 and 5 minutes remaining
  - Every minute when < 5 minutes remaining
- Multi-language support for notifications
- Session identification support

## Configuration

Create a `plugin_cc_time_session_cfg.yml` file with the following parameters:

```yaml
# Required: Session duration in minutes (default: 30)
SessionTimeMinutes: 30

# Required: Unique identifier for this session
SessionId: "your-session-id"

# Optional: Language for broadcast messages (default: "en")
Language: "en"
```

## Notification Intervals

The plugin sends notifications to all players with the following logic:

### Time Remaining > 30 minutes
- Notifications every 10 minutes

### Time Remaining between 15 and 5 minutes
- Notifications every 5 minutes

### Time Remaining < 5 minutes
- Notifications every minute

### Session End
- Final notification when session ends

## Message Format

Messages are broadcast in the following format:
```
[CorsaClub] - {time_remaining_message}
```

Examples:
- `[CorsaClub] - Il reste 30 minutes.`
- `[CorsaClub] - Il reste 5 minutes.`
- `[CorsaClub] - Fin de session`

## Error Handling

The plugin:
- Validates all configuration at startup
- Continues operating until session time expires
- Gracefully handles server shutdowns
- Logs all important events and time notifications

## Dependencies

- AssettoServer
- .NET 9.0
- FluentValidation for configuration validation 