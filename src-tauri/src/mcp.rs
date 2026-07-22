use crate::{database::Database, gaal};
use reqwest::header::{HeaderMap, HeaderName, HeaderValue};
use rmcp::{
    model::{CallToolRequestParam, JsonObject, Tool},
    transport::{
        sse_client::SseClientConfig, streamable_http_client::StreamableHttpClientTransportConfig,
        SseClientTransport, StreamableHttpClientTransport, TokioChildProcess,
    },
    ServiceExt,
};
use serde::{Deserialize, Serialize};
use std::{
    collections::{BTreeMap, HashSet},
    env, fs,
    path::{Path, PathBuf},
    process::Command,
    sync::{Mutex, MutexGuard, OnceLock},
    time::{Duration, Instant, SystemTime, UNIX_EPOCH},
};
use tokio::process::Command as TokioCommand;

const DEFAULT_AGENTS_KEY: &str = "mcp.defaultAgents";
const DEFAULT_AGENT: &str = "codex";
const DEBUG_TIMEOUT: Duration = Duration::from_secs(30);

static WRITE_LOCK: OnceLock<Mutex<()>> = OnceLock::new();

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpServer {
    pub id: i64,
    pub name: String,
    pub transport: String,
    pub command: String,
    pub args: Vec<String>,
    pub env: BTreeMap<String, String>,
    pub url: String,
    pub headers: BTreeMap<String, String>,
    pub agents: Vec<String>,
    pub enabled: bool,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct McpServerInput {
    name: String,
    transport: String,
    command: String,
    args: Vec<String>,
    env: BTreeMap<String, String>,
    url: String,
    headers: BTreeMap<String, String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpDashboard {
    pub servers: Vec<McpServer>,
    pub gaal: gaal::GaalInfo,
    pub default_agents: Vec<String>,
    pub available_agents: Vec<String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpDebugSnapshot {
    pub tools: Vec<McpTool>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpTool {
    pub name: String,
    pub title: String,
    pub description: String,
    pub input_schema: serde_json::Value,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpToolCallOutput {
    pub result: serde_json::Value,
    pub duration_ms: u128,
}

#[derive(Serialize)]
struct GaalConfig<'a> {
    schema: u8,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    mcps: Vec<GaalMcp<'a>>,
}

#[derive(Serialize)]
struct GaalMcp<'a> {
    name: &'a str,
    agents: &'a [String],
    global: bool,
    inline: GaalInline<'a>,
}

#[derive(Serialize)]
struct GaalInline<'a> {
    #[serde(rename = "type", skip_serializing_if = "str::is_empty")]
    transport: &'a str,
    #[serde(skip_serializing_if = "str::is_empty")]
    command: &'a str,
    #[serde(skip_serializing_if = "slice_is_empty")]
    args: &'a [String],
    #[serde(skip_serializing_if = "BTreeMap::is_empty")]
    env: &'a BTreeMap<String, String>,
    #[serde(skip_serializing_if = "str::is_empty")]
    url: &'a str,
    #[serde(skip_serializing_if = "BTreeMap::is_empty")]
    headers: &'a BTreeMap<String, String>,
}

struct Store {
    database: Database,
    config_path: PathBuf,
}

impl Store {
    fn new() -> Result<Self, String> {
        Self::from_root(home_dir()?.join(".agent-manager"))
    }

    fn from_root(root: PathBuf) -> Result<Self, String> {
        Ok(Self {
            database: Database::new(root.join("agent-manager.db"))?,
            config_path: root.join("mcp").join("gaal.yaml"),
        })
    }

    fn dashboard(&self) -> Result<McpDashboard, String> {
        Ok(McpDashboard {
            servers: self.database.load_mcp_servers()?,
            gaal: gaal::inspect()?,
            default_agents: self.load_default_agents()?,
            available_agents: available_agents().into_iter().map(str::to_string).collect(),
        })
    }

    fn load_default_agents(&self) -> Result<Vec<String>, String> {
        let Some(value) = self.database.load_app_setting(DEFAULT_AGENTS_KEY)? else {
            return Ok(vec![DEFAULT_AGENT.to_string()]);
        };
        let agents = serde_json::from_str::<Vec<String>>(&value)
            .map_err(|error| format!("解析默认目标 Agent 失败：{error}"))?;
        validate_agents(&agents)?;
        Ok(agents)
    }

    fn save_default_agents(&self, agents: &[String]) -> Result<(), String> {
        validate_agents(agents)?;
        let value = serde_json::to_string(agents)
            .map_err(|error| format!("序列化默认目标 Agent 失败：{error}"))?;
        self.database
            .save_app_setting(DEFAULT_AGENTS_KEY, &value, &timestamp())
    }

    fn save(&self, servers: &[McpServer]) -> Result<(), String> {
        let previous = self.database.load_mcp_servers()?;
        let previous_config = fs::read(&self.config_path).ok();
        self.database.save_mcp_servers(servers)?;
        if let Err(error) = self.write_config(servers) {
            let _ = self.database.save_mcp_servers(&previous);
            match previous_config {
                Some(content) => {
                    let _ = atomic_write(&self.config_path, &content);
                }
                None => {
                    let _ = fs::remove_file(&self.config_path);
                }
            }
            return Err(error);
        }
        Ok(())
    }

    fn write_config(&self, servers: &[McpServer]) -> Result<(), String> {
        let agents = self.load_default_agents()?;
        self.write_config_with_agents(servers, &agents)
    }

    fn write_config_with_agents(
        &self,
        servers: &[McpServer],
        agents: &[String],
    ) -> Result<(), String> {
        let mcps = servers
            .iter()
            .filter(|server| server.enabled)
            .map(|server| GaalMcp {
                name: &server.name,
                agents,
                global: true,
                inline: GaalInline {
                    transport: if server.transport == "stdio" {
                        ""
                    } else {
                        &server.transport
                    },
                    command: &server.command,
                    args: &server.args,
                    env: &server.env,
                    url: &server.url,
                    headers: &server.headers,
                },
            })
            .collect();
        let content = serde_yaml::to_string(&GaalConfig { schema: 1, mcps })
            .map_err(|error| format!("生成 GAAL MCP 配置失败：{error}"))?;
        atomic_write(&self.config_path, content.as_bytes())
    }

    fn sync(&self, servers: &[McpServer]) -> Result<(), String> {
        let info = gaal::inspect()?;
        if !info.installed {
            return Err("尚未安装 GAAL，请先在设置中安装".to_string());
        }
        self.write_config(servers)?;
        let work_dir = self
            .config_path
            .parent()
            .ok_or_else(|| "GAAL MCP 配置缺少父目录".to_string())?;
        let output = Command::new(&info.path)
            .arg("--config")
            .arg(&self.config_path)
            .arg("--no-banner")
            .arg("sync")
            .current_dir(work_dir)
            .output()
            .map_err(|error| format!("通过 GAAL 分发 MCP 失败：{error}"))?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
            let stdout = String::from_utf8_lossy(&output.stdout).trim().to_string();
            let detail = if stderr.is_empty() { stdout } else { stderr };
            return Err(format!("通过 GAAL 分发 MCP 失败：{detail}"));
        }
        Ok(())
    }
}

#[tauri::command]
pub fn get_mcp_dashboard() -> Result<McpDashboard, String> {
    Store::new()?.dashboard()
}

#[tauri::command]
pub fn create_mcp_server(input: McpServerInput) -> Result<McpDashboard, String> {
    let _guard = try_write_lock()?;
    let store = Store::new()?;
    let mut servers = store.database.load_mcp_servers()?;
    validate_server(&input, &servers, None)?;
    let now = timestamp();
    servers.push(server_from_input(
        next_server_id(&servers),
        input,
        now.clone(),
        now,
    ));
    store.save(&servers)?;
    store.dashboard()
}

#[tauri::command]
pub fn update_mcp_server(id: i64, input: McpServerInput) -> Result<McpDashboard, String> {
    let _guard = try_write_lock()?;
    let store = Store::new()?;
    let mut servers = store.database.load_mcp_servers()?;
    validate_server(&input, &servers, Some(id))?;
    let index = servers
        .iter()
        .position(|server| server.id == id)
        .ok_or_else(|| format!("未找到 MCP 服务 {id}"))?;
    let old = servers[index].clone();
    let mut next = server_from_input(id, input, old.created_at, timestamp());
    next.enabled = old.enabled;
    servers[index] = next;
    store.save(&servers)?;
    store.dashboard()
}

#[tauri::command]
pub fn delete_mcp_server(id: i64) -> Result<McpDashboard, String> {
    let _guard = try_write_lock()?;
    let store = Store::new()?;
    let mut servers = store.database.load_mcp_servers()?;
    let previous = servers.len();
    servers.retain(|server| server.id != id);
    if servers.len() == previous {
        return Err(format!("未找到 MCP 服务 {id}"));
    }
    store.save(&servers)?;
    store.dashboard()
}

#[tauri::command]
pub fn set_mcp_server_enabled(id: i64, enabled: bool) -> Result<McpDashboard, String> {
    let _guard = try_write_lock()?;
    let store = Store::new()?;
    let mut servers = store.database.load_mcp_servers()?;
    let server = servers
        .iter_mut()
        .find(|server| server.id == id)
        .ok_or_else(|| format!("未找到 MCP 服务 {id}"))?;
    server.enabled = enabled;
    server.updated_at = timestamp();
    store.save(&servers)?;
    store.dashboard()
}

#[tauri::command]
pub fn set_mcp_default_agents(agents: Vec<String>) -> Result<McpDashboard, String> {
    let _guard = try_write_lock()?;
    let store = Store::new()?;
    let previous = store.load_default_agents()?;
    store.save_default_agents(&agents)?;
    let servers = store.database.load_mcp_servers()?;
    if let Err(error) = store.write_config_with_agents(&servers, &agents) {
        let _ = store.save_default_agents(&previous);
        let _ = store.write_config_with_agents(&servers, &previous);
        return Err(error);
    }
    store.dashboard()
}

#[tauri::command]
pub async fn sync_mcps() -> Result<McpDashboard, String> {
    tauri::async_runtime::spawn_blocking(|| {
        let _guard = try_write_lock()?;
        let store = Store::new()?;
        let servers = store.database.load_mcp_servers()?;
        store.sync(&servers)?;
        store.dashboard()
    })
    .await
    .map_err(|error| format!("分发 MCP 任务异常结束：{error}"))?
}

#[tauri::command]
pub async fn inspect_mcp_tools(id: i64) -> Result<McpDebugSnapshot, String> {
    let server = load_server(id)?;
    let tools = tokio::time::timeout(DEBUG_TIMEOUT, list_tools(&server))
        .await
        .map_err(|_| format!("读取 MCP {} tools 失败：连接或响应超过 30 秒", server.name))??;
    Ok(McpDebugSnapshot {
        tools: tools.into_iter().map(McpTool::from).collect(),
    })
}

#[tauri::command]
pub async fn call_mcp_tool(
    id: i64,
    tool_name: String,
    arguments: serde_json::Value,
) -> Result<McpToolCallOutput, String> {
    let server = load_server(id)?;
    let arguments = match arguments {
        serde_json::Value::Object(value) => value,
        _ => return Err("调用 MCP tool 失败：参数必须是 JSON 对象".to_string()),
    };
    let started = Instant::now();
    let result = tokio::time::timeout(
        DEBUG_TIMEOUT,
        invoke_tool(&server, tool_name.trim(), arguments),
    )
    .await
    .map_err(|_| format!("调用 MCP {} tool 失败：执行超过 30 秒", server.name))??;
    Ok(McpToolCallOutput {
        result: serde_json::to_value(result)
            .map_err(|error| format!("序列化 MCP tool 调用结果失败：{error}"))?,
        duration_ms: started.elapsed().as_millis(),
    })
}

fn validate_server(
    input: &McpServerInput,
    servers: &[McpServer],
    current_id: Option<i64>,
) -> Result<(), String> {
    let name = input.name.trim();
    if name.is_empty() || name.chars().count() > 80 || name.chars().any(char::is_whitespace) {
        return Err("MCP 名称必须为 1 到 80 个不含空格的字符".to_string());
    }
    if servers
        .iter()
        .any(|server| server.id != current_id.unwrap_or_default() && server.name == name)
    {
        return Err(format!("MCP 名称 {name} 已存在"));
    }
    match input.transport.as_str() {
        "stdio" if input.command.trim().is_empty() => Err("stdio MCP 必须填写命令".to_string()),
        "http" | "sse" if input.url.trim().is_empty() => {
            Err("HTTP/SSE MCP 必须填写 URL".to_string())
        }
        "stdio" | "http" | "sse" => Ok(()),
        _ => Err("MCP 传输类型必须为 stdio、http 或 sse".to_string()),
    }
}

fn server_from_input(
    id: i64,
    input: McpServerInput,
    created_at: String,
    updated_at: String,
) -> McpServer {
    McpServer {
        id,
        name: input.name.trim().to_string(),
        transport: input.transport,
        command: input.command.trim().to_string(),
        args: input.args,
        env: input.env,
        url: input.url.trim().to_string(),
        headers: input.headers,
        agents: Vec::new(),
        enabled: true,
        created_at,
        updated_at,
    }
}

fn next_server_id(servers: &[McpServer]) -> i64 {
    servers.iter().map(|server| server.id).max().unwrap_or(0) + 1
}

fn validate_agents(agents: &[String]) -> Result<(), String> {
    if agents.is_empty() || agents.iter().any(|agent| agent.trim().is_empty()) {
        return Err("设置默认目标 Agent 失败：至少选择一个 Agent".to_string());
    }
    if agents.len() > 1 && agents.iter().any(|agent| agent == "*") {
        return Err("设置默认目标 Agent 失败：全部 Agent 不能与具体 Agent 同时选择".to_string());
    }
    let available: HashSet<&str> = available_agents().iter().copied().collect();
    if let Some(agent) = agents
        .iter()
        .find(|agent| !available.contains(agent.as_str()))
    {
        return Err(format!("设置默认目标 Agent 失败：GAAL 不支持 {agent}"));
    }
    Ok(())
}

fn available_agents() -> Vec<&'static str> {
    vec![
        "*",
        "amp",
        "antigravity",
        "augment",
        "claude-code",
        "claude-desktop",
        "cline",
        "codex",
        "continue",
        "cursor",
        "gemini-cli",
        "generic",
        "github-copilot",
        "goose",
        "kilo",
        "kiro-cli",
        "opencode",
        "openhands",
        "roo",
        "trae",
        "warp",
        "windsurf",
        "zencoder",
    ]
}

fn load_server(id: i64) -> Result<McpServer, String> {
    Store::new()?
        .database
        .load_mcp_servers()?
        .into_iter()
        .find(|server| server.id == id)
        .ok_or_else(|| format!("读取 MCP 调试配置失败：未找到服务 {id}"))
}

impl From<Tool> for McpTool {
    fn from(tool: Tool) -> Self {
        Self {
            name: tool.name.into_owned(),
            title: tool.title.unwrap_or_default(),
            description: tool
                .description
                .map(|description| description.into_owned())
                .unwrap_or_default(),
            input_schema: serde_json::Value::Object((*tool.input_schema).clone()),
        }
    }
}

async fn list_tools(server: &McpServer) -> Result<Vec<Tool>, String> {
    match server.transport.as_str() {
        "stdio" => {
            let service = ()
                .serve(stdio_transport(server)?)
                .await
                .map_err(|error| format!("连接 MCP {} 读取 tools 失败：{error}", server.name))?;
            let tools = service
                .list_all_tools()
                .await
                .map_err(|error| format!("读取 MCP {} tools 失败：{error}", server.name))?;
            service
                .cancel()
                .await
                .map_err(|error| format!("关闭 MCP {} 调试连接失败：{error}", server.name))?;
            Ok(tools)
        }
        "http" => {
            let service = ()
                .serve(http_transport(server)?)
                .await
                .map_err(|error| format!("连接 MCP {} 读取 tools 失败：{error}", server.name))?;
            let tools = service
                .list_all_tools()
                .await
                .map_err(|error| format!("读取 MCP {} tools 失败：{error}", server.name))?;
            service
                .cancel()
                .await
                .map_err(|error| format!("关闭 MCP {} 调试连接失败：{error}", server.name))?;
            Ok(tools)
        }
        "sse" => {
            let service = ()
                .serve(sse_transport(server).await?)
                .await
                .map_err(|error| format!("连接 MCP {} 读取 tools 失败：{error}", server.name))?;
            let tools = service
                .list_all_tools()
                .await
                .map_err(|error| format!("读取 MCP {} tools 失败：{error}", server.name))?;
            service
                .cancel()
                .await
                .map_err(|error| format!("关闭 MCP {} 调试连接失败：{error}", server.name))?;
            Ok(tools)
        }
        transport => Err(format!(
            "读取 MCP {} tools 失败：不支持 {transport} 传输",
            server.name
        )),
    }
}

async fn invoke_tool(
    server: &McpServer,
    tool_name: &str,
    arguments: JsonObject,
) -> Result<rmcp::model::CallToolResult, String> {
    if tool_name.is_empty() {
        return Err("调用 MCP tool 失败：tool 名称不能为空".to_string());
    }
    let request = || CallToolRequestParam {
        name: tool_name.to_string().into(),
        arguments: Some(arguments.clone()),
    };
    match server.transport.as_str() {
        "stdio" => {
            let service = ()
                .serve(stdio_transport(server)?)
                .await
                .map_err(|error| format!("连接 MCP {} 调用 tool 失败：{error}", server.name))?;
            let result = service.call_tool(request()).await.map_err(|error| {
                format!("调用 MCP {} tool {tool_name} 失败：{error}", server.name)
            })?;
            service
                .cancel()
                .await
                .map_err(|error| format!("关闭 MCP {} 调试连接失败：{error}", server.name))?;
            Ok(result)
        }
        "http" => {
            let service = ()
                .serve(http_transport(server)?)
                .await
                .map_err(|error| format!("连接 MCP {} 调用 tool 失败：{error}", server.name))?;
            let result = service.call_tool(request()).await.map_err(|error| {
                format!("调用 MCP {} tool {tool_name} 失败：{error}", server.name)
            })?;
            service
                .cancel()
                .await
                .map_err(|error| format!("关闭 MCP {} 调试连接失败：{error}", server.name))?;
            Ok(result)
        }
        "sse" => {
            let service = ()
                .serve(sse_transport(server).await?)
                .await
                .map_err(|error| format!("连接 MCP {} 调用 tool 失败：{error}", server.name))?;
            let result = service.call_tool(request()).await.map_err(|error| {
                format!("调用 MCP {} tool {tool_name} 失败：{error}", server.name)
            })?;
            service
                .cancel()
                .await
                .map_err(|error| format!("关闭 MCP {} 调试连接失败：{error}", server.name))?;
            Ok(result)
        }
        transport => Err(format!(
            "调用 MCP {} tool 失败：不支持 {transport} 传输",
            server.name
        )),
    }
}

fn stdio_transport(server: &McpServer) -> Result<TokioChildProcess, String> {
    let mut command = TokioCommand::new(&server.command);
    command.args(&server.args);
    for (key, value) in &server.env {
        command.env(key, expand_environment(value));
    }
    TokioChildProcess::new(command)
        .map_err(|error| format!("启动 MCP {} 调试进程失败：{error}", server.name))
}

fn http_transport(
    server: &McpServer,
) -> Result<StreamableHttpClientTransport<reqwest::Client>, String> {
    let client = reqwest_client(server)?;
    Ok(StreamableHttpClientTransport::with_client(
        client,
        StreamableHttpClientTransportConfig::with_uri(server.url.clone()),
    ))
}

async fn sse_transport(server: &McpServer) -> Result<SseClientTransport<reqwest::Client>, String> {
    SseClientTransport::start_with_client(
        reqwest_client(server)?,
        SseClientConfig {
            sse_endpoint: server.url.clone().into(),
            ..Default::default()
        },
    )
    .await
    .map_err(|error| format!("连接 MCP {} SSE 端点失败：{error}", server.name))
}

fn reqwest_client(server: &McpServer) -> Result<reqwest::Client, String> {
    let mut headers = HeaderMap::new();
    for (name, value) in &server.headers {
        let name = HeaderName::from_bytes(name.as_bytes())
            .map_err(|error| format!("解析 MCP {} 请求头名称失败：{error}", server.name))?;
        let value = HeaderValue::from_str(&expand_environment(value))
            .map_err(|error| format!("解析 MCP {} 请求头值失败：{error}", server.name))?;
        headers.insert(name, value);
    }
    reqwest::Client::builder()
        .default_headers(headers)
        .build()
        .map_err(|error| format!("创建 MCP {} HTTP 调试客户端失败：{error}", server.name))
}

fn expand_environment(value: &str) -> String {
    let mut expanded = value.to_string();
    for (key, item) in env::vars() {
        expanded = expanded.replace(&format!("${{{key}}}"), &item);
    }
    expanded
}

fn slice_is_empty<T>(value: &&[T]) -> bool {
    value.is_empty()
}

fn atomic_write(path: &Path, content: &[u8]) -> Result<(), String> {
    let parent = path
        .parent()
        .ok_or_else(|| format!("路径 {} 没有父目录", path.display()))?;
    fs::create_dir_all(parent)
        .map_err(|error| format!("创建 {} 失败：{error}", parent.display()))?;
    let temporary = parent.join(format!(".gaal.{}.tmp", timestamp_nanos()));
    fs::write(&temporary, content).map_err(|error| format!("写入临时配置失败：{error}"))?;
    if let Err(error) = fs::rename(&temporary, path) {
        let _ = fs::remove_file(&temporary);
        return Err(format!("原子替换 {} 失败：{error}", path.display()));
    }
    Ok(())
}

fn try_write_lock() -> Result<MutexGuard<'static, ()>, String> {
    WRITE_LOCK
        .get_or_init(|| Mutex::new(()))
        .try_lock()
        .map_err(|_| "另一个 MCP 写入任务正在运行，请稍后再试".to_string())
}

fn home_dir() -> Result<PathBuf, String> {
    env::var_os("HOME")
        .or_else(|| env::var_os("USERPROFILE"))
        .map(PathBuf::from)
        .ok_or_else(|| "无法确定用户主目录".to_string())
}

fn timestamp() -> String {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
        .to_string()
}

fn timestamp_nanos() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn writes_agent_level_gaal_config() {
        let root = env::temp_dir().join(format!("agent-manager-mcp-{}", timestamp_nanos()));
        let store = Store::from_root(root.join(".agent-manager")).expect("store");
        let servers = vec![McpServer {
            id: 1,
            name: "filesystem".into(),
            transport: "stdio".into(),
            command: "uvx".into(),
            args: vec!["mcp-server-filesystem".into()],
            env: BTreeMap::new(),
            url: String::new(),
            headers: BTreeMap::new(),
            agents: vec!["codex".into()],
            enabled: true,
            created_at: "1".into(),
            updated_at: "1".into(),
        }];
        store
            .write_config_with_agents(&servers, &["codex".into()])
            .expect("config");
        let content = fs::read_to_string(&store.config_path).expect("read");
        assert!(content.contains("name: filesystem"));
        assert!(content.contains("agents:\n  - codex"));
        assert!(content.contains("global: true"));
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn persists_default_agents_and_uses_them_for_every_mcp() {
        let root = env::temp_dir().join(format!("agent-manager-mcp-targets-{}", timestamp_nanos()));
        let store = Store::from_root(root.join(".agent-manager")).expect("store");
        let agents = vec!["claude-code".into(), "codex".into()];
        store.save_default_agents(&agents).expect("save agents");
        assert_eq!(store.load_default_agents().expect("load agents"), agents);

        let servers = vec![McpServer {
            id: 1,
            name: "demo".into(),
            transport: "http".into(),
            command: String::new(),
            args: Vec::new(),
            env: BTreeMap::new(),
            url: "https://example.com/mcp".into(),
            headers: BTreeMap::new(),
            agents: vec!["cursor".into()],
            enabled: true,
            created_at: "1".into(),
            updated_at: "1".into(),
        }];
        store.write_config(&servers).expect("config");
        let content = fs::read_to_string(&store.config_path).expect("read");
        assert!(content.contains("agents:\n  - claude-code\n  - codex"));
        assert!(!content.contains("cursor"));
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn validates_default_agent_selection() {
        assert!(validate_agents(&[]).is_err());
        assert!(validate_agents(&["*".into(), "codex".into()]).is_err());
        assert!(validate_agents(&["unknown".into()]).is_err());
        assert!(validate_agents(&["codex".into()]).is_ok());
    }

    #[test]
    fn expands_environment_placeholders_for_debugging() {
        let key = format!("AGENT_MANAGER_MCP_TEST_{}", timestamp_nanos());
        env::set_var(&key, "expanded");
        assert_eq!(
            expand_environment(&format!("before-${{{key}}}-after")),
            "before-expanded-after"
        );
        env::remove_var(key);
    }

    #[tokio::test]
    #[ignore = "downloads and starts the MCP reference server"]
    async fn debugs_stdio_server_end_to_end() {
        let server = McpServer {
            id: 1,
            name: "reference".into(),
            transport: "stdio".into(),
            command: "npx".into(),
            args: vec![
                "-y".into(),
                "@modelcontextprotocol/server-everything".into(),
            ],
            env: BTreeMap::new(),
            url: String::new(),
            headers: BTreeMap::new(),
            agents: Vec::new(),
            enabled: true,
            created_at: "1".into(),
            updated_at: "1".into(),
        };
        let tools = list_tools(&server).await.expect("list tools");
        assert!(tools.iter().any(|tool| tool.name == "echo"));
        let result = invoke_tool(
            &server,
            "echo",
            serde_json::Map::from_iter([("message".into(), "hello".into())]),
        )
        .await
        .expect("call echo");
        let json = serde_json::to_string(&result).expect("result json");
        assert!(json.contains("hello"));
    }
}
