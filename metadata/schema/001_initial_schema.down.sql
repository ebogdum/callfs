-- Rollback script for initial schema
-- This removes all tables and functions created in 001_initial_schema.up.sql

-- Drop triggers
DROP TRIGGER IF EXISTS update_inodes_updated_at ON inodes;
DROP TRIGGER IF EXISTS update_single_use_links_updated_at ON single_use_links;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables (order matters due to foreign keys)
DROP TABLE IF EXISTS single_use_links;
DROP TABLE IF EXISTS inodes;
