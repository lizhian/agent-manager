use crate::database::Database;
use serde::{Deserialize, Serialize};
use std::{
    collections::{BTreeMap, BTreeSet},
    env, fs, io,
    path::{Path, PathBuf},
    process::Command,
    sync::{Mutex, MutexGuard, OnceLock},
    time::{SystemTime, UNIX_EPOCH},
};
use tauri::ipc::Channel;
use tauri_plugin_opener::OpenerExt;

static WRITE_LOCK: OnceLock<Mutex<()>> = OnceLock::new();
static MIGRATION_LOCK: OnceLock<Mutex<()>> = OnceLock::new();

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Catalog {
    #[serde(default)]
    pub sources: Vec<SourceRecord>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SourceRecord {
    pub source: String,
    pub source_safe: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub installed_at: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub updated_at: String,
    #[serde(default)]
    pub skills: Vec<SkillRecord>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillRecord {
    pub name: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub description: String,
    #[serde(default)]
    pub global_enabled: bool,
    #[serde(default, rename = "projectPaths", skip_serializing)]
    pub(crate) legacy_project_paths: Vec<String>,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub installed_at: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub updated_at: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillsDashboard {
    pub catalog: Catalog,
    pub repository_path: String,
    pub global_target_path: String,
    pub npx_available: bool,
    pub enabled_content_chars: usize,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SourceSyncResult {
    pub source_safe: String,
    pub installed_count: usize,
    pub removed_count: usize,
    pub updated: bool,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoveSourceImpact {
    pub installed_skills: usize,
    pub global_enabled_skills: usize,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillDetail {
    pub source: String,
    pub source_safe: String,
    pub name: String,
    pub description: String,
    pub metadata_name: String,
    pub metadata_description: String,
    pub global_enabled: bool,
    pub updated_at: String,
    pub path: String,
    pub content: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillDocument {
    pub source_safe: String,
    pub skill_name: String,
    pub title: String,
    pub relative_path: String,
    pub content: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OperationProgress {
    pub operation: String,
    pub source: String,
    pub stage: String,
    pub message: String,
    pub percent: Option<u8>,
    pub completed_sources: usize,
    pub total_sources: usize,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BatchUpdateFailure {
    pub source: String,
    pub error: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BatchUpdateResult {
    pub total_sources: usize,
    pub succeeded_sources: usize,
    pub failed_sources: usize,
    pub installed_count: usize,
    pub removed_count: usize,
    pub failures: Vec<BatchUpdateFailure>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ListedSkill {
    name: String,
    path: String,
}

#[derive(Debug)]
struct PreparedSkill {
    name: String,
    path: PathBuf,
}

struct Store {
    repository_dir: PathBuf,
    database: Database,
    legacy_catalog_path: PathBuf,
    global_target_dir: PathBuf,
}

impl Store {
    fn new() -> Result<Self, String> {
        let home = home_dir()?;
        Self::from_root(home.join(".agent-manager"))
    }

    fn from_root(root: PathBuf) -> Result<Self, String> {
        let home = root.parent().ok_or("Agent Manager 根目录无效")?;
        let store = Self {
            repository_dir: root.join("skills"),
            database: Database::new(root.join("agent-manager.db"))?,
            legacy_catalog_path: root.join("install-skills.json"),
            global_target_dir: home.join(".agents").join("skills"),
        };
        store.migrate_legacy_catalog()?;
        Ok(store)
    }

    fn load_catalog_raw(&self) -> Result<Catalog, String> {
        let mut catalog = self.database.load_catalog()?;
        normalize_catalog(&mut catalog);
        Ok(catalog)
    }

    fn load_catalog(&self) -> Result<Catalog, String> {
        let mut catalog = self.load_catalog_raw()?;
        if catalog_has_legacy_projects(&catalog) {
            let _guard = try_write_lock()?;
            catalog = self.load_catalog_raw()?;
            self.migrate_legacy_projects(&mut catalog)?;
        }
        Ok(catalog)
    }

    fn save_catalog(&self, catalog: &Catalog) -> Result<(), String> {
        let mut catalog = catalog.clone();
        normalize_catalog(&mut catalog);
        self.database.save_catalog(&catalog)
    }

    fn migrate_legacy_catalog(&self) -> Result<(), String> {
        let _guard = MIGRATION_LOCK
            .get_or_init(|| Mutex::new(()))
            .lock()
            .map_err(|_| "旧 catalog 迁移锁已损坏".to_string())?;
        let data = match fs::read(&self.legacy_catalog_path) {
            Ok(data) => data,
            Err(error) if error.kind() == io::ErrorKind::NotFound => return Ok(()),
            Err(error) => return Err(format!("读取旧 Skills catalog 失败：{error}")),
        };
        let mut catalog: Catalog = serde_json::from_slice(&data)
            .map_err(|error| format!("解析旧 Skills catalog 失败：{error}"))?;
        normalize_catalog(&mut catalog);
        if catalog_has_legacy_projects(&catalog) {
            self.migrate_legacy_projects(&mut catalog)?;
        } else {
            self.save_catalog(&catalog)?;
        }
        fs::remove_file(&self.legacy_catalog_path)
            .map_err(|error| format!("删除旧 Skills catalog 失败：{error}"))
    }

    fn migrate_legacy_projects(&self, catalog: &mut Catalog) -> Result<(), String> {
        for source in &catalog.sources {
            for skill in &source.skills {
                for project in &skill.legacy_project_paths {
                    let link = Path::new(project)
                        .join(".agents")
                        .join("skills")
                        .join(activation_name(&source.source_safe, &skill.name));
                    let metadata = match fs::symlink_metadata(&link) {
                        Ok(metadata) => metadata,
                        Err(error) if error.kind() == io::ErrorKind::NotFound => continue,
                        Err(error) => return Err(format!("检查旧项目 Skill 失败：{error}")),
                    };
                    let expected = self
                        .repository_dir
                        .join(&source.source_safe)
                        .join(&skill.name);
                    if !metadata.file_type().is_symlink() {
                        return Err(format!("旧项目路径 {} 不是软链，已保留", link.display()));
                    }
                    let target = fs::read_link(&link)
                        .map_err(|error| format!("读取旧项目软链失败：{error}"))?;
                    if target != expected {
                        return Err(format!(
                            "旧项目软链 {} 指向非受管目标，已保留",
                            link.display()
                        ));
                    }
                    fs::remove_file(&link)
                        .map_err(|error| format!("清理旧项目软链失败：{error}"))?;
                }
            }
        }
        for source in &mut catalog.sources {
            for skill in &mut source.skills {
                skill.legacy_project_paths.clear();
            }
        }
        self.save_catalog(catalog)
    }

    fn install_or_update(
        &self,
        source: &str,
        operation: &str,
        progress: &ProgressReporter,
    ) -> Result<SourceSyncResult, String> {
        progress.send(source, "validate", "正在校验来源", Some(4));
        validate_source(source)?;
        let source = source.trim();
        let source_safe = source_safe_name(source);
        progress.send(source, "prepare", "正在准备临时目录", Some(10));
        let temp_dir = env::temp_dir().join(format!(
            "agent-manager-skills-{}-{}",
            std::process::id(),
            timestamp()
        ));
        fs::create_dir_all(&temp_dir).map_err(|error| format!("创建临时安装目录失败：{error}"))?;
        progress.send(source, "download", "正在下载并解析来源", None);
        let result = self.prepare_source(source, &temp_dir).and_then(|prepared| {
            progress.send(source, "parse", "已发现 Skills，正在读取元信息", Some(48));
            self.commit_prepared_source(source, &source_safe, prepared, progress)
        });
        let _ = fs::remove_dir_all(temp_dir);
        if result.is_ok() {
            progress.send(
                source,
                "complete",
                if operation == "install" {
                    "安装完成"
                } else {
                    "更新完成"
                },
                Some(100),
            );
        }
        result
    }

    fn prepare_source(&self, source: &str, temp_dir: &Path) -> Result<Vec<PreparedSkill>, String> {
        run_skills_command(temp_dir, &["add", source, "-y"])?;
        let output = Command::new("npx")
            .args(["--yes", "skills", "list", "--json"])
            .current_dir(temp_dir)
            .output()
            .map_err(|error| format!("运行 npx skills list 失败：{error}"))?;
        if !output.status.success() {
            return Err(command_error("读取已安装 Skills", &output));
        }
        let stdout = String::from_utf8_lossy(&output.stdout);
        let json = stdout
            .find('[')
            .map(|index| &stdout[index..])
            .unwrap_or("[]");
        let listed: Vec<ListedSkill> = serde_json::from_str(json.trim())
            .map_err(|error| format!("解析 npx skills 输出失败：{error}"))?;
        let prepared: Vec<PreparedSkill> = listed
            .into_iter()
            .filter_map(|skill| {
                let name = skill.name.trim().to_string();
                let path = PathBuf::from(skill.path.trim());
                (!name.is_empty() && !skill.path.trim().is_empty())
                    .then_some(PreparedSkill { name, path })
            })
            .collect();
        if prepared.is_empty() {
            return Err(format!("来源 {source} 中没有可安装的 Skill"));
        }
        Ok(prepared)
    }

    fn commit_prepared_source(
        &self,
        source: &str,
        source_safe: &str,
        mut prepared: Vec<PreparedSkill>,
        progress: &ProgressReporter,
    ) -> Result<SourceSyncResult, String> {
        prepared.sort_by(|a, b| a.name.cmp(&b.name));
        let old_catalog = self.load_catalog_raw()?;
        let old_source = old_catalog
            .sources
            .iter()
            .find(|item| item.source_safe == source_safe)
            .cloned();
        let updated = old_source.is_some();
        let now = timestamp();
        let mut new_source = SourceRecord {
            source: source.to_string(),
            source_safe: source_safe.to_string(),
            installed_at: old_source
                .as_ref()
                .map(|item| item.installed_at.clone())
                .filter(|value| !value.is_empty())
                .unwrap_or_else(|| now.clone()),
            updated_at: now.clone(),
            skills: Vec::new(),
        };
        for skill in &prepared {
            let previous = old_source
                .as_ref()
                .and_then(|item| item.skills.iter().find(|old| old.name == skill.name));
            new_source.skills.push(SkillRecord {
                name: skill.name.clone(),
                description: read_skill_description(&skill.path),
                global_enabled: previous.is_some_and(|item| item.global_enabled),
                legacy_project_paths: Vec::new(),
                installed_at: previous
                    .map(|item| item.installed_at.clone())
                    .filter(|value| !value.is_empty())
                    .unwrap_or_else(|| now.clone()),
                updated_at: now.clone(),
            });
        }

        progress.send(source, "mirror", "正在写入本地镜像", Some(62));
        fs::create_dir_all(&self.repository_dir)
            .map_err(|error| format!("创建 Skills 仓库失败：{error}"))?;
        let staging = self.repository_dir.join(format!(".staging-{source_safe}"));
        let final_dir = self.repository_dir.join(source_safe);
        let backup = self.repository_dir.join(format!("{source_safe}.bak"));
        remove_if_exists(&staging)?;
        remove_if_exists(&backup)?;
        fs::create_dir_all(&staging).map_err(|error| format!("创建暂存目录失败：{error}"))?;
        for skill in &prepared {
            copy_dir(&skill.path, &staging.join(&skill.name))?;
        }
        if final_dir.exists() {
            fs::rename(&final_dir, &backup).map_err(|error| format!("备份旧来源失败：{error}"))?;
        }
        if let Err(error) = fs::rename(&staging, &final_dir) {
            let _ = fs::rename(&backup, &final_dir);
            return Err(format!("安装来源镜像失败：{error}"));
        }

        let mut new_catalog = old_catalog.clone();
        if let Some(index) = new_catalog
            .sources
            .iter()
            .position(|item| item.source_safe == source_safe)
        {
            new_catalog.sources[index] = new_source.clone();
        } else {
            new_catalog.sources.push(new_source.clone());
        }
        progress.send(source, "catalog", "正在保存 catalog", Some(78));
        if let Err(error) = self.save_catalog(&new_catalog) {
            let _ = remove_if_exists(&final_dir);
            let _ = fs::rename(&backup, &final_dir);
            return Err(error);
        }
        progress.send(source, "global", "正在同步全局 Skills", Some(90));
        let managed = union_managed_names(&old_catalog, &new_catalog);
        if let Err(error) = self.sync_global(&new_catalog, &managed) {
            let _ = self.save_catalog(&old_catalog);
            let _ = remove_if_exists(&final_dir);
            let _ = fs::rename(&backup, &final_dir);
            let _ = self.sync_global(&old_catalog, &managed);
            return Err(error);
        }
        let _ = remove_if_exists(&backup);
        let new_names: BTreeSet<_> = new_source
            .skills
            .iter()
            .map(|item| item.name.as_str())
            .collect();
        let removed_count = old_source
            .map(|item| {
                item.skills
                    .iter()
                    .filter(|skill| !new_names.contains(skill.name.as_str()))
                    .count()
            })
            .unwrap_or(0);
        Ok(SourceSyncResult {
            source_safe: source_safe.to_string(),
            installed_count: new_source.skills.len(),
            removed_count,
            updated,
        })
    }

    fn set_global_enabled(
        &self,
        source_safe: &str,
        skill_name: &str,
        enabled: bool,
    ) -> Result<(), String> {
        let mut catalog = self.load_catalog_raw()?;
        let old_catalog = catalog.clone();
        let source = catalog
            .sources
            .iter_mut()
            .find(|item| item.source_safe == source_safe)
            .ok_or_else(|| format!("未找到来源 {source_safe}"))?;
        let skill = source
            .skills
            .iter_mut()
            .find(|item| item.name == skill_name)
            .ok_or_else(|| format!("未找到 Skill {skill_name}"))?;
        skill.global_enabled = enabled;
        self.save_catalog(&catalog)?;
        let managed = union_managed_names(&old_catalog, &catalog);
        if let Err(error) = self.sync_global(&catalog, &managed) {
            let _ = self.save_catalog(&old_catalog);
            let _ = self.sync_global(&old_catalog, &managed);
            return Err(error);
        }
        Ok(())
    }

    fn skill_detail(&self, source_safe: &str, skill_name: &str) -> Result<SkillDetail, String> {
        let catalog = self.load_catalog()?;
        let source = catalog
            .sources
            .iter()
            .find(|item| item.source_safe == source_safe)
            .ok_or_else(|| format!("未找到来源 {source_safe}"))?;
        let skill = source
            .skills
            .iter()
            .find(|item| item.name == skill_name)
            .ok_or_else(|| format!("未找到 Skill {skill_name}"))?;
        let path = self.repository_dir.join(source_safe).join(skill_name);
        let content = fs::read_to_string(path.join("SKILL.md"))
            .map_err(|error| format!("读取 {skill_name}/SKILL.md 失败：{error}"))?;
        let metadata = parse_skill_metadata(&content);
        Ok(SkillDetail {
            source: source.source.clone(),
            source_safe: source.source_safe.clone(),
            name: skill.name.clone(),
            description: skill.description.clone(),
            metadata_name: metadata.name.unwrap_or_else(|| skill.name.clone()),
            metadata_description: skill.description.clone(),
            global_enabled: skill.global_enabled,
            updated_at: skill.updated_at.clone(),
            path: path.to_string_lossy().into_owned(),
            content,
        })
    }

    fn resolve_skill_document(
        &self,
        source_safe: &str,
        skill_name: &str,
        relative_path: &str,
    ) -> Result<(PathBuf, PathBuf), String> {
        let catalog = self.load_catalog()?;
        let source = catalog
            .sources
            .iter()
            .find(|item| item.source_safe == source_safe)
            .ok_or_else(|| format!("未找到来源 {source_safe}"))?;
        source
            .skills
            .iter()
            .find(|item| item.name == skill_name)
            .ok_or_else(|| format!("未找到 Skill {skill_name}"))?;

        let relative = Path::new(relative_path);
        if relative.as_os_str().is_empty() || relative.is_absolute() {
            return Err("Markdown 文档路径无效".to_string());
        }
        if relative
            .extension()
            .and_then(|value| value.to_str())
            .is_none_or(|value| !value.eq_ignore_ascii_case("md"))
        {
            return Err("仅支持预览 Markdown 文档".to_string());
        }

        let source_root = fs::canonicalize(self.repository_dir.join(source_safe))
            .map_err(|error| format!("解析来源本地目录失败：{error}"))?;
        let document_path = fs::canonicalize(source_root.join(relative))
            .map_err(|error| format!("读取本地 Markdown 文档失败：{error}"))?;
        if !document_path.starts_with(&source_root) {
            return Err("拒绝读取来源目录之外的文档".to_string());
        }
        let metadata = fs::metadata(&document_path)
            .map_err(|error| format!("检查本地 Markdown 文档失败：{error}"))?;
        if !metadata.is_file() {
            return Err("Markdown 文档路径不是文件".to_string());
        }
        let normalized = document_path
            .strip_prefix(&source_root)
            .map_err(|_| "无法确定 Markdown 文档相对路径".to_string())?
            .to_path_buf();

        Ok((document_path, normalized))
    }

    fn skill_document(
        &self,
        source_safe: &str,
        skill_name: &str,
        relative_path: &str,
    ) -> Result<SkillDocument, String> {
        let (document_path, normalized) =
            self.resolve_skill_document(source_safe, skill_name, relative_path)?;
        let content = fs::read_to_string(&document_path)
            .map_err(|error| format!("读取本地 Markdown 文档失败：{error}"))?;

        Ok(SkillDocument {
            source_safe: source_safe.to_string(),
            skill_name: skill_name.to_string(),
            title: document_path
                .file_name()
                .and_then(|value| value.to_str())
                .unwrap_or("Markdown 文档")
                .to_string(),
            relative_path: normalized.to_string_lossy().into_owned(),
            content,
        })
    }

    fn remove_impact(&self, source_safe: &str) -> Result<RemoveSourceImpact, String> {
        let catalog = self.load_catalog()?;
        let source = catalog
            .sources
            .iter()
            .find(|item| item.source_safe == source_safe)
            .ok_or_else(|| format!("未找到来源 {source_safe}"))?;
        Ok(RemoveSourceImpact {
            installed_skills: source.skills.len(),
            global_enabled_skills: source
                .skills
                .iter()
                .filter(|skill| skill.global_enabled)
                .count(),
        })
    }

    fn remove_source(&self, source_safe: &str) -> Result<SourceSyncResult, String> {
        let old_catalog = self.load_catalog_raw()?;
        let index = old_catalog
            .sources
            .iter()
            .position(|item| item.source_safe == source_safe)
            .ok_or_else(|| format!("未找到来源 {source_safe}"))?;
        let removed_count = old_catalog.sources[index].skills.len();
        let mut new_catalog = old_catalog.clone();
        new_catalog.sources.remove(index);
        let final_dir = self.repository_dir.join(source_safe);
        let backup = self.repository_dir.join(format!("{source_safe}.bak"));
        remove_if_exists(&backup)?;
        if final_dir.exists() {
            fs::rename(&final_dir, &backup).map_err(|error| format!("备份来源失败：{error}"))?;
        }
        let managed = union_managed_names(&old_catalog, &new_catalog);
        if let Err(error) = self
            .save_catalog(&new_catalog)
            .and_then(|_| self.sync_global(&new_catalog, &managed))
        {
            let _ = self.save_catalog(&old_catalog);
            let _ = fs::rename(&backup, &final_dir);
            let _ = self.sync_global(&old_catalog, &managed);
            return Err(error);
        }
        let _ = remove_if_exists(&backup);
        Ok(SourceSyncResult {
            source_safe: source_safe.to_string(),
            installed_count: 0,
            removed_count,
            updated: false,
        })
    }

    fn sync_global(&self, catalog: &Catalog, managed: &BTreeSet<String>) -> Result<(), String> {
        let desired = desired_links(catalog, &self.repository_dir);
        sync_target_dir(&self.global_target_dir, &desired, managed)
    }
}

struct ProgressReporter {
    channel: Channel<OperationProgress>,
    operation: String,
    completed_sources: usize,
    total_sources: usize,
}

impl ProgressReporter {
    fn send(&self, source: &str, stage: &str, message: &str, percent: Option<u8>) {
        let _ = self.channel.send(OperationProgress {
            operation: self.operation.clone(),
            source: source.to_string(),
            stage: stage.to_string(),
            message: message.to_string(),
            percent,
            completed_sources: self.completed_sources,
            total_sources: self.total_sources,
        });
    }

    fn for_source(&self, completed_sources: usize) -> Self {
        Self {
            channel: self.channel.clone(),
            operation: self.operation.clone(),
            completed_sources,
            total_sources: self.total_sources,
        }
    }
}

#[tauri::command]
pub fn get_skills_dashboard() -> Result<SkillsDashboard, String> {
    let store = Store::new()?;
    let catalog = store.load_catalog()?;
    let enabled_content_chars = enabled_content_char_count(&store.repository_dir, &catalog);
    Ok(SkillsDashboard {
        catalog,
        repository_path: store.repository_dir.to_string_lossy().into_owned(),
        global_target_path: store.global_target_dir.to_string_lossy().into_owned(),
        npx_available: Command::new("npx").arg("--version").output().is_ok(),
        enabled_content_chars,
    })
}

fn enabled_content_char_count(repository_dir: &Path, catalog: &Catalog) -> usize {
    catalog
        .sources
        .iter()
        .flat_map(|source| {
            source
                .skills
                .iter()
                .filter(|skill| skill.global_enabled)
                .map(|skill| repository_dir.join(&source.source_safe).join(&skill.name))
        })
        .filter_map(|skill_dir| fs::read_to_string(skill_dir.join("SKILL.md")).ok())
        .map(|content| content.chars().count())
        .sum()
}

#[tauri::command]
pub async fn install_skill_source(
    source: String,
    on_progress: Channel<OperationProgress>,
) -> Result<SourceSyncResult, String> {
    run_source_task(source, "install", on_progress).await
}

#[tauri::command]
pub async fn update_skill_source(
    source: String,
    on_progress: Channel<OperationProgress>,
) -> Result<SourceSyncResult, String> {
    run_source_task(source, "update", on_progress).await
}

async fn run_source_task(
    source: String,
    operation: &str,
    channel: Channel<OperationProgress>,
) -> Result<SourceSyncResult, String> {
    let operation = operation.to_string();
    tauri::async_runtime::spawn_blocking(move || {
        let _guard = try_write_lock()?;
        let store = Store::new()?;
        let mut catalog = store.load_catalog_raw()?;
        if catalog_has_legacy_projects(&catalog) {
            store.migrate_legacy_projects(&mut catalog)?;
        }
        let reporter = ProgressReporter {
            channel,
            operation: operation.clone(),
            completed_sources: 0,
            total_sources: 1,
        };
        store.install_or_update(&source, &operation, &reporter)
    })
    .await
    .map_err(|error| format!("Skills 任务异常结束：{error}"))?
}

#[tauri::command]
pub async fn update_all_skill_sources(
    on_progress: Channel<OperationProgress>,
) -> Result<BatchUpdateResult, String> {
    tauri::async_runtime::spawn_blocking(move || {
        let _guard = try_write_lock()?;
        let store = Store::new()?;
        let mut catalog = store.load_catalog_raw()?;
        if catalog_has_legacy_projects(&catalog) {
            store.migrate_legacy_projects(&mut catalog)?;
        }
        let sources: Vec<String> = catalog
            .sources
            .iter()
            .map(|item| item.source.clone())
            .collect();
        let total = sources.len();
        let base = ProgressReporter {
            channel: on_progress,
            operation: "update-all".to_string(),
            completed_sources: 0,
            total_sources: total,
        };
        let mut result = BatchUpdateResult {
            total_sources: total,
            succeeded_sources: 0,
            failed_sources: 0,
            installed_count: 0,
            removed_count: 0,
            failures: Vec::new(),
        };
        for (index, source) in sources.iter().enumerate() {
            match store.install_or_update(source, "update", &base.for_source(index)) {
                Ok(sync) => {
                    result.succeeded_sources += 1;
                    result.installed_count += sync.installed_count;
                    result.removed_count += sync.removed_count;
                }
                Err(error) => {
                    result.failed_sources += 1;
                    result.failures.push(BatchUpdateFailure {
                        source: source.clone(),
                        error,
                    });
                }
            }
            base.for_source(index + 1).send(
                source,
                "source-complete",
                "当前来源处理完成",
                Some((((index + 1) * 100) / total.max(1)) as u8),
            );
        }
        Ok(result)
    })
    .await
    .map_err(|error| format!("全部更新任务异常结束：{error}"))?
}

#[tauri::command]
pub fn set_global_skill_enabled(
    source_safe: String,
    skill_name: String,
    enabled: bool,
) -> Result<(), String> {
    let _guard = try_write_lock()?;
    let store = Store::new()?;
    let mut catalog = store.load_catalog_raw()?;
    if catalog_has_legacy_projects(&catalog) {
        store.migrate_legacy_projects(&mut catalog)?;
    }
    store.set_global_enabled(&source_safe, &skill_name, enabled)
}

#[tauri::command]
pub fn get_skill_detail(source_safe: String, skill_name: String) -> Result<SkillDetail, String> {
    Store::new()?.skill_detail(&source_safe, &skill_name)
}

#[tauri::command]
pub fn get_skill_document(
    source_safe: String,
    skill_name: String,
    relative_path: String,
) -> Result<SkillDocument, String> {
    Store::new()?.skill_document(&source_safe, &skill_name, &relative_path)
}

#[tauri::command]
pub fn open_skill_document(
    app: tauri::AppHandle,
    source_safe: String,
    skill_name: String,
    relative_path: String,
) -> Result<(), String> {
    let store = Store::new()?;
    let (path, _) = store.resolve_skill_document(&source_safe, &skill_name, &relative_path)?;
    let display_path = path.to_string_lossy().into_owned();
    app.opener()
        .open_path(&display_path, None::<&str>)
        .map_err(|error| format!("使用系统默认应用打开文档 {display_path} 失败：{error}"))
}

#[tauri::command]
pub fn open_skill_folder(
    app: tauri::AppHandle,
    source_safe: String,
    skill_name: String,
) -> Result<(), String> {
    let detail = Store::new()?.skill_detail(&source_safe, &skill_name)?;
    app.opener()
        .open_path(&detail.path, None::<&str>)
        .map_err(|error| format!("打开 Skill 文件夹 {} 失败：{error}", detail.path))
}

#[tauri::command]
pub fn open_global_skills_folder(app: tauri::AppHandle) -> Result<(), String> {
    let store = Store::new()?;
    fs::create_dir_all(&store.global_target_dir)
        .map_err(|error| format!("创建全局 Skills 文件夹失败：{error}"))?;
    let path = store.global_target_dir.to_string_lossy().into_owned();
    app.opener()
        .open_path(&path, None::<&str>)
        .map_err(|error| format!("打开全局 Skills 文件夹 {path} 失败：{error}"))
}

#[tauri::command]
pub fn get_remove_source_impact(source_safe: String) -> Result<RemoveSourceImpact, String> {
    Store::new()?.remove_impact(&source_safe)
}

#[tauri::command]
pub fn remove_skill_source(source_safe: String) -> Result<SourceSyncResult, String> {
    let _guard = try_write_lock()?;
    let store = Store::new()?;
    let mut catalog = store.load_catalog_raw()?;
    if catalog_has_legacy_projects(&catalog) {
        store.migrate_legacy_projects(&mut catalog)?;
    }
    store.remove_source(&source_safe)
}

fn try_write_lock() -> Result<MutexGuard<'static, ()>, String> {
    WRITE_LOCK
        .get_or_init(|| Mutex::new(()))
        .try_lock()
        .map_err(|_| "另一个 Skills 写入任务正在运行，请稍后再试".to_string())
}

fn validate_source(source: &str) -> Result<(), String> {
    let parts: Vec<&str> = source.trim().split('/').collect();
    let valid_part = |part: &str| {
        !part.is_empty()
            && part
                .chars()
                .all(|character| character.is_ascii_alphanumeric() || "._-".contains(character))
    };
    if parts.len() == 2 && parts.iter().all(|part| valid_part(part)) {
        Ok(())
    } else {
        Err("来源格式无效，请输入 owner/repo".to_string())
    }
}

fn home_dir() -> Result<PathBuf, String> {
    env::var_os("HOME")
        .or_else(|| env::var_os("USERPROFILE"))
        .map(PathBuf::from)
        .ok_or_else(|| "无法确定用户主目录".to_string())
}

fn source_safe_name(source: &str) -> String {
    source.trim().replace('/', "_")
}

fn activation_name(source_safe: &str, skill_name: &str) -> String {
    format!("{source_safe}__{skill_name}")
}

fn timestamp() -> String {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
        .to_string()
}

fn run_skills_command(workdir: &Path, args: &[&str]) -> Result<(), String> {
    let output = Command::new("npx")
        .arg("--yes")
        .arg("skills")
        .args(args)
        .current_dir(workdir)
        .output()
        .map_err(|error| format!("运行 npx skills 失败：{error}"))?;
    output
        .status
        .success()
        .then_some(())
        .ok_or_else(|| command_error("安装 Skills 来源", &output))
}

fn command_error(action: &str, output: &std::process::Output) -> String {
    let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
    let stdout = String::from_utf8_lossy(&output.stdout).trim().to_string();
    let detail = if stderr.is_empty() { stdout } else { stderr };
    if detail.is_empty() {
        format!("{action}失败，退出状态：{}", output.status)
    } else {
        format!("{action}失败：{detail}")
    }
}

fn catalog_has_legacy_projects(catalog: &Catalog) -> bool {
    catalog.sources.iter().any(|source| {
        source
            .skills
            .iter()
            .any(|skill| !skill.legacy_project_paths.is_empty())
    })
}

fn normalize_catalog(catalog: &mut Catalog) {
    for source in &mut catalog.sources {
        source.source = source.source.trim().to_string();
        source.source_safe = source.source_safe.trim().to_string();
        for skill in &mut source.skills {
            skill.name = skill.name.trim().to_string();
            skill.description = skill.description.trim().to_string();
            skill.legacy_project_paths.sort();
            skill.legacy_project_paths.dedup();
        }
        source.skills.sort_by(|a, b| a.name.cmp(&b.name));
    }
    catalog.sources.sort_by(|a, b| a.source.cmp(&b.source));
}

#[derive(Debug, Default, Deserialize, PartialEq, Eq)]
struct SkillMetadata {
    name: Option<String>,
    description: Option<String>,
}

fn read_skill_description(skill_dir: &Path) -> String {
    let Ok(content) = fs::read_to_string(skill_dir.join("SKILL.md")) else {
        return String::new();
    };
    parse_skill_metadata(&content)
        .description
        .unwrap_or_default()
}

fn parse_skill_metadata(content: &str) -> SkillMetadata {
    let content = content.trim_start_matches('\u{feff}').replace("\r\n", "\n");
    let mut lines = content.lines();
    if lines.next().map(str::trim) != Some("---") {
        return SkillMetadata::default();
    }
    let mut frontmatter = Vec::new();
    for line in lines {
        if line.trim() == "---" {
            break;
        }
        frontmatter.push(line);
    }
    let Ok(mut metadata) = serde_yaml::from_str::<SkillMetadata>(&frontmatter.join("\n")) else {
        return SkillMetadata::default();
    };
    metadata.name = metadata.name.map(|value| value.trim().to_string());
    metadata.description = metadata.description.map(|value| value.trim().to_string());
    metadata
}

fn copy_dir(source: &Path, target: &Path) -> Result<(), String> {
    fs::create_dir_all(target)
        .map_err(|error| format!("创建目录 {} 失败：{error}", target.display()))?;
    for entry in
        fs::read_dir(source).map_err(|error| format!("读取 {} 失败：{error}", source.display()))?
    {
        let entry = entry.map_err(|error| format!("读取目录项失败：{error}"))?;
        let file_type = entry
            .file_type()
            .map_err(|error| format!("读取文件类型失败：{error}"))?;
        let destination = target.join(entry.file_name());
        if file_type.is_dir() {
            copy_dir(&entry.path(), &destination)?;
        } else if file_type.is_symlink() {
            let resolved =
                fs::canonicalize(entry.path()).map_err(|error| format!("解析软链失败：{error}"))?;
            if resolved.is_dir() {
                copy_dir(&resolved, &destination)?;
            } else {
                fs::copy(resolved, destination)
                    .map_err(|error| format!("复制文件失败：{error}"))?;
            }
        } else {
            fs::copy(entry.path(), destination)
                .map_err(|error| format!("复制文件失败：{error}"))?;
        }
    }
    Ok(())
}

fn remove_if_exists(path: &Path) -> Result<(), String> {
    match fs::symlink_metadata(path) {
        Ok(metadata) if metadata.is_dir() && !metadata.file_type().is_symlink() => {
            fs::remove_dir_all(path)
        }
        Ok(_) => fs::remove_file(path),
        Err(error) if error.kind() == io::ErrorKind::NotFound => return Ok(()),
        Err(error) => return Err(format!("检查 {} 失败：{error}", path.display())),
    }
    .map_err(|error| format!("删除 {} 失败：{error}", path.display()))
}

fn managed_names(catalog: &Catalog) -> BTreeSet<String> {
    catalog
        .sources
        .iter()
        .flat_map(|source| {
            source
                .skills
                .iter()
                .map(|skill| activation_name(&source.source_safe, &skill.name))
        })
        .collect()
}

fn union_managed_names(a: &Catalog, b: &Catalog) -> BTreeSet<String> {
    managed_names(a).union(&managed_names(b)).cloned().collect()
}

fn desired_links(catalog: &Catalog, repository: &Path) -> BTreeMap<String, PathBuf> {
    catalog
        .sources
        .iter()
        .flat_map(|source| {
            source
                .skills
                .iter()
                .filter(|skill| skill.global_enabled)
                .map(|skill| {
                    (
                        activation_name(&source.source_safe, &skill.name),
                        repository.join(&source.source_safe).join(&skill.name),
                    )
                })
        })
        .collect()
}

fn sync_target_dir(
    target_dir: &Path,
    desired: &BTreeMap<String, PathBuf>,
    managed: &BTreeSet<String>,
) -> Result<(), String> {
    fs::create_dir_all(target_dir)
        .map_err(|error| format!("创建激活目录 {} 失败：{error}", target_dir.display()))?;
    for entry in fs::read_dir(target_dir).map_err(|error| format!("读取激活目录失败：{error}"))?
    {
        let entry = entry.map_err(|error| format!("读取激活项失败：{error}"))?;
        let name = entry.file_name().to_string_lossy().into_owned();
        if managed.contains(&name) && !desired.contains_key(&name) {
            remove_if_exists(&entry.path())?;
        }
    }
    for (name, target) in desired {
        let link = target_dir.join(name);
        if let Ok(metadata) = fs::symlink_metadata(&link) {
            if !metadata.file_type().is_symlink() {
                return Err(format!("受管路径 {} 已存在且不是软链", link.display()));
            }
            if fs::read_link(&link).ok().as_deref() == Some(target.as_path()) {
                continue;
            }
            fs::remove_file(&link).map_err(|error| format!("替换软链失败：{error}"))?;
        }
        create_symlink(target, &link)?;
    }
    Ok(())
}

#[cfg(unix)]
fn create_symlink(target: &Path, link: &Path) -> Result<(), String> {
    std::os::unix::fs::symlink(target, link).map_err(|error| format!("创建软链失败：{error}"))
}

#[cfg(windows)]
fn create_symlink(target: &Path, link: &Path) -> Result<(), String> {
    std::os::windows::fs::symlink_dir(target, link)
        .map_err(|error| format!("创建目录软链失败：{error}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_store(root: &Path) -> Store {
        Store::from_root(root.join(".agent-manager")).expect("create test store")
    }

    fn skill(name: &str, global_enabled: bool) -> SkillRecord {
        SkillRecord {
            name: name.into(),
            description: String::new(),
            global_enabled,
            legacy_project_paths: Vec::new(),
            installed_at: String::new(),
            updated_at: String::new(),
        }
    }

    #[test]
    fn validates_source_format() {
        assert!(validate_source("mattpocock/skills").is_ok());
        assert!(validate_source("https://github.com/mattpocock/skills").is_err());
        assert_eq!(source_safe_name("mattpocock/skills"), "mattpocock_skills");
    }

    #[test]
    fn counts_content_characters_for_enabled_skills_only() {
        let root = env::temp_dir().join(format!("agent-manager-char-count-{}", timestamp()));
        let repository = root.join("skills");
        let source_dir = repository.join("owner_repo");
        fs::create_dir_all(source_dir.join("enabled")).expect("create enabled skill");
        fs::create_dir_all(source_dir.join("disabled")).expect("create disabled skill");
        fs::write(source_dir.join("enabled/SKILL.md"), "hello\n中文").expect("write enabled");
        fs::write(source_dir.join("disabled/SKILL.md"), "not counted").expect("write disabled");
        let catalog = Catalog {
            sources: vec![SourceRecord {
                source: "owner/repo".into(),
                source_safe: "owner_repo".into(),
                installed_at: String::new(),
                updated_at: String::new(),
                skills: vec![skill("enabled", true), skill("disabled", false)],
            }],
        };

        assert_eq!(enabled_content_char_count(&repository, &catalog), 8);
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn parses_skill_frontmatter_metadata() {
        let root = env::temp_dir().join(format!("agent-manager-test-{}", timestamp()));
        fs::create_dir_all(&root).expect("create test dir");
        let content = "---\nname: buddy-sings\ndescription: >\n  Use when user wants their Claude Code pet (/buddy) to sing a song. Triggers on any\n  request that combines the concept of their Claude Code buddy, pet, or companion with\n  singing or music.\nlicense: MIT\nmetadata:\n  version: \"1.1\"\n  category: creative\n---\n# Skill\n";
        fs::write(root.join("SKILL.md"), content).expect("write skill");
        assert_eq!(
            parse_skill_metadata(content),
            SkillMetadata {
                name: Some("buddy-sings".into()),
                description: Some("Use when user wants their Claude Code pet (/buddy) to sing a song. Triggers on any request that combines the concept of their Claude Code buddy, pet, or companion with singing or music.".into()),
            }
        );
        assert_eq!(
            read_skill_description(&root),
            "Use when user wants their Claude Code pet (/buddy) to sing a song. Triggers on any request that combines the concept of their Claude Code buddy, pet, or companion with singing or music."
        );
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn global_toggle_creates_and_removes_link() {
        let root = env::temp_dir().join(format!("agent-manager-toggle-{}", timestamp()));
        let store = test_store(&root);
        let skill_dir = store.repository_dir.join("owner_repo").join("demo");
        fs::create_dir_all(&skill_dir).expect("create skill");
        let catalog = Catalog {
            sources: vec![SourceRecord {
                source: "owner/repo".into(),
                source_safe: "owner_repo".into(),
                installed_at: String::new(),
                updated_at: String::new(),
                skills: vec![skill("demo", false)],
            }],
        };
        store.save_catalog(&catalog).expect("save catalog");
        store
            .set_global_enabled("owner_repo", "demo", true)
            .expect("enable");
        assert_eq!(
            fs::read_link(store.global_target_dir.join("owner_repo__demo")).expect("link"),
            skill_dir
        );
        store
            .set_global_enabled("owner_repo", "demo", false)
            .expect("disable");
        assert!(!store.global_target_dir.join("owner_repo__demo").exists());
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn previews_only_local_markdown_inside_registered_source() {
        let root = env::temp_dir().join(format!("agent-manager-document-{}", timestamp()));
        let store = test_store(&root);
        let source_dir = store.repository_dir.join("owner_repo");
        let skill_dir = source_dir.join("demo");
        fs::create_dir_all(skill_dir.join("references")).expect("create document dirs");
        fs::write(skill_dir.join("SKILL.md"), "# Demo").expect("write skill");
        fs::write(skill_dir.join("references/guide.md"), "# Guide").expect("write guide");
        fs::write(
            store
                .repository_dir
                .parent()
                .expect("manager root")
                .join("outside.md"),
            "# Outside",
        )
        .expect("write outside");
        store
            .save_catalog(&Catalog {
                sources: vec![SourceRecord {
                    source: "owner/repo".into(),
                    source_safe: "owner_repo".into(),
                    installed_at: String::new(),
                    updated_at: String::new(),
                    skills: vec![skill("demo", false)],
                }],
            })
            .expect("save catalog");

        let document = store
            .skill_document("owner_repo", "demo", "demo/references/guide.md")
            .expect("read local document");
        assert_eq!(document.title, "guide.md");
        assert_eq!(document.relative_path, "demo/references/guide.md");
        assert_eq!(document.content, "# Guide");
        assert!(store
            .skill_document("owner_repo", "demo", "../../outside.md")
            .is_err());
        assert!(store
            .skill_document("owner_repo", "demo", "demo/SKILL.txt")
            .is_err());
        assert!(store
            .skill_document("owner_repo", "missing", "demo/SKILL.md")
            .is_err());
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn migrates_only_matching_legacy_project_link() {
        let root = env::temp_dir().join(format!("agent-manager-migrate-{}", timestamp()));
        let store = test_store(&root);
        let project = root.join("project");
        let target = store.repository_dir.join("owner_repo").join("demo");
        fs::create_dir_all(&target).expect("create target");
        let link_dir = project.join(".agents").join("skills");
        fs::create_dir_all(&link_dir).expect("create link dir");
        create_symlink(&target, &link_dir.join("owner_repo__demo")).expect("create link");
        let mut record = skill("demo", false);
        record.legacy_project_paths = vec![project.to_string_lossy().into_owned()];
        let mut catalog = Catalog {
            sources: vec![SourceRecord {
                source: "owner/repo".into(),
                source_safe: "owner_repo".into(),
                installed_at: String::new(),
                updated_at: String::new(),
                skills: vec![record],
            }],
        };
        store
            .migrate_legacy_projects(&mut catalog)
            .expect("migrate");
        assert!(!link_dir.join("owner_repo__demo").exists());
        let saved = store.load_catalog_raw().expect("catalog");
        assert!(saved.sources[0].skills[0].legacy_project_paths.is_empty());
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn migrates_json_catalog_to_sqlite_and_removes_legacy_file() {
        let root = env::temp_dir().join(format!("agent-manager-json-migrate-{}", timestamp()));
        let manager_root = root.join(".agent-manager");
        fs::create_dir_all(&manager_root).expect("create root");
        let legacy_path = manager_root.join("install-skills.json");
        let catalog = Catalog {
            sources: vec![SourceRecord {
                source: "owner/repo".into(),
                source_safe: "owner_repo".into(),
                installed_at: "100".into(),
                updated_at: "200".into(),
                skills: vec![SkillRecord {
                    name: "demo".into(),
                    description: "Demo skill".into(),
                    global_enabled: true,
                    legacy_project_paths: Vec::new(),
                    installed_at: "100".into(),
                    updated_at: "200".into(),
                }],
            }],
        };
        fs::write(
            &legacy_path,
            serde_json::to_vec_pretty(&catalog).expect("serialize catalog"),
        )
        .expect("write legacy catalog");

        let store = Store::from_root(manager_root).expect("migrate store");
        let migrated = store.load_catalog_raw().expect("load migrated catalog");
        assert!(!legacy_path.exists());
        assert!(store.database.path().exists());
        assert_eq!(migrated.sources.len(), 1);
        assert_eq!(migrated.sources[0].source, "owner/repo");
        assert_eq!(migrated.sources[0].skills.len(), 1);
        assert_eq!(migrated.sources[0].skills[0].name, "demo");
        assert!(migrated.sources[0].skills[0].global_enabled);
        let _ = fs::remove_dir_all(root);
    }
}
