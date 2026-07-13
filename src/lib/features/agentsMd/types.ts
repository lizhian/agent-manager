export type AgentsMdFragment = {
  id: number;
  title: string;
  content: string;
  enabled: boolean;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
};

export type AgentsMdDashboard = {
  fragments: AgentsMdFragment[];
  targetPath: string;
  combinedContent: string;
  enabledContentChars: number;
};
