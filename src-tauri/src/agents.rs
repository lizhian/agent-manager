use crate::{database::Database, gaal};
use serde::{Deserialize, Serialize};
use std::{env, path::Path, process::Command};

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentsDashboard {
    pub agents: Vec<AgentRecord>,
    pub gaal: gaal::GaalInfo,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct AgentRecord {
    pub name: String,
    pub installed: bool,
    pub source: String,
    #[serde(
        default,
        rename(deserialize = "project_skills_dir", serialize = "projectSkillsDir")
    )]
    pub project_skills_dir: String,
    #[serde(
        default,
        rename(deserialize = "global_skills_dir", serialize = "globalSkillsDir")
    )]
    pub global_skills_dir: String,
    #[serde(
        default,
        rename(
            deserialize = "project_mcp_config_file",
            serialize = "projectMcpConfigFile"
        )
    )]
    pub project_mcp_config_file: String,
    #[serde(
        default,
        rename(
            deserialize = "global_mcp_config_file",
            serialize = "globalMcpConfigFile"
        )
    )]
    pub global_mcp_config_file: String,
    #[serde(
        default,
        rename(
            deserialize = "project_skills_via_generic",
            serialize = "supportsGenericProject"
        )
    )]
    pub supports_generic_project: bool,
    #[serde(
        default,
        rename(
            deserialize = "global_skills_via_generic",
            serialize = "supportsGenericGlobal"
        )
    )]
    pub supports_generic_global: bool,
}

#[derive(Debug, Deserialize)]
struct AgentsOutput {
    agents: Vec<AgentRecord>,
}

#[tauri::command]
pub async fn get_agents_dashboard() -> Result<AgentsDashboard, String> {
    tauri::async_runtime::spawn_blocking(load_dashboard)
        .await
        .map_err(|error| format!("读取本地 Agents 任务异常结束：{error}"))?
}

fn load_dashboard() -> Result<AgentsDashboard, String> {
    let database = Database::new(agent_manager_root()?.join("agent-manager.db"))?;
    if let Some((agents, gaal_version)) = database.load_agent_cache()? {
        let path = gaal::managed_binary_path()?;
        let directory = path
            .parent()
            .unwrap_or(&path)
            .to_string_lossy()
            .into_owned();
        let gaal_info = gaal::GaalInfo {
            installed: path.is_file(),
            path: path.to_string_lossy().into_owned(),
            directory,
            version: gaal_version,
        };
        return Ok(AgentsDashboard {
            agents,
            gaal: gaal_info,
        });
    }

    let gaal_info = gaal::inspect()?;
    if !gaal_info.installed {
        return Ok(AgentsDashboard {
            agents: Vec::new(),
            gaal: gaal_info,
        });
    }

    let output = Command::new(&gaal_info.path)
        .args(["--no-banner", "agents", "-o", "json"])
        .output()
        .map_err(|error| format!("读取本地 Agents 失败：无法启动 GAAL：{error}"))?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
        let stdout = String::from_utf8_lossy(&output.stdout).trim().to_string();
        let detail = if stderr.is_empty() { stdout } else { stderr };
        return Err(format!("读取本地 Agents 失败：{detail}"));
    }

    let mut parsed = serde_json::from_slice::<AgentsOutput>(&output.stdout)
        .map_err(|error| format!("解析本地 Agents 列表失败：{error}"))?;
    if let Some(home) = home_dir() {
        for agent in &mut parsed.agents {
            agent.project_skills_dir = display_path(&agent.project_skills_dir, &home);
            agent.global_skills_dir = display_path(&agent.global_skills_dir, &home);
            agent.project_mcp_config_file = display_path(&agent.project_mcp_config_file, &home);
            agent.global_mcp_config_file = display_path(&agent.global_mcp_config_file, &home);
        }
    }
    database.save_agent_cache(&parsed.agents, &gaal_info.version, &timestamp())?;
    Ok(AgentsDashboard {
        agents: parsed.agents,
        gaal: gaal_info,
    })
}

fn home_dir() -> Option<std::path::PathBuf> {
    env::var_os("HOME")
        .or_else(|| env::var_os("USERPROFILE"))
        .map(Into::into)
}

fn agent_manager_root() -> Result<std::path::PathBuf, String> {
    home_dir()
        .map(|home| home.join(".agent-manager"))
        .ok_or_else(|| "无法确定用户主目录".to_string())
}

fn timestamp() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_secs().to_string())
        .unwrap_or_else(|_| "0".to_string())
}

fn display_path(value: &str, home: &Path) -> String {
    if value.is_empty() {
        return String::new();
    }
    let path = Path::new(value);
    match path.strip_prefix(home) {
        Ok(relative) if relative.as_os_str().is_empty() => "~".to_string(),
        Ok(relative) => Path::new("~").join(relative).to_string_lossy().into_owned(),
        Err(_) => value.to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_gaal_agents_output_with_optional_fields() {
        let output = serde_json::from_str::<AgentsOutput>(
            r#"{"agents":[{"name":"codex","installed":true,"source":"builtin","project_skills_dir":".agents/skills","global_skills_dir":"/tmp/home/.codex/skills","global_mcp_config_file":"/tmp/home/.codex/config.toml"}]}"#,
        )
        .expect("valid agents output");

        assert_eq!(output.agents.len(), 1);
        assert_eq!(output.agents[0].project_mcp_config_file, "");
        assert!(!output.agents[0].supports_generic_project);
        let serialized = serde_json::to_value(&output.agents[0]).expect("serializable agent");
        assert_eq!(serialized["globalSkillsDir"], "/tmp/home/.codex/skills");
        assert!(serialized.get("global_skills_dir").is_none());
    }

    #[test]
    fn shortens_paths_inside_home_directory() {
        let home = Path::new("/tmp/home");
        assert_eq!(
            display_path("/tmp/home/.codex/skills", home),
            "~/.codex/skills"
        );
        assert_eq!(display_path(".agents/skills", home), ".agents/skills");
        assert_eq!(display_path("", home), "");
    }
}
