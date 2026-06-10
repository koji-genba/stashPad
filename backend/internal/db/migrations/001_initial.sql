CREATE TABLE works (
    id             INTEGER PRIMARY KEY,
    rj_number      TEXT UNIQUE,
    title          TEXT NOT NULL,
    circle         TEXT,
    series_name    TEXT,
    purchase_date  TEXT,
    work_type      TEXT,
    age_rating     TEXT,
    file_format    TEXT,
    file_size_text TEXT,
    event          TEXT,
    root_path      TEXT,
    thumbnail_path TEXT,
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE tags (
    id       INTEGER PRIMARY KEY,
    name     TEXT NOT NULL,
    category TEXT NOT NULL,
    UNIQUE (name, category)
);

CREATE TABLE work_tags (
    work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    PRIMARY KEY (work_id, tag_id)
);

CREATE TABLE play_history (
    id        INTEGER PRIMARY KEY,
    work_id   INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    played_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_work_tags_tag ON work_tags(tag_id);
CREATE INDEX idx_play_history_work ON play_history(work_id, played_at);
