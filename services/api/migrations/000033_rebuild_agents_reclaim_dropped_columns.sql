-- Rebuild the agents table when it carries many dropped columns, so their
-- attribute numbers come back (SPA-8852).
--
-- Why: until SPA-8755 (#62), 000010/000011 re-added four agents columns on
-- every boot and 000019 dropped them again. Postgres never gives back the
-- attnum of a dropped column (VACUUM FULL does not either); only a new table
-- does. Staging's tenant "galaxy" reached 1600 attnums (1582 dropped), the hard
-- limit, so 000010 failed with SQLSTATE 54011 and the API crash-looped. #62
-- stopped the churn, but the table is still at the limit: any future
-- ADD COLUMN on agents would fail the same way. The other tenant databases
-- carry 134-198 dropped columns each.
--
-- Every migration file re-runs on every boot (RunMigrationsFS keeps no
-- ledger), so this file must be cheap and a no-op once the table is clean: it
-- does nothing while agents has fewer than 100 dropped columns. After a rebuild
-- it has none.
--
-- The rebuild runs inside the one transaction the runner opens for this file:
-- any error rolls the whole thing back and leaves the old table untouched.
--   1. Take ACCESS EXCLUSIVE on agents (000019/000020 take the same lock on
--      every boot already, so this waits no longer than they do).
--   2. Record, from the catalog, everything that belongs to agents and that
--      CREATE TABLE ... LIKE does not carry over: its constraints (primary key,
--      unique, check, outgoing foreign keys) by name, its other indexes by
--      name, foreign keys of other tables that point at it, owner, grants,
--      row-level-security flags and table comment.
--   3. Copy the live columns and their rows into a new table, drop the
--      incoming foreign keys, drop the old table, rename the new one to agents
--      and recreate everything recorded in step 2 under the same names.
--   4. Check the row count matches and no dropped column is left; otherwise
--      raise, which rolls everything back.
--
-- Objects this file does not know how to carry over (triggers, RLS policies,
-- rules, views or other objects depending on agents, inheritance, identity or
-- generated columns, sequences owned by agents, column-level grants) make it
-- skip the rebuild with a NOTICE instead of losing them. None of them exist on
-- agents today.

DO $rebuild$
DECLARE
    rel          regclass := to_regclass('public.agents');
    n_dropped    int;
    n_before     bigint;
    n_after      bigint;
    owner_name   name;
    acl          aclitem[];
    rls_on       boolean;
    rls_forced   boolean;
    tbl_comment  text;
    own_cons     jsonb;  -- [{name, def}] on agents, in recreate order
    own_idx      jsonb;  -- [def] indexes not backing a constraint
    in_fks       jsonb;  -- [{tbl, name, def}] foreign keys of other tables -> agents
    r            record;
    blocker      text;
BEGIN
    IF rel IS NULL THEN
        RETURN;
    END IF;

    SELECT count(*) INTO n_dropped
      FROM pg_attribute WHERE attrelid = rel AND attnum > 0 AND attisdropped;
    IF n_dropped < 100 THEN
        RETURN;
    END IF;

    -- Things this rebuild would silently lose: skip instead.
    SELECT CASE
        WHEN EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = rel AND NOT tgisinternal) THEN 'triggers'
        WHEN EXISTS (SELECT 1 FROM pg_policy WHERE polrelid = rel) THEN 'RLS policies'
        WHEN EXISTS (SELECT 1 FROM pg_inherits WHERE inhrelid = rel OR inhparent = rel) THEN 'inheritance'
        WHEN EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid = rel AND attnum > 0 AND NOT attisdropped
                       AND (attidentity <> '' OR attgenerated <> '' OR attacl IS NOT NULL)) THEN 'identity/generated columns or column grants'
        WHEN EXISTS (SELECT 1 FROM pg_depend d JOIN pg_class s ON s.oid = d.objid
                      WHERE d.refobjid = rel AND d.classid = 'pg_class'::regclass AND s.relkind = 'S') THEN 'owned sequences'
        WHEN EXISTS (SELECT 1 FROM pg_depend d JOIN pg_rewrite w ON w.oid = d.objid
                      WHERE d.refobjid = rel AND d.classid = 'pg_rewrite'::regclass) THEN 'views or rules'
        WHEN EXISTS (SELECT 1 FROM pg_publication_rel WHERE prrelid = rel) THEN 'publications'
        WHEN (SELECT relreplident FROM pg_class WHERE oid = rel) NOT IN ('d') THEN 'non-default replica identity'
        WHEN EXISTS (SELECT 1 FROM pg_statistic_ext WHERE stxrelid = rel) THEN 'extended statistics'
        WHEN EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = rel AND contype NOT IN ('p', 'u', 'c', 'f', 'n')) THEN 'exclusion constraints'
    END INTO blocker;
    IF blocker IS NOT NULL THEN
        RAISE NOTICE 'agents has % dropped columns but has %; not rebuilding it (SPA-8852)', n_dropped, blocker;
        RETURN;
    END IF;

    -- Same lock 000019/000020 already take on agents earlier in every boot.
    LOCK TABLE public.agents IN ACCESS EXCLUSIVE MODE;

    SELECT pg_get_userbyid(relowner), relacl, relrowsecurity, relforcerowsecurity, obj_description(oid, 'pg_class')
      INTO owner_name, acl, rls_on, rls_forced, tbl_comment
      FROM pg_class WHERE oid = rel;

    -- Primary key and unique first, then checks, then outgoing foreign keys.
    SELECT coalesce(jsonb_agg(jsonb_build_object('name', conname, 'def', pg_get_constraintdef(oid))
                              ORDER BY array_position(ARRAY['p', 'u', 'c', 'f']::"char"[], contype), conname), '[]')
      INTO own_cons
      FROM pg_constraint WHERE conrelid = rel AND contype IN ('p', 'u', 'c', 'f');  -- NOT NULL ('n', PG 18) comes with LIKE

    SELECT coalesce(jsonb_agg(pg_get_indexdef(i.indexrelid) ORDER BY c.relname), '[]')
      INTO own_idx
      FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
     WHERE i.indrelid = rel
       AND NOT EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conindid = i.indexrelid AND k.conrelid = rel);

    SELECT coalesce(jsonb_agg(jsonb_build_object('tbl', conrelid::regclass::text, 'name', conname,
                                                 'def', pg_get_constraintdef(oid))
                              ORDER BY conrelid::regclass::text, conname), '[]')
      INTO in_fks
      FROM pg_constraint WHERE confrelid = rel AND conrelid <> rel AND contype = 'f';

    SELECT count(*) INTO n_before FROM public.agents;

    CREATE TABLE public.agents__spa8852_rebuild
        (LIKE public.agents INCLUDING DEFAULTS INCLUDING STORAGE INCLUDING COMPRESSION INCLUDING COMMENTS);
    INSERT INTO public.agents__spa8852_rebuild SELECT * FROM public.agents;

    FOR r IN SELECT * FROM jsonb_to_recordset(in_fks) AS x(tbl text, name text, def text) LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', r.tbl, r.name);
    END LOOP;

    DROP TABLE public.agents;  -- no CASCADE: anything unexpected still depending on it aborts the rebuild
    ALTER TABLE public.agents__spa8852_rebuild RENAME TO agents;
    EXECUTE format('ALTER TABLE public.agents OWNER TO %I', owner_name);

    FOR r IN SELECT * FROM jsonb_to_recordset(own_cons) AS x(name text, def text) LOOP
        EXECUTE format('ALTER TABLE public.agents ADD CONSTRAINT %I %s', r.name, r.def);
    END LOOP;
    FOR r IN SELECT value AS def FROM jsonb_array_elements_text(own_idx) LOOP
        EXECUTE r.def;
    END LOOP;
    FOR r IN SELECT * FROM jsonb_to_recordset(in_fks) AS x(tbl text, name text, def text) LOOP
        EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I %s', r.tbl, r.name, r.def);
    END LOOP;

    IF acl IS NOT NULL THEN
        FOR r IN SELECT a.grantee, a.privilege_type, a.is_grantable FROM aclexplode(acl) a LOOP
            EXECUTE format('GRANT %s ON public.agents TO %s%s', r.privilege_type,
                           CASE WHEN r.grantee = 0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(r.grantee)) END,
                           CASE WHEN r.is_grantable THEN ' WITH GRANT OPTION' ELSE '' END);
        END LOOP;
    END IF;
    IF rls_on THEN
        ALTER TABLE public.agents ENABLE ROW LEVEL SECURITY;
    END IF;
    IF rls_forced THEN
        ALTER TABLE public.agents FORCE ROW LEVEL SECURITY;
    END IF;
    IF tbl_comment IS NOT NULL THEN
        EXECUTE format('COMMENT ON TABLE public.agents IS %L', tbl_comment);
    END IF;

    SELECT count(*) INTO n_after FROM public.agents;
    IF n_after <> n_before THEN
        RAISE EXCEPTION 'agents rebuild copied % rows, expected %; rolled back (SPA-8852)', n_after, n_before;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid = 'public.agents'::regclass AND attisdropped) THEN
        RAISE EXCEPTION 'agents still has dropped columns after the rebuild; rolled back (SPA-8852)';
    END IF;

    RAISE NOTICE 'agents rebuilt: % rows, % dropped columns reclaimed (SPA-8852)', n_after, n_dropped;
END
$rebuild$;
