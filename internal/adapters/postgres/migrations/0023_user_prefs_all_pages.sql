-- Design 0020: whether a person's sidebar shows every page. Off is simple,
-- so everyone who set preferences before this column existed starts simple.
ALTER TABLE user_prefs ADD COLUMN IF NOT EXISTS all_pages boolean NOT NULL DEFAULT false;
