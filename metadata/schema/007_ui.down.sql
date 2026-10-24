DROP INDEX IF EXISTS idx_single_use_links_owner;
DROP INDEX IF EXISTS idx_single_use_links_status_expires_at;
UPDATE single_use_links SET status = 'expired' WHERE status = 'revoked';
ALTER TABLE single_use_links DROP CONSTRAINT IF EXISTS single_use_links_status_check;
ALTER TABLE single_use_links ADD CONSTRAINT single_use_links_status_check CHECK (status IN ('active', 'used', 'expired'));
