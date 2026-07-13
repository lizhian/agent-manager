import { invoke } from "@tauri-apps/api/core";
import type { AgentsMdDashboard } from "./types";

function inTauriRuntime() {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

function requireTauriRuntime() {
  if (!inTauriRuntime()) throw new Error("该操作需要在 Agent Manager 桌面应用中执行");
}

export const agentsMdApi = {
  dashboard() {
    if (!inTauriRuntime()) {
      return Promise.resolve<AgentsMdDashboard>({
        fragments: [],
        targetPath: "~/.agents/AGENTS.md",
        combinedContent: "",
        enabledContentChars: 0,
      });
    }
    return invoke<AgentsMdDashboard>("get_agents_md_dashboard");
  },
  create(title: string, content: string) {
    requireTauriRuntime();
    return invoke<AgentsMdDashboard>("create_agents_md_fragment", { title, content });
  },
  update(id: number, title: string, content: string) {
    requireTauriRuntime();
    return invoke<AgentsMdDashboard>("update_agents_md_fragment", { id, title, content });
  },
  delete(id: number) {
    requireTauriRuntime();
    return invoke<AgentsMdDashboard>("delete_agents_md_fragment", { id });
  },
  setEnabled(id: number, enabled: boolean) {
    requireTauriRuntime();
    return invoke<AgentsMdDashboard>("set_agents_md_fragment_enabled", { id, enabled });
  },
  reorder(ids: number[]) {
    requireTauriRuntime();
    return invoke<AgentsMdDashboard>("reorder_agents_md_fragments", { ids });
  },
  openFolder() {
    requireTauriRuntime();
    return invoke<void>("open_agents_md_folder");
  },
};
