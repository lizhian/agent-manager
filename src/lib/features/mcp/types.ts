import type { GaalInfo } from "$lib/features/settings/types";

export type McpTransport = "stdio" | "http" | "sse";

export type McpServer = {
  id: number;
  name: string;
  transport: McpTransport;
  command: string;
  args: string[];
  env: Record<string, string>;
  url: string;
  headers: Record<string, string>;
  agents: string[];
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type McpDashboard = {
  servers: McpServer[];
  gaal: GaalInfo;
  defaultAgents: string[];
  availableAgents: string[];
};

export type McpServerInput = Omit<McpServer, "id" | "agents" | "enabled" | "createdAt" | "updatedAt">;

export type McpTool = {
  name: string;
  title: string;
  description: string;
  inputSchema: Record<string, unknown>;
};

export type McpDebugSnapshot = {
  tools: McpTool[];
};

export type McpToolCallOutput = {
  result: unknown;
  durationMs: number;
};
