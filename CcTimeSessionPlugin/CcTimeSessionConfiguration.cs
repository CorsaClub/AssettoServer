using AssettoServer.Server.Configuration;
using JetBrains.Annotations;
using YamlDotNet.Serialization;

namespace CcTimeSessionPlugin;

[UsedImplicitly(ImplicitUseKindFlags.Assign, ImplicitUseTargetFlags.WithMembers)]
public class CcTimeSessionConfiguration : IValidateConfiguration<CcTimeSessionConfigurationValidator>
{
    [YamlMember(Description = "Session duration in minutes")]
    public int SessionTimeMinutes { get; init; } = 30;
    [YamlMember(Description = "The unique ID that will be sent as part of the API POST request")]
    public string? SessionId { get; init; } = null;
    [YamlMember(Description = "Language to use for sending broadcast messages")]
    public string? Language { get; init; } = "en";

    [YamlIgnore] public int SessionTimeSeconds => SessionTimeMinutes * 60;
}