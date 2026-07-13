export type SkillRecord = {
  name: string;
  description: string;
  globalEnabled: boolean;
  installedAt: string;
  updatedAt: string;
};

export type SourceRecord = {
  source: string;
  sourceSafe: string;
  installedAt: string;
  updatedAt: string;
  skills: SkillRecord[];
};

export type SkillsDashboard = {
  catalog: { sources: SourceRecord[] };
  repositoryPath: string;
  globalTargetPath: string;
  npxAvailable: boolean;
  enabledContentChars: number;
};

export type SourceSyncResult = {
  sourceSafe: string;
  installedCount: number;
  removedCount: number;
  updated: boolean;
};

export type RemoveSourceImpact = {
  installedSkills: number;
  globalEnabledSkills: number;
};

export type SkillDetail = {
  source: string;
  sourceSafe: string;
  name: string;
  description: string;
  metadataName: string;
  metadataDescription: string;
  globalEnabled: boolean;
  updatedAt: string;
  path: string;
  content: string;
};

export type SkillDocument = {
  sourceSafe: string;
  skillName: string;
  title: string;
  relativePath: string;
  content: string;
};

export type OperationProgress = {
  operation: "install" | "update" | "update-all";
  source: string;
  stage: string;
  message: string;
  percent: number | null;
  completedSources: number;
  totalSources: number;
};

export type BatchUpdateFailure = { source: string; error: string };

export type BatchUpdateResult = {
  totalSources: number;
  succeededSources: number;
  failedSources: number;
  installedCount: number;
  removedCount: number;
  failures: BatchUpdateFailure[];
};
