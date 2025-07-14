-- Initial schema for CallFS metadata store
-- This migration creates the core tables for inodes and single-use links

-- Create inodes table for filesystem metadata
CREATE TABLE IF NOT EXISTS inodes (
    id BIGSERIAL PRIMARY KEY,
    parent_id BIGINT REFERENCES inodes(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    path TEXT NOT NULL UNIQUE,
    type VARCHAR(20) NOT NULL CHECK (type IN ('file', 'directory')),
    size BIGINT NOT NULL DEFAULT 0,
    mode VARCHAR(10) NOT NULL DEFAULT '0644',
    uid INTEGER NOT NULL DEFAULT 1000,
    gid INTEGER NOT NULL DEFAULT 1000,
    atime TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    mtime TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ctime TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    backend_type VARCHAR(50) NOT NULL CHECK (backend_type IN ('localfs', 's3')),
    callfs_instance_id VARCHAR(100), -- Required for localfs, NULL for s3
    symlink_target TEXT, -- For future symlink support
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_inodes_path ON inodes(path);
CREATE INDEX IF NOT EXISTS idx_inodes_parent_id ON inodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_inodes_name ON inodes(name);
CREATE INDEX IF NOT EXISTS idx_inodes_backend_type ON inodes(backend_type);
CREATE INDEX IF NOT EXISTS idx_inodes_callfs_instance_id ON inodes(callfs_instance_id);

-- Create single_use_links table for secure download links
CREATE TABLE IF NOT EXISTS single_use_links (
    id BIGSERIAL PRIMARY KEY,
    token VARCHAR(255) NOT NULL UNIQUE,
    file_path TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'used', 'expired')),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE,
    used_by_ip INET,
    hmac_signature VARCHAR(512) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for single_use_links
CREATE INDEX IF NOT EXISTS idx_single_use_links_token ON single_use_links(token);
CREATE INDEX IF NOT EXISTS idx_single_use_links_status ON single_use_links(status);
CREATE INDEX IF NOT EXISTS idx_single_use_links_expires_at ON single_use_links(expires_at);
CREATE INDEX IF NOT EXISTS idx_single_use_links_file_path ON single_use_links(file_path);

-- Create trigger to update updated_at column automatically
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to both tables
CREATE TRIGGER update_inodes_updated_at 
    BEFORE UPDATE ON inodes 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_single_use_links_updated_at 
    BEFORE UPDATE ON single_use_links 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Insert root directory entry
INSERT INTO inodes (id, parent_id, name, path, type, size, mode, uid, gid, backend_type, callfs_instance_id)
VALUES (1, NULL, '', '/', 'directory', 0, '0755', 0, 0, 'localfs', 'root')
ON CONFLICT (path) DO NOTHING;
