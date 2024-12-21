using AssettoServer.Server.Plugin;
using Autofac;

namespace CcTimeSessionPlugin;

public class CcTimeSessionModule : AssettoServerModule<CcTimeSessionConfiguration>
{
    protected override void Load(ContainerBuilder builder)
    {
        builder.RegisterType<CcTimeSession>().AsSelf().As<IAssettoServerAutostart>().SingleInstance();
    }
}