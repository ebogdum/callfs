ALTER TABLE single_use_links DROP CONSTRAINT IF EXISTS single_use_links_status_check;
ALTER TABLE single_use_links ADD CONSTRAINT single_use_links_status_check CHECK (status IN ('active', 'used', 'expired', 'revoked'));
CREATE INDEX IF NOT EXISTS idx_single_use_links_status_expires_at ON single_use_links (status, expires_at);
CREATE INDEX IF NOT EXISTS idx_single_use_links_owner ON single_use_links (owner);
