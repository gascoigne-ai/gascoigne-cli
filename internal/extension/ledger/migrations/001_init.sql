CREATE TABLE events (
	id          INTEGER PRIMARY KEY,
	ts          INTEGER NOT NULL, -- unix ms
	pipeline    TEXT    NOT NULL,
	key         TEXT    NOT NULL,
	route       TEXT    NOT NULL,
	optimizers  TEXT    NOT NULL, -- comma separated ids
	raw_tokens  INTEGER NOT NULL,
	out_tokens  INTEGER NOT NULL,
	duration_ms INTEGER NOT NULL,
	raw_id      TEXT    NOT NULL,
	outcome     TEXT    NOT NULL
);
CREATE INDEX events_key ON events(key);

CREATE TABLE raw (
	id      TEXT PRIMARY KEY,
	created INTEGER NOT NULL, -- unix ms
	key     TEXT    NOT NULL,
	payload BLOB    NOT NULL  -- JSON encoded payload
);
CREATE INDEX raw_created ON raw(created);

CREATE TABLE failures (
	key     TEXT PRIMARY KEY,
	count   INTEGER NOT NULL,
	last_ts INTEGER NOT NULL, -- unix ms
	reason  TEXT    NOT NULL
);
