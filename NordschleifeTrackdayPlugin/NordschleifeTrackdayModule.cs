using AssettoServer.Server.Plugin;
using Autofac;
using AssettoServer.Server;

namespace NordschleifeTrackdayPlugin;

public class NordschleifeTrackdayModule : AssettoServerModule<NordschleifeTrackdayConfiguration>
{
    protected override void Load(ContainerBuilder builder)
    {
        builder.RegisterType<NordschleifeTrackdayPlugin>().AsSelf().As<IAssettoServerAutostart>().SingleInstance();
    }
}
