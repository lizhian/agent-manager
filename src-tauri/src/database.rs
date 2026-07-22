use crate::{
    agents::AgentRecord,
    agents_md::AgentsMdFragment,
    mcp::McpServer,
    skills::{Catalog, SkillRecord, SourceRecord},
};
use rusqlite::{params, Connection};
use std::{collections::BTreeSet, fs, path::PathBuf, time::Duration};

pub struct Database {
    path: PathBuf,
}

impl Database {
    pub fn new(path: PathBuf) -> Result<Self, String> {
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent)
                .map_err(|error| format!("创建数据库目录 {} 失败：{error}", parent.display()))?;
        }
        let database = Self { path };
        database.initialize()?;
        Ok(database)
    }

    pub fn load_catalog(&self) -> Result<Catalog, String> {
        let connection = self.connect()?;
        let mut source_statement = connection
            .prepare(
                "SELECT id, source, source_safe, installed_at, updated_at
                 FROM skill_sources
                 ORDER BY source",
            )
            .map_err(|error| format!("准备读取 Skill 来源失败：{error}"))?;
        let source_rows = source_statement
            .query_map([], |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, String>(3)?,
                    row.get::<_, String>(4)?,
                ))
            })
            .map_err(|error| format!("读取 Skill 来源失败：{error}"))?;

        let mut sources = Vec::new();
        for source_row in source_rows {
            let (source_id, source, source_safe, installed_at, updated_at) =
                source_row.map_err(|error| format!("解析 Skill 来源失败：{error}"))?;
            let mut skill_statement = connection
                .prepare(
                    "SELECT name, description, global_enabled, installed_at, updated_at
                     FROM skills
                     WHERE source_id = ?1
                     ORDER BY name",
                )
                .map_err(|error| format!("准备读取 Skills 失败：{error}"))?;
            let skill_rows = skill_statement
                .query_map([source_id], |row| {
                    Ok(SkillRecord {
                        name: row.get(0)?,
                        description: row.get(1)?,
                        global_enabled: row.get::<_, i64>(2)? != 0,
                        legacy_project_paths: Vec::new(),
                        installed_at: row.get(3)?,
                        updated_at: row.get(4)?,
                    })
                })
                .map_err(|error| format!("读取 Skills 失败：{error}"))?;
            let skills = skill_rows
                .collect::<Result<Vec<_>, _>>()
                .map_err(|error| format!("解析 Skills 失败：{error}"))?;
            sources.push(SourceRecord {
                source,
                source_safe,
                installed_at,
                updated_at,
                skills,
            });
        }
        Ok(Catalog { sources })
    }

    pub fn save_catalog(&self, catalog: &Catalog) -> Result<(), String> {
        let mut connection = self.connect()?;
        let transaction = connection
            .transaction()
            .map_err(|error| format!("开始保存 catalog 事务失败：{error}"))?;
        let desired_sources: BTreeSet<&str> = catalog
            .sources
            .iter()
            .map(|source| source.source_safe.as_str())
            .collect();

        for source in &catalog.sources {
            transaction
                .execute(
                    "INSERT INTO skill_sources (source, source_safe, installed_at, updated_at)
                     VALUES (?1, ?2, ?3, ?4)
                     ON CONFLICT(source_safe) DO UPDATE SET
                       source = excluded.source,
                       installed_at = excluded.installed_at,
                       updated_at = excluded.updated_at",
                    params![
                        source.source,
                        source.source_safe,
                        source.installed_at,
                        source.updated_at
                    ],
                )
                .map_err(|error| format!("保存来源 {} 失败：{error}", source.source))?;
            let source_id: i64 = transaction
                .query_row(
                    "SELECT id FROM skill_sources WHERE source_safe = ?1",
                    [&source.source_safe],
                    |row| row.get(0),
                )
                .map_err(|error| format!("读取来源 {} ID 失败：{error}", source.source))?;
            let desired_skills: BTreeSet<&str> = source
                .skills
                .iter()
                .map(|skill| skill.name.as_str())
                .collect();
            for skill in &source.skills {
                transaction
                    .execute(
                        "INSERT INTO skills
                           (source_id, name, description, global_enabled, installed_at, updated_at)
                         VALUES (?1, ?2, ?3, ?4, ?5, ?6)
                         ON CONFLICT(source_id, name) DO UPDATE SET
                           description = excluded.description,
                           global_enabled = excluded.global_enabled,
                           installed_at = excluded.installed_at,
                           updated_at = excluded.updated_at",
                        params![
                            source_id,
                            skill.name,
                            skill.description,
                            i64::from(skill.global_enabled),
                            skill.installed_at,
                            skill.updated_at
                        ],
                    )
                    .map_err(|error| format!("保存 Skill {} 失败：{error}", skill.name))?;
            }
            let existing_skills = collect_strings(
                &transaction,
                "SELECT name FROM skills WHERE source_id = ?1",
                [source_id],
            )?;
            for skill_name in existing_skills {
                if !desired_skills.contains(skill_name.as_str()) {
                    transaction
                        .execute(
                            "DELETE FROM skills WHERE source_id = ?1 AND name = ?2",
                            params![source_id, skill_name],
                        )
                        .map_err(|error| format!("清理失效 Skill 失败：{error}"))?;
                }
            }
        }

        let existing_sources =
            collect_strings(&transaction, "SELECT source_safe FROM skill_sources", [])?;
        for source_safe in existing_sources {
            if !desired_sources.contains(source_safe.as_str()) {
                transaction
                    .execute(
                        "DELETE FROM skill_sources WHERE source_safe = ?1",
                        [&source_safe],
                    )
                    .map_err(|error| format!("清理失效来源 {source_safe} 失败：{error}"))?;
            }
        }
        transaction
            .commit()
            .map_err(|error| format!("提交 catalog 事务失败：{error}"))
    }

    pub fn load_agents_md_fragments(&self) -> Result<Vec<AgentsMdFragment>, String> {
        let connection = self.connect()?;
        let mut statement = connection
            .prepare(
                "SELECT id, title, content, enabled, sort_order, created_at, updated_at
                 FROM agents_md_fragments
                 ORDER BY sort_order, id",
            )
            .map_err(|error| format!("准备读取 AGENTS.md 片段失败：{error}"))?;
        let rows = statement
            .query_map([], |row| {
                Ok(AgentsMdFragment {
                    id: row.get(0)?,
                    title: row.get(1)?,
                    content: row.get(2)?,
                    enabled: row.get::<_, i64>(3)? != 0,
                    sort_order: row.get(4)?,
                    created_at: row.get(5)?,
                    updated_at: row.get(6)?,
                })
            })
            .map_err(|error| format!("读取 AGENTS.md 片段失败：{error}"))?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(|error| format!("解析 AGENTS.md 片段失败：{error}"))
    }

    pub fn save_agents_md_fragments(&self, fragments: &[AgentsMdFragment]) -> Result<(), String> {
        let mut connection = self.connect()?;
        let transaction = connection
            .transaction()
            .map_err(|error| format!("开始保存 AGENTS.md 片段事务失败：{error}"))?;
        transaction
            .execute("DELETE FROM agents_md_fragments", [])
            .map_err(|error| format!("准备覆盖 AGENTS.md 片段失败：{error}"))?;
        for fragment in fragments {
            transaction
                .execute(
                    "INSERT INTO agents_md_fragments
                       (id, title, content, enabled, sort_order, created_at, updated_at)
                     VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
                    params![
                        fragment.id,
                        fragment.title,
                        fragment.content,
                        i64::from(fragment.enabled),
                        fragment.sort_order,
                        fragment.created_at,
                        fragment.updated_at
                    ],
                )
                .map_err(|error| format!("保存 AGENTS.md 片段 {} 失败：{error}", fragment.title))?;
        }
        transaction
            .commit()
            .map_err(|error| format!("提交 AGENTS.md 片段事务失败：{error}"))
    }

    pub fn load_mcp_servers(&self) -> Result<Vec<McpServer>, String> {
        let connection = self.connect()?;
        let mut statement = connection
            .prepare(
                "SELECT id, name, transport, command, args_json, env_json, url,
                        headers_json, agents_json, enabled, created_at, updated_at
                 FROM agent_mcp_servers ORDER BY name, id",
            )
            .map_err(|error| format!("准备读取 MCP 服务失败：{error}"))?;
        let rows = statement
            .query_map([], |row| {
                let args_json = row.get::<_, String>(4)?;
                let env_json = row.get::<_, String>(5)?;
                let headers_json = row.get::<_, String>(7)?;
                let agents_json = row.get::<_, String>(8)?;
                Ok(McpServer {
                    id: row.get(0)?,
                    name: row.get(1)?,
                    transport: row.get(2)?,
                    command: row.get(3)?,
                    args: parse_json_column(&args_json, 4)?,
                    env: parse_json_column(&env_json, 5)?,
                    url: row.get(6)?,
                    headers: parse_json_column(&headers_json, 7)?,
                    agents: parse_json_column(&agents_json, 8)?,
                    enabled: row.get::<_, i64>(9)? != 0,
                    created_at: row.get(10)?,
                    updated_at: row.get(11)?,
                })
            })
            .map_err(|error| format!("读取 MCP 服务失败：{error}"))?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(|error| format!("解析 MCP 服务失败：{error}"))
    }

    pub fn save_mcp_servers(&self, servers: &[McpServer]) -> Result<(), String> {
        let mut connection = self.connect()?;
        let transaction = connection
            .transaction()
            .map_err(|error| format!("开始保存 MCP 服务事务失败：{error}"))?;
        transaction
            .execute("DELETE FROM agent_mcp_servers", [])
            .map_err(|error| format!("准备覆盖 MCP 服务失败：{error}"))?;
        for server in servers {
            transaction
                .execute(
                    "INSERT INTO agent_mcp_servers
                       (id, name, transport, command, args_json, env_json, url,
                        headers_json, agents_json, enabled, created_at, updated_at)
                     VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)",
                    params![
                        server.id,
                        server.name,
                        server.transport,
                        server.command,
                        serde_json::to_string(&server.args)
                            .map_err(|error| format!("序列化 MCP 参数失败：{error}"))?,
                        serde_json::to_string(&server.env)
                            .map_err(|error| format!("序列化 MCP 环境变量失败：{error}"))?,
                        server.url,
                        serde_json::to_string(&server.headers)
                            .map_err(|error| format!("序列化 MCP 请求头失败：{error}"))?,
                        serde_json::to_string(&server.agents)
                            .map_err(|error| format!("序列化 MCP Agent 失败：{error}"))?,
                        i64::from(server.enabled),
                        server.created_at,
                        server.updated_at,
                    ],
                )
                .map_err(|error| format!("保存 MCP 服务 {} 失败：{error}", server.name))?;
        }
        transaction
            .commit()
            .map_err(|error| format!("提交 MCP 服务事务失败：{error}"))
    }

    pub fn load_agent_cache(&self) -> Result<Option<(Vec<AgentRecord>, String)>, String> {
        let connection = self.connect()?;
        let gaal_version = match connection.query_row(
            "SELECT gaal_version FROM agent_cache_metadata WHERE id = 1",
            [],
            |row| row.get::<_, String>(0),
        ) {
            Ok(value) => value,
            Err(rusqlite::Error::QueryReturnedNoRows) => return Ok(None),
            Err(error) => return Err(format!("读取 Agents 缓存元数据失败：{error}")),
        };
        let mut statement = connection
            .prepare(
                "SELECT name, installed, source, project_skills_dir, global_skills_dir,
                        project_mcp_config_file, global_mcp_config_file,
                        supports_generic_project, supports_generic_global
                 FROM agent_cache ORDER BY installed DESC, name",
            )
            .map_err(|error| format!("准备读取 Agents 缓存失败：{error}"))?;
        let rows = statement
            .query_map([], |row| {
                Ok(AgentRecord {
                    name: row.get(0)?,
                    installed: row.get::<_, i64>(1)? != 0,
                    source: row.get(2)?,
                    project_skills_dir: row.get(3)?,
                    global_skills_dir: row.get(4)?,
                    project_mcp_config_file: row.get(5)?,
                    global_mcp_config_file: row.get(6)?,
                    supports_generic_project: row.get::<_, i64>(7)? != 0,
                    supports_generic_global: row.get::<_, i64>(8)? != 0,
                })
            })
            .map_err(|error| format!("读取 Agents 缓存失败：{error}"))?;
        let agents = rows
            .collect::<Result<Vec<_>, _>>()
            .map_err(|error| format!("解析 Agents 缓存失败：{error}"))?;
        Ok(Some((agents, gaal_version)))
    }

    pub fn save_agent_cache(
        &self,
        agents: &[AgentRecord],
        gaal_version: &str,
        updated_at: &str,
    ) -> Result<(), String> {
        let mut connection = self.connect()?;
        let transaction = connection
            .transaction()
            .map_err(|error| format!("开始保存 Agents 缓存事务失败：{error}"))?;
        transaction
            .execute("DELETE FROM agent_cache", [])
            .map_err(|error| format!("准备覆盖 Agents 缓存失败：{error}"))?;
        for agent in agents {
            transaction
                .execute(
                    "INSERT INTO agent_cache
                       (name, installed, source, project_skills_dir, global_skills_dir,
                        project_mcp_config_file, global_mcp_config_file,
                        supports_generic_project, supports_generic_global)
                     VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)",
                    params![
                        agent.name,
                        i64::from(agent.installed),
                        agent.source,
                        agent.project_skills_dir,
                        agent.global_skills_dir,
                        agent.project_mcp_config_file,
                        agent.global_mcp_config_file,
                        i64::from(agent.supports_generic_project),
                        i64::from(agent.supports_generic_global),
                    ],
                )
                .map_err(|error| format!("保存 Agent {} 缓存失败：{error}", agent.name))?;
        }
        transaction
            .execute(
                "INSERT INTO agent_cache_metadata (id, gaal_version, updated_at)
                 VALUES (1, ?1, ?2)
                 ON CONFLICT(id) DO UPDATE SET
                   gaal_version = excluded.gaal_version,
                   updated_at = excluded.updated_at",
                params![gaal_version, updated_at],
            )
            .map_err(|error| format!("保存 Agents 缓存元数据失败：{error}"))?;
        transaction
            .commit()
            .map_err(|error| format!("提交 Agents 缓存事务失败：{error}"))
    }

    pub fn load_app_setting(&self, key: &str) -> Result<Option<String>, String> {
        let connection = self.connect()?;
        match connection.query_row(
            "SELECT value_json FROM app_settings WHERE key = ?1",
            [key],
            |row| row.get(0),
        ) {
            Ok(value) => Ok(Some(value)),
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
            Err(error) => Err(format!("读取应用设置 {key} 失败：{error}")),
        }
    }

    pub fn save_app_setting(
        &self,
        key: &str,
        value_json: &str,
        updated_at: &str,
    ) -> Result<(), String> {
        let connection = self.connect()?;
        connection
            .execute(
                "INSERT INTO app_settings (key, value_json, updated_at)
                 VALUES (?1, ?2, ?3)
                 ON CONFLICT(key) DO UPDATE SET
                   value_json = excluded.value_json,
                   updated_at = excluded.updated_at",
                params![key, value_json, updated_at],
            )
            .map_err(|error| format!("保存应用设置 {key} 失败：{error}"))?;
        Ok(())
    }

    #[cfg(test)]
    pub fn path(&self) -> &std::path::Path {
        &self.path
    }

    fn initialize(&self) -> Result<(), String> {
        let connection = self.connect()?;
        connection
            .execute_batch(
                "PRAGMA journal_mode = WAL;
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
                   VALUES (4, strftime('%s', 'now'));",
            )
            .map_err(|error| format!("初始化 SQLite 数据库失败：{error}"))
    }

    fn connect(&self) -> Result<Connection, String> {
        let connection = Connection::open(&self.path)
            .map_err(|error| format!("打开数据库 {} 失败：{error}", self.path.display()))?;
        connection
            .busy_timeout(Duration::from_secs(5))
            .map_err(|error| format!("配置数据库等待时间失败：{error}"))?;
        connection
            .pragma_update(None, "foreign_keys", "ON")
            .map_err(|error| format!("启用数据库外键失败：{error}"))?;
        Ok(connection)
    }
}

fn collect_strings<P>(connection: &Connection, sql: &str, params: P) -> Result<Vec<String>, String>
where
    P: rusqlite::Params,
{
    let mut statement = connection
        .prepare(sql)
        .map_err(|error| format!("准备数据库清理查询失败：{error}"))?;
    let rows = statement
        .query_map(params, |row| row.get(0))
        .map_err(|error| format!("执行数据库清理查询失败：{error}"))?;
    rows.collect::<Result<Vec<_>, _>>()
        .map_err(|error| format!("读取数据库清理结果失败：{error}"))
}

fn parse_json_column<T: serde::de::DeserializeOwned>(
    value: &str,
    column: usize,
) -> rusqlite::Result<T> {
    serde_json::from_str(value).map_err(|error| {
        rusqlite::Error::FromSqlConversionFailure(
            column,
            rusqlite::types::Type::Text,
            Box::new(error),
        )
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn sample_catalog(description: &str) -> Catalog {
        Catalog {
            sources: vec![SourceRecord {
                source: "owner/repo".into(),
                source_safe: "owner_repo".into(),
                installed_at: "100".into(),
                updated_at: "200".into(),
                skills: vec![SkillRecord {
                    name: "demo".into(),
                    description: description.into(),
                    global_enabled: true,
                    legacy_project_paths: Vec::new(),
                    installed_at: "100".into(),
                    updated_at: "200".into(),
                }],
            }],
        }
    }

    fn sample_agent(name: &str, installed: bool) -> AgentRecord {
        AgentRecord {
            name: name.into(),
            installed,
            source: "builtin".into(),
            project_skills_dir: ".agents/skills".into(),
            global_skills_dir: "~/.agents/skills".into(),
            project_mcp_config_file: String::new(),
            global_mcp_config_file: "~/.agent/mcp.json".into(),
            supports_generic_project: true,
            supports_generic_global: false,
        }
    }

    #[test]
    fn initializes_schema_and_preserves_ids_on_update() {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("time")
            .as_nanos();
        let root = std::env::temp_dir().join(format!("agent-manager-db-{nonce}"));
        let database = Database::new(root.join("agent-manager.db")).expect("database");
        database
            .save_catalog(&sample_catalog("first"))
            .expect("first save");
        let connection = Connection::open(database.path()).expect("open database");
        let source_id: i64 = connection
            .query_row("SELECT id FROM skill_sources", [], |row| row.get(0))
            .expect("source id");
        let skill_id: i64 = connection
            .query_row("SELECT id FROM skills", [], |row| row.get(0))
            .expect("skill id");
        let settings_table: i64 = connection
            .query_row(
                "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'app_settings'",
                [],
                |row| row.get(0),
            )
            .expect("settings table");
        drop(connection);

        database
            .save_catalog(&sample_catalog("updated"))
            .expect("second save");
        let connection = Connection::open(database.path()).expect("reopen database");
        let updated_source_id: i64 = connection
            .query_row("SELECT id FROM skill_sources", [], |row| row.get(0))
            .expect("updated source id");
        let updated_skill_id: i64 = connection
            .query_row("SELECT id FROM skills", [], |row| row.get(0))
            .expect("updated skill id");
        let description: String = connection
            .query_row("SELECT description FROM skills", [], |row| row.get(0))
            .expect("description");

        assert_eq!(settings_table, 1);
        assert_eq!(source_id, updated_source_id);
        assert_eq!(skill_id, updated_skill_id);
        assert_eq!(description, "updated");
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn saves_and_replaces_agent_cache_atomically() {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("time")
            .as_nanos();
        let root = std::env::temp_dir().join(format!("agent-manager-agent-cache-{nonce}"));
        let database = Database::new(root.join("agent-manager.db")).expect("database");

        assert!(database.load_agent_cache().expect("empty cache").is_none());
        database
            .save_agent_cache(
                &[sample_agent("codex", true), sample_agent("cursor", false)],
                "gaal v1",
                "100",
            )
            .expect("save cache");
        let (agents, version) = database
            .load_agent_cache()
            .expect("load cache")
            .expect("cache exists");
        assert_eq!(version, "gaal v1");
        assert_eq!(agents.len(), 2);
        assert_eq!(agents[0].name, "codex");

        database
            .save_agent_cache(&[sample_agent("cursor", false)], "gaal v2", "200")
            .expect("replace cache");
        let (agents, version) = database
            .load_agent_cache()
            .expect("reload cache")
            .expect("cache exists");
        assert_eq!(version, "gaal v2");
        assert_eq!(agents.len(), 1);
        assert_eq!(agents[0].name, "cursor");
        let _ = fs::remove_dir_all(root);
    }
}
