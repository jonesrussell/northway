-- name: CreateAgentGrant :exec
INSERT INTO agent_grants(id,tenant_id,digest,scopes,created_at,expires_at,label) VALUES(?,?,?,?,?,?,?);
-- name: LookupAgentGrant :one
SELECT * FROM agent_grants WHERE id=?;
-- name: TouchAgentGrant :execrows
UPDATE agent_grants SET last_used_at=max(coalesce(last_used_at,created_at),?) WHERE tenant_id=? AND id=? AND revoked_at IS NULL AND created_at<=? AND expires_at>?;
-- name: RevokeAgentGrant :execrows
UPDATE agent_grants SET revoked_at=coalesce(revoked_at,max(created_at,?)) WHERE tenant_id=? AND id=?;
-- name: AgentGrantCount :one
SELECT count(*) FROM agent_grants WHERE tenant_id=?;
-- name: DeleteAgentGrants :exec
DELETE FROM agent_grants WHERE tenant_id=?;
