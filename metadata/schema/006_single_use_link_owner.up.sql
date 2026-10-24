ALTER TABLE single_use_links ADD COLUMN IF NOT EXISTS owner TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_single_use_links_owner_active ON single_use_links (owner, expires_at) WHERE status = 'active';
