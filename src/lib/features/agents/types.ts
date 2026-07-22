import type { GaalInfo } from "$lib/features/settings/types";

export type AgentRecord = {
  name: string;
  installed: boolean;
  source: string;
  projectSkillsDir: string;
  globalSkillsDir: string;
  projectMcpConfigFile: string;
  globalMcpConfigFile: string;
  supportsGenericProject: boolean;
  supportsGenericGlobal: boolean;
};

export type AgentsDashboard = {
  agents: AgentRecord[];
  gaal: GaalInfo;
};
