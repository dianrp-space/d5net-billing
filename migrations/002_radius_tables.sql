-- +goose Up
CREATE TABLE IF NOT EXISTS radcheck (
    id          SERIAL PRIMARY KEY,
    username    TEXT NOT NULL DEFAULT '',
    attribute   TEXT NOT NULL DEFAULT '',
    op          CHAR(2) NOT NULL DEFAULT '==',
    value       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX radcheck_username ON radcheck(username);

CREATE TABLE IF NOT EXISTS radreply (
    id          SERIAL PRIMARY KEY,
    username    TEXT NOT NULL DEFAULT '',
    attribute   TEXT NOT NULL DEFAULT '',
    op          CHAR(2) NOT NULL DEFAULT '=',
    value       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX radreply_username ON radreply(username);

CREATE TABLE IF NOT EXISTS radacct (
    radacctid           BIGSERIAL PRIMARY KEY,
    acctsessionid       TEXT NOT NULL DEFAULT '',
    acctuniqueid        TEXT NOT NULL DEFAULT '',
    username            TEXT NOT NULL DEFAULT '',
    realm               TEXT DEFAULT '',
    nasipaddress        INET NOT NULL,
    nasportid           TEXT,
    nasporttype         TEXT,
    acctstarttime       TIMESTAMPTZ,
    acctupdatetime      TIMESTAMPTZ,
    acctstoptime        TIMESTAMPTZ,
    acctsessiontime     BIGINT,
    acctinputoctets     BIGINT,
    acctoutputoctets    BIGINT,
    framedipaddress     INET,
    callingstationid    TEXT,
    acctterminatecause  TEXT
);
CREATE INDEX radacct_username ON radacct(username);
CREATE INDEX radacct_active ON radacct(acctstoptime) WHERE acctstoptime IS NULL;

-- +goose Down
DROP TABLE IF EXISTS radacct, radreply, radcheck;
