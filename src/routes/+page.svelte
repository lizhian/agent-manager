<script lang="ts">
  import {
    AlertTriangle, Blocks, Bot, Check, ChevronDown, ChevronRight, ExternalLink,
    FileCode2, FolderOpen, LoaderCircle, PackagePlus, PanelBottom, PanelRight,
    Plug, RefreshCw, Search, Settings2, X,
  } from "@lucide/svelte";
  import Markdown from "svelte-exmarkdown";
  import { gfmPlugin } from "svelte-exmarkdown/gfm";
  import rehypeSlug from "rehype-slug";
  import "github-markdown-css/github-markdown-light.css";
  import { onMount } from "svelte";
  import { openUrl } from "@tauri-apps/plugin-opener";
  import VerticalResizeHandle from "$lib/components/VerticalResizeHandle.svelte";
  import { skillsApi } from "$lib/features/skills/api";
  import AgentsMdWorkspace from "$lib/features/agentsMd/AgentsMdWorkspace.svelte";
  import SettingsWorkspace from "$lib/features/settings/SettingsWorkspace.svelte";
  import { settingsApi } from "$lib/features/settings/api";
  import type { DocumentPreviewPosition, FontSize } from "$lib/features/settings/types";
  import type {
    BatchUpdateResult, OperationProgress, SkillDetail, SkillRecord,
    SkillDocument, SkillsDashboard, SourceRecord,
  } from "$lib/features/skills/types";

  type SectionId = "skills" | "mcp" | "agents" | "agents-md" | "settings";
  type Dialog = "install" | null;
  type MarkdownLinkContext = { sourceSafe: string; skillName: string; currentPath: string };

  const sections = [
    { id: "skills" as const, label: "Skills", icon: Blocks, available: true },
    { id: "mcp" as const, label: "MCP", icon: Plug, available: false },
    { id: "agents" as const, label: "Agents", icon: Bot, available: false },
    { id: "agents-md" as const, label: "AGENTS.md", icon: FileCode2, available: true },
  ];
  const markdownPlugins = [gfmPlugin(), { rehypePlugin: rehypeSlug }];

  let activeSection: SectionId = $state("skills");
  let dashboard: SkillsDashboard | null = $state(null);
  let loading = $state(true);
  let writing = $state(false);
  let activeSource = $state("");
  let togglingSkill = $state("");
  let error = $state("");
  let notice = $state("");
  let query = $state("");
  let globalOnly = $state(false);
  let dialog: Dialog = $state(null);
  let installSource = $state("");
  let progress: OperationProgress | null = $state(null);
  let batchResult: BatchUpdateResult | null = $state(null);
  let expanded = $state<Set<string>>(new Set());
  let detail: SkillDetail | null = $state(null);
  let detailLoading = $state(false);
  let documentPreviewOpen = $state(false);
  let documentPreview: SkillDocument | null = $state(null);
  let documentLoading = $state(false);
  let documentError = $state("");
  let documentRequestId = 0;
  let documentPreviewPosition: DocumentPreviewPosition = $state("bottom");
  let documentPreviewRatio = $state(0.5);
  let documentLayoutRequestId = 0;
  let previewBody: HTMLElement | null = $state(null);
  let documentPreviewPane: HTMLElement | null = $state(null);
  let fontSize: FontSize = $state("standard");
  let fontFamily = $state("");
  let settingsError = $state("");
  let sidebarWidth = $state(156);
  let skillsListWidth = $state(0);
  let skillsContentGrid: HTMLElement | null = $state(null);
  let skillsListSection: HTMLElement | null = $state(null);

  $effect(() => {
    const root = document.documentElement;
    root.style.setProperty("--app-font-family", cssFontFamily(fontFamily));
  });

  let skillCount = $derived.by(() => {
    const sources: SourceRecord[] = dashboard ? dashboard.catalog.sources : [];
    return sources.reduce((total, source) => total + source.skills.length, 0);
  });
  let enabledCount = $derived.by(() => {
    const sources: SourceRecord[] = dashboard ? dashboard.catalog.sources : [];
    return sources.reduce(
      (total, source) => total + source.skills.filter((skill) => skill.globalEnabled).length,
      0,
    );
  });
  let filteredSources = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    return (dashboard?.catalog.sources ?? [])
      .map((source) => ({
        ...source,
        skills: source.skills.filter((skill) =>
          (!globalOnly || skill.globalEnabled) &&
          (!needle || source.source.toLocaleLowerCase().includes(needle) ||
          skill.name.toLocaleLowerCase().includes(needle) ||
          skill.description.toLocaleLowerCase().includes(needle))),
      }))
      .filter((source) => source.skills.length > 0);
  });

  function clamp(value: number, minimum: number, maximum: number) {
    return Math.min(maximum, Math.max(minimum, value));
  }

  function resizeSidebar(delta: number) {
    sidebarWidth = clamp(sidebarWidth + delta, 156, 300);
  }

  function resizeSkillsList(delta: number) {
    if (!skillsContentGrid || !skillsListSection) return;
    const gridWidth = skillsContentGrid.getBoundingClientRect().width;
    if (!gridWidth) return;
    const currentWidth = skillsListSection.getBoundingClientRect().width;
    const nextWidth = clamp(currentWidth + delta, 270, Math.max(270, gridWidth - 327));
    skillsListWidth = nextWidth / gridWidth * 100;
  }

  function resizeDocumentPreview(delta: number) {
    if (!previewBody || !documentPreviewPane) return;
    const bodyRect = previewBody.getBoundingClientRect();
    const paneRect = documentPreviewPane.getBoundingClientRect();
    const totalSize = documentPreviewPosition === "right" ? bodyRect.width : bodyRect.height;
    const currentSize = documentPreviewPosition === "right" ? paneRect.width : paneRect.height;
    if (!totalSize) return;
    const availableSize = Math.max(0, totalSize - 7);
    const minimumSize = Math.max(totalSize * 0.2, Math.min(180, availableSize / 2));
    const maximumSize = Math.min(totalSize * 0.8, availableSize - minimumSize);
    const nextSize = clamp(currentSize - delta, minimumSize, Math.max(minimumSize, maximumSize));
    documentPreviewRatio = clamp(nextSize / totalSize, 0.2, 0.8);
  }

  async function persistDocumentPreviewLayout() {
    const requestId = ++documentLayoutRequestId;
    const position = documentPreviewPosition;
    const ratio = documentPreviewRatio;
    try {
      const settings = await settingsApi.setDocumentPreviewLayout(position, ratio);
      if (requestId === documentLayoutRequestId) {
        documentPreviewPosition = settings.documentPreviewPosition;
        documentPreviewRatio = settings.documentPreviewRatio;
      }
    } catch (reason) {
      if (requestId === documentLayoutRequestId) {
        error = `保存文档预览布局失败：${messageOf(reason)}`;
      }
    }
  }

  function toggleDocumentPreviewPosition() {
    documentPreviewPosition = documentPreviewPosition === "bottom" ? "right" : "bottom";
    void persistDocumentPreviewLayout();
  }

  function resetDocumentPreviewRatio() {
    documentPreviewRatio = 0.5;
    void persistDocumentPreviewLayout();
  }
  onMount(() => {
    void loadDashboard();
    void loadSettings();
  });

  async function loadSettings() {
    try {
      const settings = await settingsApi.get();
      fontSize = settings.fontSize;
      fontFamily = settings.fontFamily;
      documentPreviewPosition = settings.documentPreviewPosition;
      documentPreviewRatio = settings.documentPreviewRatio;
    } catch (reason) {
      settingsError = messageOf(reason);
    }
  }

  async function loadDashboard() {
    loading = true;
    error = "";
    try {
      dashboard = await skillsApi.dashboard();
      const selectedStillExists = detail && dashboard.catalog.sources.some((source) =>
        source.sourceSafe === detail?.sourceSafe && source.skills.some((skill) => skill.name === detail?.name),
      );
      if (!selectedStillExists) {
        const firstSource = dashboard.catalog.sources[0];
        const firstSkill = firstSource?.skills[0];
        if (firstSource && firstSkill) await openDetail(firstSource, firstSkill);
        else detail = null;
      }
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      loading = false;
    }
  }

  function isExpanded(sourceSafe: string) {
    return query.trim().length > 0 || expanded.has(sourceSafe);
  }

  function toggleExpanded(sourceSafe: string) {
    const next = new Set(expanded);
    next.has(sourceSafe) ? next.delete(sourceSafe) : next.add(sourceSafe);
    expanded = next;
  }

  function updateProgress(value: OperationProgress) {
    progress = value;
    if (value.source) activeSource = value.source;
  }

  async function install() {
    if (!installSource.trim()) return;
    writing = true;
    batchResult = null;
    error = "";
    progress = null;
    try {
      const result = await skillsApi.install(installSource.trim(), updateProgress);
      notice = `已安装 ${result.installedCount} 个 Skills`;
      installSource = "";
      await loadDashboard();
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      writing = false;
      activeSource = "";
      progress = null;
    }
  }

  async function updateSource(source: SourceRecord) {
    writing = true;
    activeSource = source.source;
    batchResult = null;
    error = "";
    progress = null;
    try {
      const result = await skillsApi.update(source.source, updateProgress);
      notice = `已更新 ${source.source}：${result.installedCount} 个 Skills${result.removedCount ? `，清理 ${result.removedCount} 个失效项` : ""}`;
      await loadDashboard();
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      writing = false;
      activeSource = "";
      progress = null;
    }
  }

  async function updateAll() {
    if (!dashboard?.catalog.sources.length) return;
    writing = true;
    batchResult = null;
    error = "";
    progress = null;
    try {
      batchResult = await skillsApi.updateAll(updateProgress);
      notice = `全部更新完成：${batchResult.succeededSources} 个成功，${batchResult.failedSources} 个失败`;
      await loadDashboard();
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      writing = false;
      activeSource = "";
      progress = null;
    }
  }

  async function openDetail(source: SourceRecord, skill: SkillRecord) {
    detailLoading = true;
    detail = null;
    error = "";
    closeDocumentPreview();
    try {
      detail = await skillsApi.detail(source.sourceSafe, skill.name);
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      detailLoading = false;
    }
  }

  async function toggleGlobal(event: MouseEvent, source: SourceRecord, skill: SkillRecord) {
    event.stopPropagation();
    const key = `${source.sourceSafe}__${skill.name}`;
    const enabled = !skill.globalEnabled;
    togglingSkill = key;
    error = "";
    try {
      await skillsApi.setGlobalEnabled(source.sourceSafe, skill.name, enabled);
      dashboard = await skillsApi.dashboard();
      if (detail?.sourceSafe === source.sourceSafe && detail.name === skill.name) {
        const savedSkill = dashboard.catalog.sources
          .find((item) => item.sourceSafe === source.sourceSafe)?.skills
          .find((item) => item.name === skill.name);
        if (savedSkill) detail = { ...detail, globalEnabled: savedSkill.globalEnabled };
      }
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      togglingSkill = "";
    }
  }

  function markdownLinks(node: HTMLElement, initialContext: MarkdownLinkContext) {
    let context = initialContext;
    const handleClick = (event: MouseEvent) => void openMarkdownLink(node, event.target, event, context);
    const handleKeydown = (event: KeyboardEvent) => {
      if (event.key === "Enter") void openMarkdownLink(node, event.target, event, context);
    };
    node.addEventListener("click", handleClick);
    node.addEventListener("keydown", handleKeydown);
    return {
      update(nextContext: MarkdownLinkContext) { context = nextContext; },
      destroy: () => {
        node.removeEventListener("click", handleClick);
        node.removeEventListener("keydown", handleKeydown);
      },
    };
  }

  async function openMarkdownLink(
    markdownRoot: HTMLElement,
    target: EventTarget | null,
    event: Event,
    context: MarkdownLinkContext,
  ) {
    const element = target instanceof Element ? target.closest("a") : null;
    if (!(element instanceof HTMLAnchorElement)) return;
    const href = element.getAttribute("href")?.trim();
    if (!href) return;
    if (href.startsWith("#")) {
      event.preventDefault();
      scrollToMarkdownAnchor(markdownRoot, href.slice(1));
      return;
    }
    if (/^https?:\/\//i.test(href)) {
      event.preventDefault();
      await openUrl(href);
      return;
    }
    if (/^[a-z][a-z\d+.-]*:/i.test(href)) return;

    const relativePath = resolveLocalDocumentPath(href, context.currentPath);
    if (!relativePath) return;
    event.preventDefault();
    const requestId = ++documentRequestId;
    documentPreviewOpen = true;
    documentLoading = true;
    documentError = "";
    documentPreview = null;
    try {
      const nextDocument = await skillsApi.document(
        context.sourceSafe,
        context.skillName,
        relativePath,
      );
      if (requestId === documentRequestId) documentPreview = nextDocument;
    } catch (reason) {
      if (requestId === documentRequestId) documentError = messageOf(reason);
    } finally {
      if (requestId === documentRequestId) documentLoading = false;
    }
  }

  function scrollToMarkdownAnchor(markdownRoot: HTMLElement, encodedId: string) {
    let id: string;
    try {
      id = decodeURIComponent(encodedId);
    } catch {
      return;
    }
    const heading = markdownRoot.querySelector<HTMLElement>(`#${CSS.escape(id)}`);
    const scrollContainer = markdownRoot.closest<HTMLElement>(".preview-scroll, .document-content");
    if (!heading || !scrollContainer) return;
    const offset = heading.getBoundingClientRect().top - scrollContainer.getBoundingClientRect().top;
    scrollContainer.scrollTo({
      top: scrollContainer.scrollTop + offset - 12,
      behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth",
    });
  }

  function resolveLocalDocumentPath(href: string, currentPath: string) {
    try {
      const base = new URL(encodeURI(currentPath), "https://local.invalid/");
      const resolved = new URL(href, base);
      return decodeURIComponent(resolved.pathname.slice(1));
    } catch {
      return null;
    }
  }

  function closeDocumentPreview() {
    documentRequestId += 1;
    documentPreviewOpen = false;
    documentPreview = null;
    documentError = "";
    documentLoading = false;
  }

  async function openSkillFolder(sourceSafe: string, skillName: string) {
    error = "";
    try {
      await skillsApi.openFolder(sourceSafe, skillName);
    } catch (reason) {
      error = `打开本地文件夹失败：${messageOf(reason)}`;
    }
  }

  async function openDocumentExternally(document: SkillDocument | null) {
    if (!document) return;
    error = "";
    try {
      await skillsApi.openDocument(
        document.sourceSafe,
        document.skillName,
        document.relativePath,
      );
    } catch (reason) {
      error = `使用系统默认应用打开文档失败：${messageOf(reason)}`;
    }
  }

  async function openGlobalSkillsFolder() {
    error = "";
    try {
      await skillsApi.openGlobalFolder();
    } catch (reason) {
      error = `打开技能文件夹失败：${messageOf(reason)}`;
    }
  }

  async function openSkillsShSource(source: string) {
    error = "";
    try {
      const path = source.split("/").map((part) => encodeURIComponent(part)).join("/");
      await openUrl(`https://www.skills.sh/${path}`);
    } catch (reason) {
      error = `打开 Skills.sh 来源失败：${messageOf(reason)}`;
    }
  }

  function formatTime(value: string) {
    if (!value) return "从未更新";
    const numeric = Number(value);
    const date = Number.isFinite(numeric) ? new Date(numeric * 1000) : new Date(value);
    return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", {
      year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit",
    }).format(date);
  }

  function formatCount(value: number) {
    return new Intl.NumberFormat("zh-CN").format(value);
  }

  function messageOf(reason: unknown) {
    return reason instanceof Error ? reason.message : String(reason);
  }

  function cssFontFamily(value: string) {
    if (!value) return 'Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif';
    const escaped = value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
    return `"${escaped}",ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif`;
  }
</script>

<svelte:head><title>Agent Manager</title></svelte:head>

<div class="window-drag-region" style:width={`${sidebarWidth}px`} data-tauri-drag-region aria-hidden="true"></div>

<main class="app-shell" style:--sidebar-width={`${sidebarWidth}px`} class:font-extra-small={fontSize === "extra-small"} class:font-small={fontSize === "small"} class:font-large={fontSize === "large"} class:font-extra-large={fontSize === "extra-large"}>
  <aside class="sidebar" aria-label="主导航">
    <div class="brand"><div class="brand-mark"><Settings2 size={19} /></div><div><h1>Agent Manager</h1></div></div>
    <nav class="nav-list">
      {#each sections as section}
        <button class:active={activeSection === section.id} aria-label={section.label} title={section.label} onclick={() => (activeSection = section.id)}>
          <section.icon size={18} /><span>{section.label}</span>
        </button>
      {/each}
    </nav>
    <div class="sidebar-bottom">
      <button class="settings-nav" class:active={activeSection === "settings"} aria-label="设置" title="设置" onclick={() => (activeSection = "settings")}><Settings2 size={17} /><span>设置</span></button>
    </div>
  </aside>

  <div class="app-divider"><VerticalResizeHandle label="调整导航栏宽度" onResize={resizeSidebar} onReset={() => (sidebarWidth = 156)} /></div>

  <section class="workspace">
    {#if activeSection === "skills"}
      <header class="topbar" data-tauri-drag-region>
        <div class="title"><h2>技能仓库</h2></div>
        <div class="metrics">
          <div><strong>{dashboard?.catalog.sources.length ?? 0}</strong><span>个来源</span></div>
          <div><strong>{skillCount}</strong><span>个 Skills</span></div>
          <div><strong>{enabledCount}</strong><span>个全局启用 · 共 {formatCount(dashboard?.enabledContentChars ?? 0)} 字符</span></div>
        </div>
        <div class="top-actions">
          <button class="secondary" onclick={() => (dialog = "install")} disabled={writing}><PackagePlus size={16} />安装来源</button>
          <button class="secondary" onclick={updateAll} disabled={writing || !dashboard?.catalog.sources.length}><RefreshCw size={16} />全部更新</button>
          <button class="secondary" onclick={openGlobalSkillsFolder}><FolderOpen size={16} />技能文件夹</button>
        </div>
      </header>

      {#if progress}
        <section class="progress-panel" aria-live="polite">
          <div class="progress-copy"><div><LoaderCircle class="spin" size={16} /><strong>{progress.message}</strong><span>{progress.source}</span></div>{#if progress.totalSources > 1}<small>{progress.completedSources}/{progress.totalSources} 个来源</small>{/if}</div>
          <div class="progress-track" class:indeterminate={progress.percent === null}><span style:width={progress.percent === null ? "38%" : `${progress.percent}%`}></span></div>
        </section>
      {/if}

      {#if error}<div class="message error"><AlertTriangle size={17} /><span>{error}</span><button aria-label="关闭" onclick={() => (error = "")}><X size={16} /></button></div>{/if}
      {#if notice}<div class="message success"><Check size={17} /><span>{notice}</span><button aria-label="关闭" onclick={() => (notice = "")}><X size={16} /></button></div>{/if}
      {#if batchResult?.failures.length}
        <details class="batch-errors"><summary>{batchResult.failures.length} 个来源更新失败</summary>{#each batchResult.failures as failure}<p><strong>{failure.source}</strong><span>{failure.error}</span></p>{/each}</details>
      {/if}

      <section class="content-grid" bind:this={skillsContentGrid} style:--list-width={`${skillsListWidth}%`}>
        <section class="list-section" bind:this={skillsListSection}>
          <header><label class="search"><Search size={16} /><input bind:value={query} placeholder="搜索来源或 Skill" /></label><button class="filter-toggle" class:on={globalOnly} role="switch" aria-checked={globalOnly} onclick={() => (globalOnly = !globalOnly)} title={globalOnly ? "显示全部 Skills" : "只显示全局启用的 Skills"}><span>只看启用</span><i></i></button></header>
          <div class="list-scroll">
            {#if loading}<div class="state"><LoaderCircle class="spin" size={24} /><p>正在读取 Skills 仓库…</p></div>
            {:else if !dashboard?.catalog.sources.length}<div class="state"><Blocks size={30} /><h3>还没有安装来源</h3><p>安装 GitHub Skill 仓库后即可在这里管理。</p><button class="primary" onclick={() => (dialog = "install")}><PackagePlus size={16} />安装第一个来源</button></div>
            {:else if !filteredSources.length}<div class="state"><Search size={28} /><h3>{globalOnly ? "没有已启用的全局技能" : "没有匹配结果"}</h3>{#if query}<button class="secondary" onclick={() => (query = "")}>清空搜索</button>{/if}</div>
            {:else}<div class="tree">
              {#each filteredSources as source}
                <section class="source-node">
                  <div class="source-row">
                    <button class="expand" aria-label={isExpanded(source.sourceSafe) ? "折叠来源" : "展开来源"} onclick={() => toggleExpanded(source.sourceSafe)}>{#if isExpanded(source.sourceSafe)}<ChevronDown size={16} />{:else}<ChevronRight size={16} />{/if}</button>
                    <button class="source-name" onclick={() => toggleExpanded(source.sourceSafe)}><strong>{source.source}</strong><span>{source.skills.length} 个 Skills · {formatTime(source.updatedAt)}</span></button>
                    <button class="icon-button" title="更新来源" aria-label={`更新 ${source.source}`} onclick={() => updateSource(source)} disabled={writing}>{#if activeSource === source.source}<LoaderCircle class="spin" size={15} />{:else}<RefreshCw size={15} />{/if}</button>
                  </div>
                  {#if isExpanded(source.sourceSafe)}
                    <div class="skill-children">
                      {#each source.skills as skill}
                        {@const key = `${source.sourceSafe}__${skill.name}`}
                        <div class="skill-row" class:selected={detail?.sourceSafe === source.sourceSafe && detail?.name === skill.name} role="button" tabindex="0" onclick={() => openDetail(source, skill)} onkeydown={(event) => (event.key === "Enter" || event.key === " ") && openDetail(source, skill)}>
                          <span class="skill-copy"><strong>{skill.name}</strong><span>{skill.description || "暂无描述"}</span></span>
                          <button class="skill-switch" class:on={skill.globalEnabled} role="switch" aria-checked={skill.globalEnabled} aria-label={`${skill.name} 全局${skill.globalEnabled ? "禁用" : "启用"}`} onclick={(event) => toggleGlobal(event, source, skill)} disabled={writing || !!togglingSkill}>
                            {#if togglingSkill === key}<LoaderCircle class="spin" size={11} />{/if}
                          </button>
                        </div>
                      {/each}
                    </div>
                  {/if}
                </section>
              {/each}
            </div>{/if}
          </div>
        </section>

        <div class="panel-divider"><VerticalResizeHandle label="调整技能列表宽度" onResize={resizeSkillsList} onReset={() => (skillsListWidth = 0)} /></div>

        <section class="preview-panel">
          {#if detailLoading}
            <div class="state"><LoaderCircle class="spin" size={24} /><p>正在读取 SKILL.md…</p></div>
          {:else if detail}
            {@const selectedDetail = detail}
            <header class="preview-header">
              <div class="preview-title">
                <h3>{selectedDetail.name}</h3>
                <button class="source-link" title={`在 Skills.sh 打开 ${selectedDetail.source}`} onclick={() => openSkillsShSource(selectedDetail.source)}>
                  <span>{selectedDetail.source}</span><ExternalLink size={12} />
                </button>
              </div>
              <button class="path-button" title="打开本地文件夹" onclick={() => openSkillFolder(selectedDetail.sourceSafe, selectedDetail.name)}><FolderOpen size={14} /><span>打开本地文件夹</span></button>
              <time title="最后更新时间">{formatTime(selectedDetail.updatedAt)}</time>
              <span class:enabled={selectedDetail.globalEnabled}>{selectedDetail.globalEnabled ? "全局启用" : "未启用"}</span>
            </header>
            <div
              class="preview-body"
              class:document-right={documentPreviewOpen && documentPreviewPosition === "right"}
              style:--document-size={`${documentPreviewRatio * 100}%`}
              bind:this={previewBody}
            >
              <div class="preview-scroll">
                <table class="metadata-table" aria-label="Skill 元数据">
                  <tbody>
                    <tr><th scope="row">name</th><td>{selectedDetail.metadataName}</td></tr>
                    <tr><th scope="row">description</th><td>{selectedDetail.metadataDescription || "暂无描述"}</td></tr>
                  </tbody>
                </table>
                <div class="markdown markdown-body" role="document" use:markdownLinks={{ sourceSafe: selectedDetail.sourceSafe, skillName: selectedDetail.name, currentPath: `${selectedDetail.name}/SKILL.md` }}><Markdown md={selectedDetail.content} plugins={markdownPlugins} /></div>
              </div>
              {#if documentPreviewOpen}
                <div class="document-divider">
                  <VerticalResizeHandle
                    label="调整文档预览占用比例"
                    orientation={documentPreviewPosition === "right" ? "vertical" : "horizontal"}
                    onResize={resizeDocumentPreview}
                    onResizeEnd={() => void persistDocumentPreviewLayout()}
                    onReset={resetDocumentPreviewRatio}
                  />
                </div>
                <section class="document-inline" aria-label="Markdown 文档预览" bind:this={documentPreviewPane}>
                  <header>
                    {#if documentPreview}
                      <button
                        class="document-path"
                        title={`使用系统默认应用打开 ${documentPreview.relativePath}`}
                        aria-label={`使用系统默认应用打开 ${documentPreview.relativePath}`}
                        onclick={() => openDocumentExternally(documentPreview)}
                      ><span>{documentPreview.relativePath}</span></button>
                    {:else}
                      <span class="document-path pending">正在读取本地文档</span>
                    {/if}
                    <div class="document-actions">
                      <button class="icon-button" aria-label={documentPreviewPosition === "bottom" ? "切换到右侧预览" : "切换到下方预览"} title={documentPreviewPosition === "bottom" ? "切换到右侧预览" : "切换到下方预览"} onclick={toggleDocumentPreviewPosition}>
                        {#if documentPreviewPosition === "bottom"}<PanelRight size={17} />{:else}<PanelBottom size={17} />{/if}
                      </button>
                      <button class="icon-button" aria-label="关闭文档预览" title="关闭文档预览" onclick={closeDocumentPreview}><X size={17} /></button>
                    </div>
                  </header>
                  <div class="document-content">
                    {#if documentLoading}
                      <div class="state"><LoaderCircle class="spin" size={24} /><p>正在读取本地 Markdown 文档…</p></div>
                    {:else if documentError}
                      <div class="state document-error"><AlertTriangle size={28} /><h3>无法打开文档</h3><p>{documentError}</p></div>
                    {:else if documentPreview}
                      <div class="markdown markdown-body" role="document" use:markdownLinks={{ sourceSafe: documentPreview.sourceSafe, skillName: documentPreview.skillName, currentPath: documentPreview.relativePath }}><Markdown md={documentPreview.content} plugins={markdownPlugins} /></div>
                    {/if}
                  </div>
                </section>
              {/if}
            </div>
          {:else}
            <div class="state"><FileCode2 size={30} /><h3>选择一个 Skill</h3><p>SKILL.md 内容将在这里显示。</p></div>
          {/if}
        </section>
      </section>
    {:else if activeSection === "agents-md"}
      <AgentsMdWorkspace />
    {:else if activeSection === "settings"}
      <SettingsWorkspace {fontSize} {fontFamily} initialError={settingsError} onFontSizeChange={(next) => (fontSize = next)} onFontFamilyChange={(next) => (fontFamily = next)} />
    {:else}
      {@const section = sections.find((item) => item.id === activeSection)}
      <div class="state standalone"><Settings2 size={30} /><h3>{section?.label} 正在建设</h3><p>该模块将沿用当前应用框架逐步加入。</p></div>
    {/if}
  </section>
</main>

{#if dialog}
  <div class="backdrop" role="presentation" onclick={(event) => event.currentTarget === event.target && !writing && (dialog = null)} onkeydown={(event) => event.key === "Escape" && !writing && (dialog = null)}>
    <div class="dialog" role="dialog" aria-modal="true">
      <header><div><h2>安装 Skill 来源</h2><p>输入 GitHub 仓库，安装任务会在后台持续运行。</p></div><button class="icon-button" onclick={() => (dialog = null)}><X size={18} /></button></header>
      {#if writing && progress?.operation === "install"}<div class="dialog-progress"><LoaderCircle class="spin" size={28} /><strong>{progress.message}</strong><span>{progress.source}</span><div class="progress-track" class:indeterminate={progress.percent === null}><i style:width={progress.percent === null ? "38%" : `${progress.percent}%`}></i></div><p>可以关闭此窗口，安装仍会继续。</p></div>
      {:else}<form onsubmit={(event) => { event.preventDefault(); install(); }}><label><span>GitHub 来源</span><input bind:value={installSource} placeholder="owner/repo" pattern="[A-Za-z0-9._-]+/[A-Za-z0-9._-]+" required /><small>需要本机可运行 npx skills。</small></label><footer><button type="button" class="secondary" onclick={() => (dialog = null)}>取消</button><button class="primary" type="submit" disabled={!installSource.trim()}>开始安装</button></footer></form>{/if}
    </div>
  </div>
{/if}

<style>
  :global(*){box-sizing:border-box}:global(:root){font-family:var(--app-font-family,Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif);color:#20262d;background:#f3f5f7;--border:#dce1e6;--muted:#68737f;--blue:#2563eb;--red:#c93434;--control-height:28px;--compact-gap:6px;--panel-gap:8px;--panel-padding:8px}:global(body){margin:0;min-width:320px;min-height:100vh}button,input{font:inherit;letter-spacing:0}button{cursor:pointer}button:disabled{cursor:not-allowed;opacity:.52}h1,h2,h3,p{margin:0}
  .window-drag-region{position:fixed;top:0;left:0;z-index:30;height:28px}.app-shell{display:grid;grid-template-columns:var(--sidebar-width,156px) 7px minmax(0,1fr);height:100vh;overflow:hidden;--font-page-title:20px;--font-metric:19px;--font-brand:16px;--font-nav:14px;--font-panel-title:14px;--font-body:15px;--font-control:13px;--font-aux:12px;--font-dialog-title:19px;--font-editor:14px}.app-shell.font-extra-small{--font-page-title:18px;--font-metric:17px;--font-brand:14px;--font-nav:12px;--font-panel-title:12px;--font-body:13px;--font-control:11px;--font-aux:10px;--font-dialog-title:17px;--font-editor:12px}.app-shell.font-small{--font-page-title:19px;--font-metric:18px;--font-brand:15px;--font-nav:13px;--font-panel-title:13px;--font-body:14px;--font-control:12px;--font-aux:11px;--font-dialog-title:18px;--font-editor:13px}.app-shell.font-large{--font-page-title:21px;--font-metric:20px;--font-brand:17px;--font-nav:15px;--font-panel-title:15px;--font-body:16px;--font-control:14px;--font-aux:13px;--font-dialog-title:20px;--font-editor:15px}.app-shell.font-extra-large{--font-page-title:23px;--font-metric:22px;--font-brand:19px;--font-nav:16px;--font-panel-title:16px;--font-body:17px;--font-control:15px;--font-aux:14px;--font-dialog-title:22px;--font-editor:16px}.sidebar{display:flex;min-width:0;flex-direction:column;height:100vh;padding:36px 8px 10px;background:#fbfcfd}.app-divider,.panel-divider{min-width:0}.brand{display:grid;grid-template-columns:30px 1fr;gap:7px;align-items:center;padding:0 6px 10px}.brand-mark{display:grid;width:30px;height:30px;place-items:center;border-radius:6px;color:white;background:#1f2937}.brand h1{font-size:13px}.nav-list{display:grid;gap:1px}.nav-list button{display:grid;grid-template-columns:18px 1fr auto;gap:7px;align-items:center;min-height:32px;padding:4px 8px;border:1px solid transparent;border-radius:6px;color:#46515d;background:transparent;text-align:left}.nav-list button.active{border-color:#d2dbe5;color:#1c2733;background:#e9eff5}.nav-list span{font-size:11px;font-weight:650}.sidebar-bottom{display:grid;grid-template-columns:auto minmax(0,1fr);gap:2px;align-items:center;margin-top:auto}.settings-nav{display:grid;grid-template-columns:17px auto;gap:5px;align-items:center;min-height:32px;padding:4px 6px;border:1px solid transparent;border-radius:6px;color:#46515d;background:transparent;text-align:left}.settings-nav:hover{background:#f0f3f6}.settings-nav.active{border-color:#d2dbe5;color:#1c2733;background:#e9eff5}.settings-nav span{font-size:var(--font-nav);font-weight:650}
  .workspace{display:flex;min-width:0;height:100vh;flex-direction:column;padding:6px 10px 10px 3px;overflow:hidden}.topbar{display:grid;grid-template-columns:minmax(140px,1fr) auto auto;gap:12px;align-items:center;padding:0 1px 6px;border-bottom:1px solid var(--border);user-select:none}.topbar>.title,.topbar>.metrics{pointer-events:none}.title h2{font-size:17px}.metrics{display:flex;gap:12px}.metrics div{display:flex;gap:3px;align-items:baseline;white-space:nowrap}.metrics strong{font-size:15px}.metrics span{color:var(--muted);font-size:9px}.top-actions{display:flex;gap:4px}.primary,.secondary,.filter-toggle{display:inline-flex;min-height:var(--control-height);align-items:center;justify-content:center;gap:4px;border-radius:5px;padding:4px 8px;font-size:10px;font-weight:650;white-space:nowrap}.primary{border:1px solid #1f58d8;color:white;background:var(--blue)}.secondary,.filter-toggle{border:1px solid #ccd3da;color:#35414c;background:white}.filter-toggle i{position:relative;width:26px;height:14px;border-radius:999px;background:#c8cfd6;transition:background .16s}.filter-toggle i:after{position:absolute;top:2px;left:2px;width:10px;height:10px;border-radius:50%;background:white;box-shadow:0 1px 2px rgba(0,0,0,.18);content:"";transition:transform .16s}.filter-toggle.on{border-color:#a9c2ef;color:#174a9e;background:#f3f7ff}.filter-toggle.on i{background:var(--blue)}.filter-toggle.on i:after{transform:translateX(12px)}
  .progress-panel,.message,.batch-errors{flex:none;margin-top:6px}.progress-panel{border:1px solid #bed0ee;border-radius:6px;padding:6px 8px;background:#f5f8ff}.progress-copy,.progress-copy>div{display:flex;align-items:center;gap:5px}.progress-copy{justify-content:space-between;font-size:9px}.progress-copy strong{font-size:10px}.progress-copy span,.progress-copy small{color:var(--muted)}.progress-track{height:3px;margin-top:5px;overflow:hidden;border-radius:999px;background:#dbe5f4}.progress-track span,.progress-track i{display:block;height:100%;border-radius:inherit;background:var(--blue);transition:width .2s}.progress-track.indeterminate span,.progress-track.indeterminate i{animation:progress 1.1s ease-in-out infinite}.message{display:grid;grid-template-columns:15px 1fr 22px;gap:5px;align-items:center;border:1px solid;border-radius:6px;padding:4px 7px;font-size:10px}.message.error{border-color:#efcaca;color:#922d2d;background:#fff3f3}.message.success{border-color:#bfe1cd;color:#24643d;background:#effaf3}.message button{display:grid;width:22px;height:22px;place-items:center;border:0;color:inherit;background:transparent}.batch-errors{border:1px solid #efcece;border-radius:6px;padding:5px 8px;color:#8f3333;background:#fff7f7;font-size:9px}.batch-errors summary{cursor:pointer;font-weight:700}.batch-errors p{display:grid;grid-template-columns:140px 1fr;gap:6px;margin-top:4px}.batch-errors span{color:#755}
  .content-grid{display:grid;min-height:0;flex:1;grid-template-columns:minmax(270px,min(var(--list-width,0%),calc(100% - 327px))) 7px minmax(320px,1fr);gap:0;margin-top:var(--panel-gap)}.list-section,.preview-panel{min-height:0;overflow:hidden;border:1px solid var(--border);border-radius:6px;background:white}.list-section{display:flex;flex-direction:column}.list-section>header{display:grid;grid-template-columns:minmax(100px,1fr) auto;gap:7px;align-items:center;padding:6px 8px;border-bottom:1px solid var(--border);background:#fafbfc}.list-section h3{font-size:11px}.search{display:flex;min-width:0;align-items:center;gap:5px;border:1px solid #ccd3da;border-radius:5px;padding:0 7px;color:#7b8791;background:white}.search:focus-within{border-color:#7ea4ee;box-shadow:0 0 0 2px rgba(37,99,235,.1)}.search input{width:100%;height:26px;border:0;outline:0;background:transparent;font-size:10px}.list-scroll{min-height:0;flex:1;overflow:auto}.source-node{border-bottom:1px solid #e8ebee}.source-node:last-child{border-bottom:0}.source-row{display:grid;grid-template-columns:24px minmax(0,1fr) 26px;gap:4px;align-items:center;min-height:40px;padding:3px 6px;background:#fafbfc}.expand,.source-name{border:0;background:transparent}.expand{display:grid;width:24px;height:24px;place-items:center;border-radius:5px;color:#697681}.expand:hover{background:#e9edf1}.source-name{min-width:0;text-align:left}.source-name strong,.source-name span{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.source-name strong{font-size:10px;font-weight:700}.source-name span{margin-top:1px;color:#7a858f;font-size:8px}.icon-button{display:grid;width:26px;height:26px;place-items:center;border:1px solid transparent;border-radius:5px;color:#65717c;background:transparent}.icon-button:hover{border-color:#d5dbe1;background:white}
  .skill-children{background:white}.skill-row{display:grid;width:100%;grid-template-columns:minmax(0,1fr) 28px;gap:4px;align-items:center;min-height:38px;padding:3px 7px 3px calc(34px + .5em);border-top:1px solid #edf0f2;color:inherit;background:white;text-align:left;cursor:pointer}.skill-row:hover{background:#f7f9fb}.skill-row:focus-visible{outline:2px solid #7ea4ee;outline-offset:-2px}.skill-row.selected{color:#174a9e;background:#edf4ff}.skill-copy{display:grid;min-width:0;gap:1px}.skill-copy strong,.skill-copy span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.skill-copy strong{font-size:10px;font-weight:500}.skill-copy span{color:#75818c;font-size:8px}.skill-switch{position:relative;display:grid;width:26px;height:14px;place-items:center;border:0;border-radius:999px;color:white;background:#c8cfd6}.skill-switch:after{position:absolute;left:2px;width:10px;height:10px;border-radius:50%;background:white;box-shadow:0 1px 2px rgba(0,0,0,.18);content:"";transition:transform .16s}.skill-switch.on{background:var(--blue)}.skill-switch.on:after{transform:translateX(12px)}.skill-switch:focus-visible{outline:2px solid #7ea4ee;outline-offset:2px}.state{display:grid;min-height:180px;place-items:center;align-content:center;gap:6px;color:var(--muted);text-align:center}.state h3{color:#303b45;font-size:13px}.state p{max-width:380px;font-size:9px}.standalone{flex:1;margin-top:8px;border:1px solid var(--border);border-radius:6px;background:white}
  .preview-panel{display:flex;flex-direction:column}.preview-header{display:grid;flex:none;grid-template-columns:minmax(110px,1fr) auto auto auto;gap:7px;align-items:center;min-height:38px;padding:4px 8px;border-bottom:1px solid var(--border);background:#fafbfc}.preview-title{display:flex;min-width:0;align-items:baseline;gap:5px}.preview-title h3{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:13px}.source-link{display:inline-flex;min-width:0;align-items:center;gap:3px;border:0;border-radius:4px;padding:2px 3px;color:var(--muted);background:transparent}.source-link:hover{color:#174a9e;background:#eaf1fb}.source-link:focus-visible{outline:2px solid #7ea4ee}.source-link span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:8px}.source-link :global(svg){flex:none}.preview-header>span{border:1px solid #d6dce2;border-radius:999px;padding:2px 6px;color:#737e88;background:white;font-size:8px;white-space:nowrap}.preview-header>span.enabled{border-color:#b7dec6;color:#24643d;background:#effaf3}.preview-header time{color:#818b94;font-size:8px;white-space:nowrap}.path-button{display:flex;min-width:0;align-items:center;gap:4px;border:0;border-radius:5px;padding:3px 5px;color:#667482;background:transparent;text-align:left;white-space:nowrap}.path-button:hover{color:#174a9e;background:#eaf1fb}.path-button:focus-visible{outline:2px solid #7ea4ee}.path-button span{font-size:8px}.path-button :global(svg){flex:none}.preview-body{display:flex;min-height:0;flex:1;flex-direction:column}.preview-scroll{min-height:0;flex:1;overflow:auto}.metadata-table{width:calc(100% - 24px);margin:10px 12px 0;border-collapse:collapse;color:#303942;background:#fafbfc;font-size:10px}.metadata-table th,.metadata-table td{border:1px solid var(--border);padding:5px 7px;text-align:left;vertical-align:top}.metadata-table th{width:92px;color:#596570;background:#f3f5f7;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-weight:650}.metadata-table td{overflow-wrap:anywhere}.markdown{padding:10px 12px;color:#303942;background:transparent;font-family:inherit;font-size:12px}
  .backdrop{position:fixed;inset:0;z-index:20;display:grid;place-items:center;padding:16px;background:rgba(18,25,33,.42)}.dialog{width:min(480px,100%);overflow:hidden;border:1px solid #cfd5dc;border-radius:7px;background:white;box-shadow:0 20px 48px rgba(17,24,39,.2)}.dialog>header{display:flex;justify-content:space-between;gap:12px;padding:11px 13px 8px;border-bottom:1px solid #e5e8eb}.dialog h2{font-size:15px}.dialog header p{margin-top:2px;color:var(--muted);font-size:9px}.dialog form{padding:12px 13px 0}.dialog label{display:grid;gap:5px}.dialog label span{font-size:11px;font-weight:650}.dialog input{height:32px;border:1px solid #cbd2d9;border-radius:6px;padding:0 9px;outline:none}.dialog label small{color:var(--muted);font-size:9px}.dialog footer{display:flex;justify-content:flex-end;gap:6px;margin-top:12px;padding:9px 13px;border-top:1px solid #e7eaed;background:#fafbfc}.dialog form footer{margin-right:-13px;margin-left:-13px}.dialog-progress{display:grid;place-items:center;gap:6px;padding:18px 16px;text-align:center}.dialog-progress span,.dialog-progress p{color:var(--muted);font-size:9px}.dialog-progress .progress-track{width:100%}
  .document-inline{display:flex;min-height:220px;flex:0 0 clamp(220px,42%,480px);flex-direction:column;border-top:1px solid #cdd5dd;background:white;box-shadow:0 -5px 14px rgba(35,48,61,.06)}.document-inline>header{display:grid;flex:none;grid-template-columns:minmax(0,1fr) auto;align-items:center;gap:4px;min-height:32px;padding:2px 6px;border-bottom:1px solid var(--border);background:#f6f8fa}.document-path{display:block;min-width:0;overflow:hidden;border:0;border-radius:4px;padding:3px 4px;color:#53606c;background:transparent;font-size:var(--font-aux);text-align:left;white-space:nowrap}.document-path:not(.pending):hover{color:#174a9e;background:#eaf1fb}.document-path:focus-visible{outline:2px solid #7ea4ee}.document-path span{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;direction:rtl;text-align:left;unicode-bidi:plaintext}.document-path.pending{color:var(--muted)}.document-content{min-height:0;flex:1;overflow:auto}.document-content .markdown{padding:10px 12px}.document-content .state{min-height:100%}.document-error{color:var(--red)}.document-error h3{color:#8f2f2f}
  .nav-list button{grid-template-columns:18px minmax(0,1fr)}.sidebar-bottom{display:block}.document-divider{min-width:0;min-height:7px;flex:0 0 7px}.document-inline{min-height:180px;flex-basis:clamp(180px,var(--document-size,50%),calc(100% - 187px))}.document-actions{display:flex;flex:none;align-items:center;gap:2px}.preview-body.document-right{flex-direction:row}.document-right>.preview-scroll{min-width:0}.document-right>.document-divider{min-width:7px;min-height:0}.document-right>.document-inline{min-width:140px;min-height:0;flex-basis:clamp(140px,var(--document-size,50%),calc(100% - 147px));border-top:0;border-left:1px solid #cdd5dd;box-shadow:-5px 0 14px rgba(35,48,61,.06)}
  .brand h1{font-size:var(--font-brand)}.nav-list span{font-size:var(--font-nav)}.title h2{font-size:var(--font-page-title)}.metrics strong{font-size:var(--font-metric)}.metrics span{font-size:var(--font-aux)}.primary,.secondary,.filter-toggle{font-size:var(--font-control)}.progress-copy{font-size:var(--font-aux)}.progress-copy strong{font-size:var(--font-control)}.message{font-size:var(--font-control)}.batch-errors{font-size:var(--font-aux)}.list-section h3{font-size:var(--font-panel-title)}.search input{font-size:var(--font-control)}.source-name strong{font-size:var(--font-body)}.source-name span{font-size:var(--font-aux)}.skill-copy strong{font-size:var(--font-body)}.skill-copy span{font-size:var(--font-aux)}.state h3{font-size:calc(var(--font-panel-title) + 2px)}.state p{font-size:var(--font-aux)}.preview-title h3{font-size:calc(var(--font-panel-title) + 2px)}.source-link span,.preview-header time,.path-button span{font-size:var(--font-aux)}.preview-header>span{font-size:var(--font-aux)}.metadata-table{font-size:var(--font-control)}.markdown{font-size:var(--font-body)}.dialog h2{font-size:var(--font-dialog-title)}.dialog header p,.dialog label small,.dialog-progress span,.dialog-progress p{font-size:var(--font-aux)}.dialog label span{font-size:var(--font-editor)}
  :global(.spin){animation:spin .8s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}@keyframes progress{0%{transform:translateX(-120%)}100%{transform:translateX(280%)}}
  @media(max-width:1040px){.topbar{grid-template-columns:1fr auto}.metrics{grid-column:1/-1;grid-row:2}.top-actions{grid-column:2;grid-row:1}}
  @media(max-width:760px){.window-drag-region{width:60px!important}.app-shell{grid-template-columns:60px minmax(0,1fr)}.app-divider,.panel-divider{display:none}.sidebar{padding-inline:6px}.brand{grid-template-columns:1fr;padding-inline:4px}.brand>div:last-child,.nav-list span,.settings-nav span{display:none}.brand-mark{margin:auto}.sidebar-bottom{display:block}.nav-list button,.settings-nav{width:100%;grid-template-columns:1fr;place-items:center}.workspace{padding-left:10px;overflow:auto}.content-grid{display:flex;min-height:auto;flex:initial;flex-direction:column}.list-section{height:290px;flex:none}.preview-panel{min-height:480px}.topbar{grid-template-columns:1fr}.top-actions{grid-column:1;grid-row:3;flex-wrap:wrap}.metrics{grid-column:1;grid-row:2}.preview-header{grid-template-columns:minmax(110px,1fr) auto auto}.path-button{grid-column:1/-1;grid-row:2}}
  @media(max-width:520px){.window-drag-region{width:100%;height:28px}.app-shell{display:block;height:auto;overflow:visible}.sidebar{display:grid;height:auto;grid-template-columns:auto 1fr auto;gap:4px;padding:36px 6px 4px;border-right:0;border-bottom:1px solid var(--border)}.brand{padding:0}.nav-list{grid-template-columns:repeat(4,1fr)}.sidebar-bottom{display:block;margin:0}.settings-nav{width:32px;height:32px;padding:4px}.workspace{height:auto;min-height:calc(100vh - 73px);padding:6px}.metrics{gap:9px}.top-actions{display:grid;grid-template-columns:1fr 1fr}.top-actions button:last-child{grid-column:1/-1}.list-section>header{grid-template-columns:1fr}.markdown{padding:10px}.progress-copy>div span{display:none}}
  @media(prefers-reduced-motion:reduce){:global(.spin),.progress-track.indeterminate span,.progress-track.indeterminate i{animation:none}}
</style>
