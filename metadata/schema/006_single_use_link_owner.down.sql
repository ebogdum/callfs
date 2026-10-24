DROP INDEX IF EXISTS idx_single_use_links_owner_active;
ALTER TABLE single_use_links DROP COLUMN IF EXISTS owner;
