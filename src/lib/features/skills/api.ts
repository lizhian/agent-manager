import { Channel, invoke } from "@tauri-apps/api/core";
import type {
  BatchUpdateResult,
  OperationProgress,
  RemoveSourceImpact,
  SkillDetail,
  SkillDocument,
  SkillsDashboard,
  SourceSyncResult,
} from "./types";

function inTauriRuntime() {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

function requireTauriRuntime() {
  if (!inTauriRuntime()) throw new Error("该操作需要在 Agent Manager 桌面应用中执行");
}

function progressChannel(onProgress: (progress: OperationProgress) => void) {
  const channel = new Channel<OperationProgress>();
  channel.onmessage = onProgress;
  return channel;
}

export const skillsApi = {
  dashboard() {
    if (!inTauriRuntime()) {
      return Promise.resolve<SkillsDashboard>({
        catalog: { sources: [] },
        repositoryPath: "~/.agent-manager/skills",
        globalTargetPath: "~/.agents/skills",
        npxAvailable: false,
        enabledContentChars: 0,
      });
    }
    return invoke<SkillsDashboard>("get_skills_dashboard");
  },
  install(source: string, onProgress: (progress: OperationProgress) => void) {
    requireTauriRuntime();
    return invoke<SourceSyncResult>("install_skill_source", {
      source,
      onProgress: progressChannel(onProgress),
    });
  },
  update(source: string, onProgress: (progress: OperationProgress) => void) {
    requireTauriRuntime();
    return invoke<SourceSyncResult>("update_skill_source", {
      source,
      onProgress: progressChannel(onProgress),
    });
  },
  updateAll(onProgress: (progress: OperationProgress) => void) {
    requireTauriRuntime();
    return invoke<BatchUpdateResult>("update_all_skill_sources", {
      onProgress: progressChannel(onProgress),
    });
  },
  setGlobalEnabled(sourceSafe: string, skillName: string, enabled: boolean) {
    requireTauriRuntime();
    return invoke<void>("set_global_skill_enabled", { sourceSafe, skillName, enabled });
  },
  detail(sourceSafe: string, skillName: string) {
    requireTauriRuntime();
    return invoke<SkillDetail>("get_skill_detail", { sourceSafe, skillName });
  },
  document(sourceSafe: string, skillName: string, relativePath: string) {
    requireTauriRuntime();
    return invoke<SkillDocument>("get_skill_document", { sourceSafe, skillName, relativePath });
  },
  openDocument(sourceSafe: string, skillName: string, relativePath: string) {
    requireTauriRuntime();
    return invoke<void>("open_skill_document", { sourceSafe, skillName, relativePath });
  },
  openFolder(sourceSafe: string, skillName: string) {
    requireTauriRuntime();
    return invoke<void>("open_skill_folder", { sourceSafe, skillName });
  },
  openGlobalFolder() {
    requireTauriRuntime();
    return invoke<void>("open_global_skills_folder");
  },
  removeImpact(sourceSafe: string) {
    requireTauriRuntime();
    return invoke<RemoveSourceImpact>("get_remove_source_impact", { sourceSafe });
  },
  remove(sourceSafe: string) {
    requireTauriRuntime();
    return invoke<SourceSyncResult>("remove_skill_source", { sourceSafe });
  },
};
