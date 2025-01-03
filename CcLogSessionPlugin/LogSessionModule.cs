﻿using AssettoServer.Server.Plugin;
using Autofac;

namespace LogSessionPlugin;

public class LogSessionModule : AssettoServerModule<CcLogSessionConfiguration>
{
    protected override void Load(ContainerBuilder builder)
    {
        builder.RegisterType<LogSessionPlugin>().AsSelf().As<IAssettoServerAutostart>().SingleInstance();
        builder.RegisterType<EntryCarLogSession>().AsSelf();
    }
}
