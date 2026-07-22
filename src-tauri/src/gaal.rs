use serde::Serialize;
use sha2::{Digest, Sha256};
use std::{
    env, fs,
    io::Write,
    path::{Path, PathBuf},
    process::{Command, Output},
};

const LATEST_DOWNLOAD_ROOT: &str = "https://github.com/getgaal/gaal/releases/latest/download";
const MAX_BINARY_BYTES: usize = 64 * 1024 * 1024;

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GaalInfo {
    pub installed: bool,
    pub path: String,
    pub directory: String,
    pub version: String,
}

pub fn managed_binary_path() -> Result<PathBuf, String> {
    let name = if cfg!(windows) { "gaal.exe" } else { "gaal" };
    Ok(home_dir()?.join(".agent-manager").join("bin").join(name))
}

pub fn inspect() -> Result<GaalInfo, String> {
    inspect_path(&managed_binary_path()?)
}

fn inspect_path(path: &Path) -> Result<GaalInfo, String> {
    let directory = path.parent().unwrap_or(path).to_string_lossy().into_owned();
    if !path.is_file() {
        return Ok(GaalInfo {
            installed: false,
            path: path.to_string_lossy().into_owned(),
            directory,
            version: String::new(),
        });
    }
    let output = Command::new(path)
        .arg("version")
        .output()
        .map_err(|error| format!("读取 GAAL 版本失败：{error}"))?;
    if !output.status.success() {
        return Err(format!(
            "读取 GAAL 版本失败：{}",
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }
    let version = String::from_utf8_lossy(&output.stdout)
        .lines()
        .find(|line| !line.trim().is_empty())
        .unwrap_or_default()
        .trim()
        .to_string();
    Ok(GaalInfo {
        installed: true,
        path: path.to_string_lossy().into_owned(),
        directory,
        version,
    })
}

#[tauri::command]
pub async fn get_gaal_info() -> Result<GaalInfo, String> {
    tauri::async_runtime::spawn_blocking(inspect)
        .await
        .map_err(|error| format!("读取 GAAL 信息任务异常结束：{error}"))?
}

#[tauri::command]
pub async fn install_gaal() -> Result<GaalInfo, String> {
    tauri::async_runtime::spawn_blocking(install)
        .await
        .map_err(|error| format!("安装 GAAL 任务异常结束：{error}"))?
}

fn install() -> Result<GaalInfo, String> {
    let asset_name = release_asset_name()?;
    let target = managed_binary_path()?;
    let parent = target
        .parent()
        .ok_or_else(|| "GAAL 安装路径缺少父目录".to_string())?;
    fs::create_dir_all(parent)
        .map_err(|error| format!("创建 GAAL 安装目录 {} 失败：{error}", parent.display()))?;
    let checksum_url = format!("{LATEST_DOWNLOAD_ROOT}/SHA256SUMS");
    let asset_url = format!("{LATEST_DOWNLOAD_ROOT}/{asset_name}");
    let checksum_bytes = download_with_system_client(&checksum_url, "GAAL 校验文件")?;
    let checksum_text = String::from_utf8(checksum_bytes)
        .map_err(|error| format!("读取 GAAL 校验文件失败：{error}"))?;
    let expected = checksum_text
        .lines()
        .find_map(|line| {
            let mut parts = line.split_whitespace();
            let checksum = parts.next()?;
            let name = parts.next()?.trim_start_matches('*');
            (name == asset_name).then(|| checksum.to_ascii_lowercase())
        })
        .ok_or_else(|| format!("SHA256SUMS 中没有 {asset_name}"))?;
    let bytes = download_with_system_client(&asset_url, "GAAL 二进制")?;
    if bytes.len() > MAX_BINARY_BYTES {
        return Err("GAAL 二进制超过 64 MiB 安全限制".to_string());
    }
    let actual = format!("{:x}", Sha256::digest(&bytes));
    if actual != expected {
        return Err(format!(
            "GAAL 二进制校验失败：期望 {expected}，实际 {actual}"
        ));
    }

    let temporary = parent.join(format!(".gaal.{}.tmp", std::process::id()));
    let result = (|| {
        let mut file = fs::File::create(&temporary)
            .map_err(|error| format!("创建 GAAL 临时文件失败：{error}"))?;
        file.write_all(&bytes)
            .map_err(|error| format!("写入 GAAL 临时文件失败：{error}"))?;
        file.sync_all()
            .map_err(|error| format!("同步 GAAL 临时文件失败：{error}"))?;
        set_executable(&temporary)?;
        fs::rename(&temporary, &target)
            .map_err(|error| format!("原子安装 GAAL 到 {} 失败：{error}", target.display()))?;
        inspect_path(&target)
    })();
    if result.is_err() {
        let _ = fs::remove_file(&temporary);
    }
    result
}

fn download_with_system_client(url: &str, label: &str) -> Result<Vec<u8>, String> {
    let output = run_curl(url).or_else(|curl_error| {
        run_powershell(url).map_err(|powershell_error| {
            format!("curl：{curl_error}；PowerShell：{powershell_error}")
        })
    });
    let output = output.map_err(|error| format!("下载 {label} 失败：{error}"))?;
    if !output.status.success() {
        return Err(format!(
            "下载 {label} 失败：{}",
            command_error_detail(&output)
        ));
    }
    Ok(output.stdout)
}

fn run_curl(url: &str) -> Result<Output, String> {
    Command::new("curl")
        .args([
            "--fail",
            "--location",
            "--silent",
            "--show-error",
            "--connect-timeout",
            "20",
            "--max-time",
            "180",
            "--user-agent",
            "agent-manager",
            url,
        ])
        .output()
        .map_err(|error| format!("无法启动 curl：{error}"))
}

#[cfg(windows)]
fn run_powershell(url: &str) -> Result<Output, String> {
    Command::new("powershell.exe")
        .args([
            "-NoProfile",
            "-NonInteractive",
            "-Command",
            "$ProgressPreference='SilentlyContinue'; (Invoke-WebRequest -UseBasicParsing -Uri $args[0]).Content",
            url,
        ])
        .output()
        .map_err(|error| format!("无法启动 PowerShell：{error}"))
}

#[cfg(not(windows))]
fn run_powershell(_url: &str) -> Result<Output, String> {
    Err("当前系统不使用 PowerShell 下载".to_string())
}

fn command_error_detail(output: &Output) -> String {
    let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
    if !stderr.is_empty() {
        return stderr;
    }
    let stdout = String::from_utf8_lossy(&output.stdout).trim().to_string();
    if stdout.is_empty() {
        format!("进程退出码 {}", output.status)
    } else {
        stdout
    }
}

fn release_asset_name() -> Result<String, String> {
    let os = match env::consts::OS {
        "macos" => "darwin",
        "linux" => "linux",
        "windows" => "windows",
        other => return Err(format!("当前操作系统 {other} 不支持自动安装 GAAL")),
    };
    let arch = match env::consts::ARCH {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        other => return Err(format!("当前处理器架构 {other} 不支持自动安装 GAAL")),
    };
    let suffix = if os == "windows" { ".exe" } else { "" };
    Ok(format!("gaal-{os}-{arch}{suffix}"))
}

#[cfg(unix)]
fn set_executable(path: &Path) -> Result<(), String> {
    use std::os::unix::fs::PermissionsExt;
    let mut permissions = fs::metadata(path)
        .map_err(|error| format!("读取 GAAL 文件权限失败：{error}"))?
        .permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(path, permissions)
        .map_err(|error| format!("设置 GAAL 可执行权限失败：{error}"))
}

#[cfg(windows)]
fn set_executable(_path: &Path) -> Result<(), String> {
    Ok(())
}

fn home_dir() -> Result<PathBuf, String> {
    env::var_os("HOME")
        .or_else(|| env::var_os("USERPROFILE"))
        .map(PathBuf::from)
        .ok_or_else(|| "无法确定用户主目录".to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn maps_supported_platform_asset() {
        let asset = release_asset_name().expect("supported test platform");
        assert!(asset.starts_with("gaal-"));
        assert!(asset.contains("amd64") || asset.contains("arm64"));
    }

    #[test]
    #[ignore = "downloads the latest GAAL release from GitHub"]
    fn installs_latest_release_end_to_end() {
        let info = install().expect("install latest GAAL release");
        assert!(info.installed);
        assert!(!info.version.is_empty());
        assert!(Path::new(&info.path).is_file());
    }
}
