-- Design 0015: the codes a laptop joins the fleet with.
--
-- The code itself is never stored: hash is an HMAC under a key derived from
-- the console's session key, which this database does not hold. A dump of
-- the table gives no code back.
--
-- No state column, for the reason elevation_requests has none: open-versus-
-- expired is a function of the clock. uses counts down; a code with uses = 0
-- or expires in the past is closed. Rows are pruned a week after expiry, so
-- the list of who made which code survives long enough to ask about it, and
-- the table never becomes a log.
CREATE TABLE IF NOT EXISTS join_codes (
    tenant     text NOT NULL DEFAULT '',
    id         text NOT NULL,
    hash       bytea NOT NULL,
    grp        text NOT NULL DEFAULT '',
    tag        text NOT NULL DEFAULT '',
    uses       integer NOT NULL CHECK (uses >= 0),
    created    timestamptz NOT NULL,
    created_by text NOT NULL DEFAULT '',
    expires    timestamptz NOT NULL,
    PRIMARY KEY (tenant, id)
);

-- A presented code is found by its hash. Unique, so two codes that happen to
-- collide cannot both exist; the console draws again.
CREATE UNIQUE INDEX IF NOT EXISTS join_codes_hash ON join_codes (tenant, hash);
