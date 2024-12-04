using AssettoServer.Server.Configuration;
using JetBrains.Annotations;
using YamlDotNet.Serialization;

namespace CcTimeSessionPlugin;

[UsedImplicitly(ImplicitUseKindFlags.Assign, ImplicitUseTargetFlags.WithMembers)]
public class CcTimeSessionConfiguration : IValidateConfiguration<CcTimeSessionConfigurationValidator>
{
    public int SessionTimeMinutes { get; init; } = 30;
    public string? SessionId { get; init; } = null;
    public string? Language { get; init; } = "en";

    [YamlIgnore] public int SessionTimeSeconds => SessionTimeMinutes * 60;
}