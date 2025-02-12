﻿using System.Net.Http.Json;
using System.Net.Security;
using System.Security.Authentication;
using System.Security.Cryptography.X509Certificates;
using AssettoServer.Network.Tcp;
using AssettoServer.Server;
using AssettoServer.Server.Configuration;
using AssettoServer.Server.Plugin;
using AssettoServer.Shared.Model;
using AssettoServer.Shared.Services;
using JetBrains.Annotations;
using Microsoft.Extensions.Hosting;
using Serilog;

namespace LogSessionPlugin;

[UsedImplicitly]
public class LogSessionPlugin : CriticalBackgroundService, IAssettoServerAutostart
{
    private List<LogSessionPlayer> PlayerData { get; set; } = [];
    private List<LogSessionPlayer> DisconnectedPlayerData { get; set; } = [];
    private int CurrentSessionGridInverted { get; set; } = 0;
    
    private readonly List<EntryCarLogSession> _instances = [];
    private readonly CcLogSessionConfiguration _configuration;
    private readonly EntryCarManager _entryCarManager;
    private readonly SessionManager _sessionManager;
    private readonly ACServerConfiguration _serverConfiguration;
    private readonly Func<EntryCar, EntryCarLogSession> _entryCarLogSessionFactory;
    private readonly HttpClient _httpClient;

    public LogSessionPlugin(CcLogSessionConfiguration configuration,
        EntryCarManager entryCarManager,
        SessionManager sessionManager,
        ACServerConfiguration serverConfiguration,
        CSPServerScriptProvider scriptProvider,
        Func<EntryCar, EntryCarLogSession> entryCarLogSessionFactory,
        IHostApplicationLifetime applicationLifetime) : base(applicationLifetime)
    {
        _entryCarManager = entryCarManager;
        _sessionManager = sessionManager;
        _serverConfiguration = serverConfiguration;
        _entryCarLogSessionFactory = entryCarLogSessionFactory;
        _configuration = configuration;

        if (_configuration is { CrtPath: not null, KeyPath: not null })
        {
            if (!Path.Exists(_configuration.CrtPath))
            {
                throw new ConfigurationException("CcLogSessionPlugin: .crt file not found");
            }
            if (!Path.Exists(_configuration.KeyPath))
            {
                throw new ConfigurationException("CcLogSessionPlugin: .key file not found");
            }
            
            var clientCertificate = X509Certificate2.CreateFromPemFile(_configuration.CrtPath, _configuration.KeyPath);
            
            _httpClient = new HttpClient(new HttpClientHandler
            {
                ClientCertificateOptions = ClientCertificateOption.Manual,
                SslProtocols =  SslProtocols.Tls12 | SslProtocols.Tls13,
                ServerCertificateCustomValidationCallback = ValidateServerCertificate,
                ClientCertificates = { clientCertificate }
            });
        }
        else
        {
            _httpClient = new HttpClient();
        }
        
        _entryCarManager.ClientConnected += (sender, _) => sender.FirstUpdateSent += OnFirstUpdateSent;
        _entryCarManager.ClientConnected += (sender, _) => sender.Collision += OnCollision;
        _entryCarManager.ClientConnected += (sender, _) => sender.LapCompleted += OnLapCompleted;
        _entryCarManager.ClientConnected += (sender, _) => sender.SectorSplit += OnSectorSplit;

        _entryCarManager.ClientDisconnected += OnDisconnecting;
        
        _sessionManager.SessionChanged += SessionManagerOnSessionChanged;
    }

    private bool ValidateServerCertificate(HttpRequestMessage requestMessage, X509Certificate2? certificate, X509Chain? chain, SslPolicyErrors sslErrors)
    {
        return sslErrors == SslPolicyErrors.None;
    }

    private void SessionManagerOnSessionChanged(SessionManager sender, SessionChangedEventArgs args)
    {
        if (args.PreviousSession == null) return;
        
        foreach (var instance in _instances)
        {
            var car = instance.FinishData();
            if (car == null) continue;
            PlayerData.Add(car);
        }
        
        PlayerData.AddRange(DisconnectedPlayerData);

        var data = new LogSessionData
        {
            ServerId = _configuration.ServerId,
            Track = _serverConfiguration.CSPTrackOptions.Track,
            TrackConfig = _serverConfiguration.Server.TrackConfig,
            SessionType = (int)args.PreviousSession.Configuration.Type,
            ReverseGrid = CurrentSessionGridInverted,
            Reason = "SessionEnd",
            Players = PlayerData,
        };

        if (_serverConfiguration.CSPTrackOptions.MinimumCSPVersion != null)
        {
            data.MinCSPVersion = _serverConfiguration.CSPTrackOptions.MinimumCSPVersion;
        }

        if (args.PreviousSession.Configuration.Type == SessionType.Race && args.PreviousSession.Grid != null)
        {
            data.Grid = args.PreviousSession.Grid.Select(car => new LogSessionGrid(car)).ToList();
        }

        CurrentSessionGridInverted = args.InvertedGridSlots;

        _ = SendData(_configuration.ApiUrlSessionEnd, data);

        PlayerData = [];
    }
    
    private void DisconnectedPlayersSend()
    {
        if (DisconnectedPlayerData.Count == 0) return;
        
        var data = new LogSessionData
        {
            ServerId = _configuration.ServerId,
            Track = _serverConfiguration.CSPTrackOptions.Track,
            TrackConfig = _serverConfiguration.Server.TrackConfig,
            SessionType = (int)_sessionManager.CurrentSession.Configuration.Type,
            ReverseGrid = CurrentSessionGridInverted,
            Reason = "PlayerLeave",
            Players = DisconnectedPlayerData,
        };

        if (_serverConfiguration.CSPTrackOptions.MinimumCSPVersion != null)
        {
            data.MinCSPVersion = _serverConfiguration.CSPTrackOptions.MinimumCSPVersion;
        }

        if (_sessionManager.CurrentSession.Configuration.Type == SessionType.Race && _sessionManager.CurrentSession.Grid != null)
        {
            data.Grid = _sessionManager.CurrentSession.Grid.Select(car => new LogSessionGrid(car)).ToList();
        }

        _ = SendData(_configuration.ApiUrlPlayerDisconnect, data);

        DisconnectedPlayerData = [];
    }

    private void OnFirstUpdateSent(ACTcpClient sender, EventArgs args) => 
        _instances[sender.SessionId].SetActive();

    private void OnDisconnecting(ACTcpClient sender, EventArgs args)
    {
        var data = _instances[sender.SessionId].FinishData();
        if (data == null) return;
        DisconnectedPlayerData.Add(data);
    }

    private void OnCollision(ACTcpClient sender, CollisionEventArgs args) => 
        _instances[sender.SessionId].UpdateCollisions(args);

    private void OnLapCompleted(ACTcpClient sender, LapCompletedEventArgs args) =>
        _instances[sender.SessionId].UpdateLaps(args);

    private void OnSectorSplit(ACTcpClient sender, SectorSplitEventArgs args) => 
        _instances[sender.SessionId].UpdateSector(args);

    private async Task SendData(string url, LogSessionData data)
    {
        var jsonData = JsonContent.Create(data);
        var response = await _httpClient.PostAsync(url, jsonData);

        var result = await response.Content.ReadAsStringAsync();
        Log.Verbose("CcLogSessionPlugin: Data - {Data}", result);

        if (response.IsSuccessStatusCode)
            Log.Information("CcLogSessionPlugin: Data sent successfully");
        else
            Log.Warning("CcLogSessionPlugin: Data not sent successfully");
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        foreach (var entryCar in _entryCarManager.EntryCars)
        {
            _instances.Add(_entryCarLogSessionFactory(entryCar));
        }

        _ = ExecuteDisconnectUpdates(stoppingToken);
        await ExecuteCarUpdates(stoppingToken);
    }

    private async Task ExecuteCarUpdates(CancellationToken stoppingToken)
    {
        using var timer = new PeriodicTimer(TimeSpan.FromSeconds(1));
        while (await timer.WaitForNextTickAsync(stoppingToken))
        {
            try
            {
                foreach (var instance in _instances)
                {
                    instance.Update();
                }
            }
            catch (Exception ex)
            {
                Log.Error(ex, "Error during CC log session update");
            }
        }
    }

    private async Task ExecuteDisconnectUpdates(CancellationToken stoppingToken)
    {
        using var timer = new PeriodicTimer(TimeSpan.FromMinutes(_configuration.SendDisconnectedFrequencyMinutes));
        while (await timer.WaitForNextTickAsync(stoppingToken))
        {
            try
            {
                DisconnectedPlayersSend();
            }
            catch (Exception ex)
            {
                Log.Error(ex, "Error during CC disconnect send update");
            }
        }
    }
}
