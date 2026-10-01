<?php
declare(strict_types=1);
namespace App\Domain;
use Waaseyaa\Auth\Extension\RegistrationPolicyInterface;
use Waaseyaa\Auth\Extension\RegistrationContext;
use Waaseyaa\Auth\Extension\RegistrationDecision;

// Possession of a generic invitation is not proof of an arbitrary email address.
// Invited beta accounts may sign in without claiming email verification.
final class InvitationApproval implements RegistrationPolicyInterface
{
    public function decide(RegistrationContext $context): RegistrationDecision { return RegistrationDecision::allow(); }
}
