import { invoke } from "@tauri-apps/api/core";
import type {
  McpDashboard, McpDebugSnapshot, McpServerInput, McpToolCallOutput,
} from "./types";

function inTauriRuntime() {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

function requireRuntime() {
  if (!inTauriRuntime()) throw new Error("该操作需要在 Agent Manager 桌面应用中执行");
}

export const mcpApi = {
  dashboard() {
    if (!inTauriRuntime()) {
      return Promise.resolve<McpDashboard>({
        servers: [],
        gaal: { installed: false, path: "~/.agent-manager/bin/gaal", directory: "~/.agent-manager/bin", version: "" },
        defaultAgents: ["codex"],
        availableAgents: ["*", "claude-code", "codex", "cursor"],
      });
    }
    return invoke<McpDashboard>("get_mcp_dashboard");
  },
  createServer(input: McpServerInput) {
    requireRuntime();
    return invoke<McpDashboard>("create_mcp_server", { input });
  },
  updateServer(id: number, input: McpServerInput) {
    requireRuntime();
    return invoke<McpDashboard>("update_mcp_server", { id, input });
  },
  deleteServer(id: number) {
    requireRuntime();
    return invoke<McpDashboard>("delete_mcp_server", { id });
  },
  setEnabled(id: number, enabled: boolean) {
    requireRuntime();
    return invoke<McpDashboard>("set_mcp_server_enabled", { id, enabled });
  },
  setDefaultAgents(agents: string[]) {
    requireRuntime();
    return invoke<McpDashboard>("set_mcp_default_agents", { agents });
  },
  sync() {
    requireRuntime();
    return invoke<McpDashboard>("sync_mcps");
  },
  inspectTools(id: number) {
    requireRuntime();
    return invoke<McpDebugSnapshot>("inspect_mcp_tools", { id });
  },
  callTool(id: number, toolName: string, argumentsValue: Record<string, unknown>) {
    requireRuntime();
    return invoke<McpToolCallOutput>("call_mcp_tool", {
      id,
      toolName,
      arguments: argumentsValue,
    });
  },
};
