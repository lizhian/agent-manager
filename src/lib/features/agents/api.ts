import { invoke } from "@tauri-apps/api/core";
import type { AgentsDashboard } from "./types";

function inTauriRuntime() {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

export const agentsApi = {
  dashboard() {
    if (!inTauriRuntime()) {
      return Promise.resolve<AgentsDashboard>({
        agents: [
          {
            name: "claude-code",
            installed: true,
            source: "builtin",
            projectSkillsDir: ".claude/skills",
            globalSkillsDir: "~/.claude/skills",
            projectMcpConfigFile: ".mcp.json",
            globalMcpConfigFile: "~/.claude.json",
            supportsGenericProject: false,
            supportsGenericGlobal: false,
          },
          {
            name: "codex",
            installed: true,
            source: "builtin",
            projectSkillsDir: ".agents/skills",
            globalSkillsDir: "~/.codex/skills",
            projectMcpConfigFile: "",
            globalMcpConfigFile: "~/.codex/config.toml",
            supportsGenericProject: true,
            supportsGenericGlobal: false,
          },
          {
            name: "claude-desktop",
            installed: false,
            source: "builtin",
            projectSkillsDir: ".agents/skills",
            globalSkillsDir: "~/.agents/skills",
            projectMcpConfigFile: "",
            globalMcpConfigFile: "~/Library/Application Support/Claude/claude_desktop_config.json",
            supportsGenericProject: true,
            supportsGenericGlobal: true,
          },
          {
            name: "cursor",
            installed: false,
            source: "builtin",
            projectSkillsDir: ".agents/skills",
            globalSkillsDir: "~/.cursor/skills",
            projectMcpConfigFile: "",
            globalMcpConfigFile: "~/.cursor/mcp.json",
            supportsGenericProject: true,
            supportsGenericGlobal: false,
          },
        ],
        gaal: {
          installed: true,
          path: "~/.agent-manager/bin/gaal",
          directory: "~/.agent-manager/bin",
          version: "dev-preview",
        },
      });
    }
    return invoke<AgentsDashboard>("get_agents_dashboard");
  },
};
