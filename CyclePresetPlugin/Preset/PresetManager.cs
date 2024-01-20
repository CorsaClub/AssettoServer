using AssettoServer;
using AssettoServer.Server;
using AssettoServer.Server.Configuration;
using AssettoServer.Shared.Network.Packets.Outgoing;
using AssettoServer.Shared.Services;
using Microsoft.Extensions.Hosting;
using Serilog;

namespace CyclePresetPlugin.Preset;

public class PresetManager : CriticalBackgroundService
{
    private readonly ACServerConfiguration _acServerConfiguration;
    private readonly EntryCarManager _entryCarManager;
    private bool _presetChangeRequested = false;
    
    private const string RestartKickReason = "SERVER RESTART FOR TRACK CHANGE (won't take long)";

    public PresetManager(ACServerConfiguration acServerConfiguration, 
        EntryCarManager entryCarManager,
        IHostApplicationLifetime applicationLifetime) : base(applicationLifetime)
    {
        _acServerConfiguration = acServerConfiguration;
        _entryCarManager = entryCarManager;
    }

    public PresetData CurrentPreset { get; private set; } = null!;

    public void SetPreset(PresetData preset)
    {
        CurrentPreset = preset;
        _presetChangeRequested = true;
        
        if (!CurrentPreset.IsInit)
            UpdatePreset();
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                if (_presetChangeRequested)
                    UpdatePreset();
            }
            catch (Exception ex)
            {
                Log.Error(ex, "Error in preset service update");
            }
            finally
            {
                await Task.Delay(1000, stoppingToken);
            }
        }
    }

    private void UpdatePreset()
    {
        if (CurrentPreset.UpcomingType != null && !CurrentPreset.Type!.Equals(CurrentPreset.UpcomingType!))
        {
            Log.Information("Preset change to \'{Name}\' initiated", CurrentPreset.UpcomingType!.Name);
            
            // Notify about restart
            Log.Information("Restarting server");
    
            if (_acServerConfiguration.Extra.EnableClientMessages)
            {
                // Reconnect clients
                Log.Information("Reconnecting all clients for preset change");
                _entryCarManager.BroadcastPacket(new ReconnectClientPacket { Time = (ushort) CurrentPreset.TransitionDuration });
            }
            else
            {
                Log.Information("Kicking all clients for preset change, server restart");
                _entryCarManager.BroadcastPacket(new CSPKickBanMessageOverride { Message = RestartKickReason });
                _entryCarManager.BroadcastPacket(new KickCar { SessionId = 255, Reason = KickReason.Kicked });
            }
        
            var preset = new DirectoryInfo(CurrentPreset.UpcomingType!.PresetFolder).Name;
        
            // Restart the server
            var sleep = (CurrentPreset.TransitionDuration - 1) * 1000;
            Thread.Sleep(sleep);
        
            Program.RestartServer(preset);
        }
    }
}
