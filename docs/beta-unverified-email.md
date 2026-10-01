# Invitation beta without email verification

Russell selected unverified accounts on 2026-10-01. No mail setup or real-person
email-verification gate is part of this beta. This supersedes the earlier
operator-activation requirement for new registrations.

A valid single-use invitation creates an active account. The app registration
adapter resets the framework invite default to email_verified=false through the
maintained User setter and repository, within the existing shared mutation lock.
It clears the provisional authenticated session and asks the user to sign in.
Password authentication, session-generation checks, CSRF, rate limits, workspace
UUID ownership, API assertion scope and tenant isolation remain enforced.
Public signup and billing remain disabled. Recovery still requires the operator
to verify the requesting person's account ownership through their trusted contact
channel; no email-based recovery or verification delivery is advertised.

Strict site verification and candidate-local PHP checks passed eight tests and
46 assertions. The complete browser journey and packaged restore are recorded in
the replacement release bundle. Go implementation is unchanged from 744a58b.

Deployment of the preceding qualified release established private origins,
Spaceship API apex/API routing, valid TLS and renewal, bounded two-tenant live
publisher metadata/query/feedback tests, and an actual combined encrypted off-host
restore on the desktop. Adjacent applications remained healthy. The replacement
must retain those checks and activate only after its immutable PHP/web artifacts
pass packaged and target acceptance. The Pi remains intact.
