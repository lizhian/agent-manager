use std::{
    env,
    ffi::{OsStr, OsString},
    process::Command,
    sync::OnceLock,
};

#[cfg(unix)]
use std::path::PathBuf;

static USER_PATH: OnceLock<Option<OsString>> = OnceLock::new();

pub(crate) fn user_command(program: impl AsRef<OsStr>) -> Command {
    let mut command = Command::new(program);
    if let Some(path) = USER_PATH.get_or_init(resolve_user_path) {
        command.env("PATH", path);
    }
    command
}

fn resolve_user_path() -> Option<OsString> {
    let inherited_path = env::var_os("PATH");
    #[cfg(unix)]
    if let Some(shell_path) = user_shell_path() {
        return merge_paths(&shell_path, inherited_path.as_deref());
    }
    inherited_path
}

#[cfg(unix)]
fn user_shell_path() -> Option<OsString> {
    const START: &str = "__AGENT_MANAGER_PATH_START__";
    const END: &str = "__AGENT_MANAGER_PATH_END__";
    const PRINT_PATH: &str =
        "printf '__AGENT_MANAGER_PATH_START__%s__AGENT_MANAGER_PATH_END__' \"$PATH\"";

    let shell = configured_shell()?;
    let output = Command::new(shell)
        .args(["-ilc", PRINT_PATH])
        .output()
        .ok()?;
    if !output.status.success() {
        return None;
    }
    let stdout = String::from_utf8(output.stdout).ok()?;
    let start = stdout.rfind(START)? + START.len();
    let end = stdout[start..].find(END)? + start;
    let path = stdout[start..end].trim();
    (!path.is_empty()).then(|| OsString::from(path))
}

#[cfg(unix)]
fn configured_shell() -> Option<PathBuf> {
    env::var_os("SHELL")
        .map(PathBuf::from)
        .filter(|path| path.is_absolute() && path.is_file())
        .or_else(default_shell)
}

#[cfg(all(unix, target_os = "macos"))]
fn default_shell() -> Option<PathBuf> {
    let shell = PathBuf::from("/bin/zsh");
    shell.is_file().then_some(shell)
}

#[cfg(all(unix, not(target_os = "macos")))]
fn default_shell() -> Option<PathBuf> {
    let shell = PathBuf::from("/bin/sh");
    shell.is_file().then_some(shell)
}

#[cfg(unix)]
fn merge_paths(shell_path: &OsStr, inherited_path: Option<&OsStr>) -> Option<OsString> {
    let mut paths = Vec::<PathBuf>::new();
    for path in
        env::split_paths(shell_path).chain(inherited_path.into_iter().flat_map(env::split_paths))
    {
        if !paths.contains(&path) {
            paths.push(path);
        }
    }
    env::join_paths(paths).ok()
}
