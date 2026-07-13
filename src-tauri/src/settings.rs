use crate::database::Database;
use font_kit::source::SystemSource;
use serde::{Deserialize, Serialize};
use std::{
    env,
    path::PathBuf,
    time::{SystemTime, UNIX_EPOCH},
};

const FONT_SIZE_KEY: &str = "appearance.font_size_v3";
const FONT_FAMILY_KEY: &str = "appearance.font_family";
const DOCUMENT_PREVIEW_LAYOUT_KEY: &str = "preview.document_layout";

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "kebab-case")]
pub enum FontSize {
    ExtraSmall,
    Small,
    #[default]
    Standard,
    Large,
    ExtraLarge,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "kebab-case")]
pub enum DocumentPreviewPosition {
    Right,
    #[default]
    Bottom,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct DocumentPreviewLayout {
    position: DocumentPreviewPosition,
    ratio: f64,
}

impl Default for DocumentPreviewLayout {
    fn default() -> Self {
        Self {
            position: DocumentPreviewPosition::Bottom,
            ratio: 0.5,
        }
    }
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AppSettings {
    pub font_size: FontSize,
    pub font_family: String,
    pub document_preview_position: DocumentPreviewPosition,
    pub document_preview_ratio: f64,
}

struct Store {
    database: Database,
}

impl Store {
    fn new() -> Result<Self, String> {
        let home = home_dir()?;
        Self::from_root(home.join(".agent-manager"))
    }

    fn from_root(root: PathBuf) -> Result<Self, String> {
        Ok(Self {
            database: Database::new(root.join("agent-manager.db"))?,
        })
    }

    fn load(&self) -> Result<AppSettings, String> {
        let font_size = self
            .database
            .load_app_setting(FONT_SIZE_KEY)?
            .map(|value| {
                serde_json::from_str(&value)
                    .map_err(|error| format!("解析字体大小设置失败：{error}"))
            })
            .transpose()?
            .unwrap_or_default();
        let font_family = self
            .database
            .load_app_setting(FONT_FAMILY_KEY)?
            .map(|value| {
                serde_json::from_str(&value).map_err(|error| format!("解析字体设置失败：{error}"))
            })
            .transpose()?
            .unwrap_or_default();
        let document_preview_layout = self
            .database
            .load_app_setting(DOCUMENT_PREVIEW_LAYOUT_KEY)?
            .map(|value| {
                serde_json::from_str::<DocumentPreviewLayout>(&value)
                    .map_err(|error| format!("解析文档预览布局设置失败：{error}"))
            })
            .transpose()?
            .filter(|layout| valid_preview_ratio(layout.ratio))
            .unwrap_or_default();
        Ok(AppSettings {
            font_size,
            font_family,
            document_preview_position: document_preview_layout.position,
            document_preview_ratio: document_preview_layout.ratio,
        })
    }

    fn set_font_size(&self, font_size: FontSize) -> Result<AppSettings, String> {
        let value = serde_json::to_string(&font_size)
            .map_err(|error| format!("序列化字体大小设置失败：{error}"))?;
        self.database
            .save_app_setting(FONT_SIZE_KEY, &value, &timestamp())?;
        self.load()
    }

    fn set_font_family(&self, font_family: String) -> Result<AppSettings, String> {
        validate_font_family(&font_family)?;
        let value = serde_json::to_string(font_family.trim())
            .map_err(|error| format!("序列化字体设置失败：{error}"))?;
        self.database
            .save_app_setting(FONT_FAMILY_KEY, &value, &timestamp())?;
        self.load()
    }

    fn set_document_preview_layout(
        &self,
        position: DocumentPreviewPosition,
        ratio: f64,
    ) -> Result<AppSettings, String> {
        if !valid_preview_ratio(ratio) {
            return Err("文档预览占用比例必须在 20% 到 80% 之间".to_string());
        }
        let value = serde_json::to_string(&DocumentPreviewLayout { position, ratio })
            .map_err(|error| format!("序列化文档预览布局设置失败：{error}"))?;
        self.database
            .save_app_setting(DOCUMENT_PREVIEW_LAYOUT_KEY, &value, &timestamp())?;
        self.load()
    }
}

#[tauri::command]
pub fn get_app_settings() -> Result<AppSettings, String> {
    Store::new()?.load()
}

#[tauri::command]
pub fn set_font_size(font_size: FontSize) -> Result<AppSettings, String> {
    Store::new()?.set_font_size(font_size)
}

#[tauri::command]
pub fn set_font_family(font_family: String) -> Result<AppSettings, String> {
    Store::new()?.set_font_family(font_family)
}

#[tauri::command]
pub fn set_document_preview_layout(
    position: DocumentPreviewPosition,
    ratio: f64,
) -> Result<AppSettings, String> {
    Store::new()?.set_document_preview_layout(position, ratio)
}

#[tauri::command]
pub async fn get_system_fonts() -> Result<Vec<String>, String> {
    tauri::async_runtime::spawn_blocking(list_system_fonts)
        .await
        .map_err(|error| format!("读取系统字体任务异常结束：{error}"))?
}

fn list_system_fonts() -> Result<Vec<String>, String> {
    let mut families = SystemSource::new()
        .all_families()
        .map_err(|error| format!("读取系统字体失败：{error}"))?;
    families.sort_by_key(|family| family.to_lowercase());
    families.dedup_by(|left, right| left.eq_ignore_ascii_case(right));
    Ok(families)
}

fn validate_font_family(font_family: &str) -> Result<(), String> {
    let font_family = font_family.trim();
    if font_family.chars().count() > 128 {
        return Err("字体名称不能超过 128 个字符".to_string());
    }
    if font_family.chars().any(char::is_control) {
        return Err("字体名称包含无效字符".to_string());
    }
    Ok(())
}

fn valid_preview_ratio(ratio: f64) -> bool {
    ratio.is_finite() && (0.2..=0.8).contains(&ratio)
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn persists_font_size_setting() {
        let root = env::temp_dir().join(format!(
            "agent-manager-settings-{}",
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("time")
                .as_nanos()
        ));
        let store = Store::from_root(root.join(".agent-manager")).expect("store");
        assert_eq!(
            store.load().expect("defaults").font_size,
            FontSize::Standard
        );
        assert_eq!(store.load().expect("defaults").font_family, "");
        assert_eq!(
            store.load().expect("defaults").document_preview_position,
            DocumentPreviewPosition::Bottom
        );
        assert_eq!(store.load().expect("defaults").document_preview_ratio, 0.5);
        store
            .set_font_size(FontSize::ExtraLarge)
            .expect("save font size");
        assert_eq!(
            store.load().expect("saved settings").font_size,
            FontSize::ExtraLarge
        );
        store
            .set_font_family("Example Sans".into())
            .expect("save font family");
        assert_eq!(
            store.load().expect("saved settings").font_family,
            "Example Sans"
        );
        store
            .set_document_preview_layout(DocumentPreviewPosition::Right, 0.62)
            .expect("save document preview layout");
        let saved = store.load().expect("saved settings");
        assert_eq!(
            saved.document_preview_position,
            DocumentPreviewPosition::Right
        );
        assert_eq!(saved.document_preview_ratio, 0.62);
        assert!(store
            .set_document_preview_layout(DocumentPreviewPosition::Bottom, 0.9)
            .is_err());
        let _ = std::fs::remove_dir_all(root);
    }
}
