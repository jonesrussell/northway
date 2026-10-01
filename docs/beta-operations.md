# Invitation beta operator workflow

Candidate implementation, 2026-10-01. Production activation still requires
packaged/target qualification, approved credentials and ingress, and a verified
combined backup/restore. This is not a deployment record.

## Accounts and recovery

`control-plane/bin/beta-account.php` reads a small JSON object from standard input.
Never put passwords, tokens or seeds in command arguments, logs or source control.
`invite` and `recovery` require `output_file` in a private 0700 directory and create
only a new 0600 file. Deliver the token through the existing trusted personal
contact channel. Invitations expire after 24 hours; recovery tokens after one hour.

Registration requires an invitation and creates an inactive account. The operator
verifies the invited person's identity and control of the registered address,
then supplies `action: activate`, exact `account_id` and `identity_verified: true`.
Token possession alone does not verify an arbitrary email address. Public signup,
billing, 2FA, OIDC and generic entity/admin routes are not exposed in this beta.

For recovery, independently verify the person, then supply `action: recovery`,
the exact account ID, `identity_verified: true`, and private output path. The user
enters the token and new password at `/reset-password`. Other old customer
sessions fail after the password changes; login/reset/token mutations are
serialized with a nonblocking private-volume lock across PHP workers. A busy
operation returns 503 and may be retried. Multi-host PHP operation is unsupported.

## Export, suspension and deletion

Verify the requestor and map the **immutable account UUID** to its personal Go
workspace. Never infer this mapping from a browser field or assume it matches
the Pi pilot. Account export (`action: export`) writes identity/roles/UUID privately;
it never exports passwords, token hashes, signing keys or session secrets.

The Go support command deliberately refuses a database owned by a running server.
Announce a bounded beta maintenance window, stop NorthCloud web/PHP/API writers,
and use the exact release image and volumes. Do not stop adjacent workloads.

```text
northway customer export --database <private-db> --tenant <uuid> --output <new-private-file>
northway customer suspend --database <private-db> --tenant <uuid> --confirm-tenant <uuid>
northway customer delete --database <private-db> --tenant <uuid> --confirm-tenant <uuid>
```

Export is paged NDJSON: versioned header, feeds, sources, permitted article
metadata, snapshots and feedback. Internal credential/replay hashes are excluded.
Treat any failed/partial export as unusable; retry to a new private file.
Suspension denies customer API requests and removes the workspace from background
polling. There is no automatic reactivation command in this beta.

Deletion removes customer keys, feed/source/corpus/version/FTS data, work,
snapshots, feedback and customer acquisition state. A minimal UUID/state tombstone
prevents silent re-provisioning. Anonymous acquisition charges retain global caps
through their existing window; deletion must never reset publisher budgets.
This is logical deletion, not a promise of forensic erasure from media or backups.

After the Go deletion succeeds, call the account tool with `action: delete`,
`account_id`, matching `confirm_account_id`, exact `workspace_uuid`,
`identity_verified: true`, and `data_plane_deleted: true`. This deactivates/removes
the account and revokes its auth tokens. Record the tombstone in the protected
deletion ledger used during restore. Verify another customer's account/workspace
still works before ending maintenance. Never use these commands on Pi pilot data
without its separate migration/disposition approval.

## Retention and recovery gates

Existing Go retention is seven days for query replay/work, 90 days for corpus and
24 hours for completed acquisition accounting; snapshot guarantees may extend
retention. Explicit feedback remains until account deletion. These are actual
code policies, not assumed backup retention or an advertised SLA.

Recommend daily encrypted off-host backups retained for seven days during beta,
an initial 24-hour RPO and four-hour RTO target. Confirm operational ownership and
demonstrate recovery before promising these targets. Back up both stores and the
account UUID mapping coherently while writers are quiesced, retain exact image,
schema and profile provenance, and protect master/signing recovery material
separately. Exclude session files from restored service so users sign in again.

Restore into an isolated environment first, apply the deletion/suspension ledger
before enabling accounts or polling, regenerate framework field-access preflight,
and prove account login, UUID mapping, feed/query/feedback isolation and key
revocation. Do not restore only the Go database or blindly restart an old Pi copy.
Off-host destination, encryption custody and automated failure alerts must be
recorded in Intersnipe deployment evidence; local rehearsals alone do not satisfy
the off-host gate.
