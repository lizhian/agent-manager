CREATE TABLE IF NOT EXISTS schema_migrations (
                   version INTEGER PRIMARY KEY,
                   applied_at TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS app_settings (
                   key TEXT PRIMARY KEY,
                   value_json TEXT NOT NULL,
                   updated_at TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS skill_sources (
                   id INTEGER PRIMARY KEY,
                   source TEXT NOT NULL UNIQUE,
                   source_safe TEXT NOT NULL UNIQUE,
                   installed_at TEXT NOT NULL DEFAULT '',
                   updated_at TEXT NOT NULL DEFAULT ''
                 );
                 CREATE TABLE IF NOT EXISTS skills (
                   id INTEGER PRIMARY KEY,
                   source_id INTEGER NOT NULL REFERENCES skill_sources(id) ON DELETE CASCADE,
                   name TEXT NOT NULL,
                   description TEXT NOT NULL DEFAULT '',
                   global_enabled INTEGER NOT NULL DEFAULT 0 CHECK (global_enabled IN (0, 1)),
                   installed_at TEXT NOT NULL DEFAULT '',
                   updated_at TEXT NOT NULL DEFAULT '',
                   UNIQUE(source_id, name)
                 );
                 CREATE INDEX IF NOT EXISTS idx_skills_global_enabled
                   ON skills(global_enabled);
                 CREATE TABLE IF NOT EXISTS agents_md_fragments (
                   id INTEGER PRIMARY KEY,
                   title TEXT NOT NULL,
                   content TEXT NOT NULL,
                   enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
                   sort_order INTEGER NOT NULL,
                   created_at TEXT NOT NULL,
                   updated_at TEXT NOT NULL
                 );
                 CREATE INDEX IF NOT EXISTS idx_agents_md_fragments_sort_order
                   ON agents_md_fragments(sort_order, id);
                 CREATE TABLE IF NOT EXISTS agent_mcp_servers (
                   id INTEGER PRIMARY KEY,
                   name TEXT NOT NULL UNIQUE,
                   transport TEXT NOT NULL,
                   command TEXT NOT NULL DEFAULT '',
                   args_json TEXT NOT NULL DEFAULT '[]',
                   env_json TEXT NOT NULL DEFAULT '{}',
                   url TEXT NOT NULL DEFAULT '',
                   headers_json TEXT NOT NULL DEFAULT '{}',
                   agents_json TEXT NOT NULL DEFAULT '[]',
                   enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
                   created_at TEXT NOT NULL,
                   updated_at TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS agent_cache (
                   name TEXT PRIMARY KEY,
                   installed INTEGER NOT NULL CHECK (installed IN (0, 1)),
                   source TEXT NOT NULL,
                   project_skills_dir TEXT NOT NULL DEFAULT '',
                   global_skills_dir TEXT NOT NULL DEFAULT '',
                   project_mcp_config_file TEXT NOT NULL DEFAULT '',
                   global_mcp_config_file TEXT NOT NULL DEFAULT '',
                   supports_generic_project INTEGER NOT NULL DEFAULT 0
                     CHECK (supports_generic_project IN (0, 1)),
                   supports_generic_global INTEGER NOT NULL DEFAULT 0
                     CHECK (supports_generic_global IN (0, 1))
                 );
                 CREATE TABLE IF NOT EXISTS agent_cache_metadata (
                   id INTEGER PRIMARY KEY CHECK (id = 1),
                   gaal_version TEXT NOT NULL DEFAULT '',
                   updated_at TEXT NOT NULL
                 );
                 INSERT OR IGNORE INTO schema_migrations(version, applied_at)
                   VALUES (1, strftime('%s', 'now'));
                 INSERT OR IGNORE INTO schema_migrations(version, applied_at)
                   VALUES (2, strftime('%s', 'now'));
                 INSERT OR IGNORE INTO schema_migrations(version, applied_at)
                   VALUES (3, strftime('%s', 'now'));
                 INSERT OR IGNORE INTO schema_migrations(version, applied_at)
                   VALUES (4, strftime('%s', 'now'));
