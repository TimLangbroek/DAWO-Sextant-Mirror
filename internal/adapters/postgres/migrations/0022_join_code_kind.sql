-- Design 0018: a code makes a laptop or an imaging station. '' is a laptop,
-- so every code made before this column existed keeps meaning what it did.
ALTER TABLE join_codes ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT '';
