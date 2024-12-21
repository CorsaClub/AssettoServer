using AssettoServer.Network.Tcp;
using AssettoServer.Server;
using AssettoServer.Server.Plugin;
using AssettoServer.Shared.Network.Packets.Shared;
using AssettoServer.Shared.Services;
using Microsoft.Extensions.Hosting;
using Serilog;

namespace CcTimeSessionPlugin;

public class CcTimeSession : CriticalBackgroundService, IAssettoServerAutostart
{
    private readonly CcTimeSessionConfiguration _configuration;
    private readonly EntryCarManager _entryCarManager;
    private int _remainingSeconds;

    public CcTimeSession(CcTimeSessionConfiguration configuration, EntryCarManager entryCarManager, IHostApplicationLifetime applicationLifetime) : base(applicationLifetime)
    {
        _configuration = configuration;
        _entryCarManager = entryCarManager;
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var totalTimeSeconds = _configuration.SessionTimeSeconds;

        const int tenMinuteInterval = 10 * 60;
        const int fiveMinuteInterval = 5 * 60;
        const int oneMinuteInterval = 60;

        const int thirtyMinutes = 30 * 60;
        const int fifteenMinutes = 15 * 60;
        const int fiveMinutes = 5 * 60;

        for (var remainingSeconds = totalTimeSeconds; remainingSeconds >= 0;)
        {
            if (stoppingToken.IsCancellationRequested) break;

            _remainingSeconds = remainingSeconds;

            // Choisissez le message en fonction du temps restant.
            var message = remainingSeconds == 0
                ? " [CorsaClub] - Fin de session"
                : $" [CorsaClub] - Il reste {remainingSeconds / 60} minutes.";

            _entryCarManager.BroadcastPacket(new ChatMessage { SessionId = 255, Message = message });
            Log.Information("Remaining time of session : {time} minutes", remainingSeconds / 60);

            if (remainingSeconds == 0)
            {
                EndSession();
                break;
            }

            var sleepInterval = remainingSeconds switch
            {
                >= thirtyMinutes => tenMinuteInterval,
                <= fifteenMinutes and > fiveMinutes => fiveMinuteInterval,
                <= fiveMinutes and > 0 => oneMinuteInterval,
                _ => remainingSeconds
            };

            await Task.Delay(sleepInterval * 1000, stoppingToken);
            remainingSeconds -= sleepInterval;
        }
    }

    private void EndSession()
    {
        Log.Information("Fin de la session");
    }
}