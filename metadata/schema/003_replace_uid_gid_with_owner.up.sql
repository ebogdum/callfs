-- Replace OS-style uid/gid with app-level owner string.
-- App users are NOT OS users; there is no mapping between them.
--
-- For databases created with the old schema (uid/gid columns):
-- This migration adds the owner column and drops uid/gid.
-- For databases created with the new schema (owner column already present):
-- This is a no-op handled by IF EXISTS / IF NOT EXISTS guards.

ALTER TABLE inodes ADD COLUMN IF NOT EXISTS owner TEXT NOT NULL DEFAULT '';

UPDATE inodes SET owner = 'root' WHERE path = '/' AND owner = '';

ALTER TABLE inodes DROP COLUMN IF EXISTS uid;
ALTER TABLE inodes DROP COLUMN IF EXISTS gid;
