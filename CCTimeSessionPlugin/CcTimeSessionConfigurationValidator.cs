using FluentValidation;
using JetBrains.Annotations;

namespace CcTimeSessionPlugin;

[UsedImplicitly]
public class CcTimeSessionConfigurationValidator : AbstractValidator<CcTimeSessionConfiguration>
{
    public CcTimeSessionConfigurationValidator()
    {
        RuleFor(cfg => cfg.SessionTimeMinutes).NotNull().GreaterThan(0);
        RuleFor(cfg => cfg.SessionId).NotNull();
    }
}