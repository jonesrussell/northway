-- +goose Up
-- Public ownership is independent of workspaces. Metadata only, disabled until
-- an operator supplies reviewed display rights and explicit activation.
CREATE TABLE public_state(singleton INTEGER PRIMARY KEY CHECK(singleton=1), corpus_revision BIGINT NOT NULL DEFAULT 0, policy_revision BIGINT NOT NULL DEFAULT 0);
INSERT INTO public_state(singleton) VALUES(1);
CREATE TABLE public_register_exclusions(canonical_url TEXT PRIMARY KEY,publisher TEXT NOT NULL,reason TEXT NOT NULL);
CREATE TABLE public_sources(
 id TEXT PRIMARY KEY CHECK(length(id)=36), canonical_url TEXT NOT NULL UNIQUE,
 title TEXT NOT NULL, publisher TEXT NOT NULL, topic TEXT NOT NULL, language TEXT NOT NULL,
 provenance TEXT NOT NULL, review_state TEXT NOT NULL CHECK(review_state IN ('pending','approved','excluded')),
 rights_basis TEXT NOT NULL CHECK(rights_basis IN ('not_assessed','reviewed_metadata')),
 onboarding BIGINT NOT NULL DEFAULT 0 CHECK(onboarding IN (0,1)),
 enabled BIGINT NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)), policy_revision BIGINT NOT NULL DEFAULT 1,
 CHECK(enabled=0 OR (review_state='approved' AND rights_basis='reviewed_metadata'))
);
CREATE TABLE public_poll_sources(
 source_id TEXT PRIMARY KEY REFERENCES public_sources(id), interval_us BIGINT NOT NULL CHECK(interval_us BETWEEN 86400000000 AND 604800000000),
 max_bytes BIGINT NOT NULL CHECK(max_bytes BETWEEN 1024 AND 2097152), next_at BIGINT NOT NULL CHECK(next_at>=0),
 etag TEXT NOT NULL DEFAULT '', modified TEXT NOT NULL DEFAULT '', claim_id TEXT,
 last_success BIGINT, last_attempt BIGINT, last_status BIGINT NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT ''
);
CREATE TABLE public_poll_attempts(
 id TEXT PRIMARY KEY, source_id TEXT NOT NULL REFERENCES public_sources(id),
 started_at BIGINT NOT NULL, lease_until BIGINT NOT NULL CHECK(lease_until>started_at),
 charged_at BIGINT NOT NULL, charged_bytes BIGINT NOT NULL CHECK(charged_bytes BETWEEN 0 AND 2097152),
 reserved_bytes BIGINT NOT NULL CHECK(reserved_bytes BETWEEN 1024 AND 2097152),
 state TEXT NOT NULL CHECK(state IN ('pending','done','abandoned'))
);
CREATE INDEX public_poll_attempt_window ON public_poll_attempts(charged_at);
CREATE TABLE public_articles(
 id TEXT PRIMARY KEY CHECK(length(id)=36), source_id TEXT NOT NULL REFERENCES public_sources(id), origin_id TEXT NOT NULL,
 url TEXT NOT NULL, title TEXT NOT NULL, content_hash TEXT NOT NULL CHECK(length(content_hash)=64),
 published_at BIGINT, observed_at BIGINT NOT NULL,
 search_vector tsvector GENERATED ALWAYS AS (northway_vector(title)) STORED,
 title_vector tsvector GENERATED ALWAYS AS (northway_vector(title)) STORED,
 UNIQUE(source_id,origin_id)
);
CREATE INDEX public_article_search ON public_articles USING GIN(search_vector);
CREATE TABLE public_article_versions(
 article_id TEXT NOT NULL REFERENCES public_articles(id) ON DELETE CASCADE,
 content_hash TEXT NOT NULL, title TEXT NOT NULL, url TEXT NOT NULL, published_at BIGINT, observed_at BIGINT NOT NULL,
 PRIMARY KEY(article_id,content_hash)
);
CREATE TABLE workspace_public_subscriptions(
 tenant_id TEXT NOT NULL REFERENCES customer_workspaces(tenant_id) ON DELETE CASCADE,
 feed_id TEXT NOT NULL, source_id TEXT NOT NULL REFERENCES public_sources(id),
 PRIMARY KEY(tenant_id,feed_id,source_id),
 FOREIGN KEY(tenant_id,feed_id) REFERENCES feeds(tenant_id,id) ON DELETE CASCADE
);
-- Composite ownership remains on private tables. These READ-ONLY projections
-- expose shared rows solely through active workspace membership and policy.
CREATE VIEW visible_public_memberships AS
 SELECT m.tenant_id,m.feed_id,m.source_id FROM workspace_public_subscriptions m
 JOIN customer_workspaces w ON w.tenant_id=m.tenant_id AND w.state='active'
 JOIN public_sources s ON s.id=m.source_id AND s.enabled=1 AND s.review_state='approved' AND s.rights_basis='reviewed_metadata';
CREATE VIEW retrieval_feed_sources AS
 SELECT tenant_id,feed_id,source_id FROM feed_sources UNION ALL SELECT * FROM visible_public_memberships;
CREATE VIEW retrieval_sources AS
 SELECT tenant_id,id,url,title,enabled FROM sources UNION ALL
 SELECT DISTINCT m.tenant_id,s.id,s.canonical_url,s.title,s.enabled FROM visible_public_memberships m JOIN public_sources s ON s.id=m.source_id;
CREATE VIEW retrieval_articles AS
 SELECT tenant_id,id,source_id,origin_id,url,title,body,content_hash,published_at,observed_at,search_vector,title_vector FROM articles UNION ALL
 SELECT DISTINCT m.tenant_id,a.id,a.source_id,a.origin_id,a.url,a.title,''::text,a.content_hash,a.published_at,a.observed_at,a.search_vector,a.title_vector
 FROM visible_public_memberships m JOIN public_articles a ON a.source_id=m.source_id;
CREATE VIEW retrieval_poll_sources AS
 SELECT tenant_id,source_id,last_success,interval_us FROM poll_sources UNION ALL
 SELECT DISTINCT m.tenant_id,p.source_id,p.last_success,p.interval_us FROM visible_public_memberships m JOIN public_poll_sources p ON p.source_id=m.source_id;
CREATE TABLE public_query_scopes(
 tenant_id TEXT NOT NULL, record_id TEXT NOT NULL, record_kind TEXT NOT NULL CHECK(record_kind IN ('work','snapshot')),
 corpus_revision BIGINT NOT NULL, policy_revision BIGINT NOT NULL, shared BOOLEAN NOT NULL,
 PRIMARY KEY(tenant_id,record_id,record_kind)
);
-- +goose StatementBegin
CREATE FUNCTION public_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='public_articles' THEN
  UPDATE public_state SET corpus_revision=corpus_revision+1;
 ELSE
  UPDATE public_state SET policy_revision=policy_revision+1;
 END IF;
 -- The existing query contract compares feed_revision at admission, completion
 -- and cache reuse. An additive revision propagates shared changes atomically.
 UPDATE feeds SET revision=revision+1 WHERE (tenant_id,id) IN (SELECT tenant_id,feed_id FROM workspace_public_subscriptions);
 RETURN NULL;
END $$;
CREATE FUNCTION public_subscription_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  UPDATE feeds SET revision=revision+1 WHERE tenant_id=OLD.tenant_id AND id=OLD.feed_id;
 ELSE
  UPDATE feeds SET revision=revision+1 WHERE tenant_id=NEW.tenant_id AND id=NEW.feed_id;
 END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION stamp_public_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO public_query_scopes(tenant_id,record_id,record_kind,corpus_revision,policy_revision,shared)
 SELECT NEW.tenant_id,NEW.id,CASE WHEN TG_TABLE_NAME='query_work' THEN 'work' ELSE 'snapshot' END,corpus_revision,policy_revision,
 EXISTS(SELECT 1 FROM workspace_public_subscriptions WHERE tenant_id=NEW.tenant_id AND feed_id=NEW.feed_id) FROM public_state WHERE singleton=1;

 RETURN NEW;
END $$;
CREATE FUNCTION forbid_public_private_collision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='sources' AND EXISTS(SELECT 1 FROM public_sources WHERE id=NEW.id) THEN RAISE EXCEPTION 'public/private source identity collision'; END IF;
 IF TG_TABLE_NAME='public_sources' THEN
 IF EXISTS(SELECT 1 FROM public_register_exclusions WHERE canonical_url=NEW.canonical_url) THEN RAISE EXCEPTION 'excluded public source'; END IF;
 END IF;
 IF TG_TABLE_NAME='public_sources' AND EXISTS(SELECT 1 FROM sources WHERE id=NEW.id) THEN RAISE EXCEPTION 'public/private source identity collision'; END IF;
 IF TG_TABLE_NAME='articles' AND EXISTS(SELECT 1 FROM public_articles WHERE id=NEW.id) THEN RAISE EXCEPTION 'public/private article identity collision'; END IF;
 IF TG_TABLE_NAME='public_articles' AND EXISTS(SELECT 1 FROM articles WHERE id=NEW.id) THEN RAISE EXCEPTION 'public/private article identity collision'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION remove_inactive_subscriptions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.state!='active' THEN DELETE FROM workspace_public_subscriptions WHERE tenant_id=NEW.tenant_id; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER public_article_changed AFTER INSERT OR UPDATE OR DELETE ON public_articles FOR EACH ROW EXECUTE FUNCTION public_changed();
CREATE TRIGGER public_policy_changed AFTER INSERT OR UPDATE OR DELETE ON public_sources FOR EACH ROW EXECUTE FUNCTION public_changed();
CREATE TRIGGER public_membership_changed AFTER INSERT OR DELETE ON workspace_public_subscriptions FOR EACH ROW EXECUTE FUNCTION public_subscription_changed();
CREATE TRIGGER public_work_scope BEFORE INSERT ON query_work FOR EACH ROW EXECUTE FUNCTION stamp_public_scope();
CREATE TRIGGER public_snapshot_scope BEFORE INSERT ON query_snapshots FOR EACH ROW EXECUTE FUNCTION stamp_public_scope();
CREATE TRIGGER public_source_identity BEFORE INSERT OR UPDATE ON public_sources FOR EACH ROW EXECUTE FUNCTION forbid_public_private_collision();
CREATE TRIGGER private_source_identity BEFORE INSERT OR UPDATE ON sources FOR EACH ROW EXECUTE FUNCTION forbid_public_private_collision();
CREATE TRIGGER public_article_identity BEFORE INSERT OR UPDATE ON public_articles FOR EACH ROW EXECUTE FUNCTION forbid_public_private_collision();
CREATE TRIGGER private_article_identity BEFORE INSERT OR UPDATE ON articles FOR EACH ROW EXECUTE FUNCTION forbid_public_private_collision();
CREATE TRIGGER workspace_public_revocation AFTER UPDATE OF state ON customer_workspaces FOR EACH ROW EXECUTE FUNCTION remove_inactive_subscriptions();

-- +goose StatementBegin
CREATE FUNCTION remove_public_query_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 DELETE FROM public_query_scopes WHERE tenant_id=OLD.tenant_id AND record_id=OLD.id AND record_kind=CASE WHEN TG_TABLE_NAME='query_work' THEN 'work' ELSE 'snapshot' END;
 RETURN OLD;
END $$;
-- +goose StatementEnd
CREATE TRIGGER public_work_cleanup AFTER DELETE ON query_work FOR EACH ROW EXECUTE FUNCTION remove_public_query_scope();
CREATE TRIGGER public_snapshot_cleanup AFTER DELETE ON query_snapshots FOR EACH ROW EXECUTE FUNCTION remove_public_query_scope();

-- +goose StatementBegin
CREATE FUNCTION query_key_revoked() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN UPDATE tenants SET entitlement_revision=entitlement_revision+1 WHERE id=OLD.tenant_id;
 ELSIF NEW.revoked_at IS DISTINCT FROM OLD.revoked_at THEN UPDATE tenants SET entitlement_revision=entitlement_revision+1 WHERE id=NEW.tenant_id;
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER query_key_revocation AFTER UPDATE OF revoked_at OR DELETE ON api_keys FOR EACH ROW EXECUTE FUNCTION query_key_revoked();
