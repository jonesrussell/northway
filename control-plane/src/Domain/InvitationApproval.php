<?php
declare(strict_types=1);
namespace App\Domain;
use Waaseyaa\Auth\Extension\RegistrationPolicyInterface;
use Waaseyaa\Auth\Extension\RegistrationContext;
use Waaseyaa\Auth\Extension\RegistrationDecision;

// Possession of a generic invitation is not proof of an arbitrary email address.
// Operator verifies the invited person's identity before activating this account.
final class InvitationApproval implements RegistrationPolicyInterface
{
    public function decide(RegistrationContext $context): RegistrationDecision { return RegistrationDecision::requireApproval(); }
}
