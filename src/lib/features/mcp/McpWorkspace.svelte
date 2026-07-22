<script lang="ts">
  import {
    AlertTriangle, Bug, Check, LoaderCircle, Pencil, Play, Plus, RefreshCw, Search, Server,
    Trash2, Users, Wrench, X,
  } from "@lucide/svelte";
  import VerticalResizeHandle from "$lib/components/VerticalResizeHandle.svelte";
  import { onMount, tick } from "svelte";
  import { mcpApi } from "./api";
  import type {
    McpDashboard, McpServer, McpServerInput, McpTool, McpToolCallOutput, McpTransport,
  } from "./types";

  type Dialog = "server" | "agents" | "delete-server" | null;
  type Pair = { key: string; value: string };

  let dashboard: McpDashboard | null = $state(null);
  let loading = $state(true);
  let saving = $state(false);
  let syncing = $state(false);
  let togglingServerId = $state<number | null>(null);
  let error = $state("");
  let notice = $state("");
  let query = $state("");
  let enabledOnly = $state(false);
  let selectedServerId = $state<number | null>(null);
  let dialog: Dialog = $state(null);
  let editingServerId = $state<number | null>(null);
  let serverName = $state("");
  let transport: McpTransport = $state("stdio");
  let command = $state("");
  let argsText = $state("");
  let url = $state("");
  let envPairs = $state<Pair[]>([]);
  let headerPairs = $state<Pair[]>([]);
  let draftAgents = $state<string[]>([]);
  let firstInput: HTMLInputElement | null = $state(null);
  let listWidth = $state(0);
  let contentGrid: HTMLElement | null = $state(null);
  let listSection: HTMLElement | null = $state(null);

  let debugTools = $state<McpTool[]>([]);
  let debugLoading = $state(false);
  let debugError = $state("");
  let selectedToolName = $state("");
  let toolArguments = $state("{}");
  let toolResult = $state<McpToolCallOutput | null>(null);
  let invokingTool = $state(false);

  let selectedServer = $derived.by(() =>
    dashboard?.servers.find((server) => server.id === selectedServerId) ?? null,
  );
  let selectedTool = $derived.by(() =>
    debugTools.find((tool) => tool.name === selectedToolName) ?? null,
  );
  let enabledCount = $derived.by(() =>
    (dashboard?.servers ?? []).filter((server) => server.enabled).length,
  );
  let filteredServers = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    return (dashboard?.servers ?? []).filter((server) => {
      if (enabledOnly && !server.enabled) return false;
      if (!needle) return true;
      const searchable = [
        server.name,
        server.transport,
        server.command,
        server.url,
        ...server.args,
        ...Object.keys(server.env),
        ...Object.keys(server.headers),
      ];
      return searchable.some((value) => value.toLocaleLowerCase().includes(needle));
    });
  });
  let targetLabel = $derived.by(() => {
    const agents = dashboard?.defaultAgents ?? [];
    if (agents.includes("*")) return "全部 Agent";
    if (agents.length === 1) return agents[0];
    return `${agents.length} 个 Agent`;
  });

  onMount(loadDashboard);

  async function loadDashboard() {
    loading = true;
    error = "";
    try {
      applyDashboard(await mcpApi.dashboard());
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      loading = false;
    }
  }

  function applyDashboard(next: McpDashboard) {
    dashboard = next;
    const selectedExists = next.servers.some((server) => server.id === selectedServerId);
    const nextId = selectedExists ? selectedServerId : (next.servers[0]?.id ?? null);
    if (nextId !== selectedServerId) resetDebugger();
    selectedServerId = nextId;
  }

  function selectServer(id: number) {
    if (id !== selectedServerId) resetDebugger();
    selectedServerId = id;
  }

  function clearListFilters() {
    query = "";
    enabledOnly = false;
  }

  function clamp(value: number, minimum: number, maximum: number) {
    return Math.min(maximum, Math.max(minimum, value));
  }

  function resizeList(delta: number) {
    if (!contentGrid || !listSection) return;
    const gridWidth = contentGrid.getBoundingClientRect().width;
    if (!gridWidth) return;
    const currentWidth = listSection.getBoundingClientRect().width;
    const nextWidth = clamp(currentWidth + delta, 250, Math.max(250, gridWidth - 327));
    listWidth = nextWidth / gridWidth * 100;
  }

  function openCreateServer() {
    editingServerId = null;
    serverName = "";
    transport = "stdio";
    command = "";
    argsText = "";
    url = "";
    envPairs = [];
    headerPairs = [];
    dialog = "server";
    focusFirstInput();
  }

  function openEditServer(server: McpServer) {
    editingServerId = server.id;
    serverName = server.name;
    transport = server.transport;
    command = server.command;
    argsText = server.args.join("\n");
    url = server.url;
    envPairs = objectToPairs(server.env);
    headerPairs = objectToPairs(server.headers);
    dialog = "server";
    focusFirstInput();
  }

  function openAgentDialog() {
    draftAgents = [...(dashboard?.defaultAgents ?? ["codex"])];
    dialog = "agents";
  }

  function focusFirstInput() {
    void tick().then(() => firstInput?.focus());
  }

  async function saveServer() {
    if (!serverName.trim()) return;
    saving = true;
    error = "";
    const input: McpServerInput = {
      name: serverName.trim(),
      transport,
      command: command.trim(),
      args: argsText.split("\n").map((value) => value.trim()).filter(Boolean),
      env: pairsToObject(envPairs),
      url: url.trim(),
      headers: pairsToObject(headerPairs),
    };
    try {
      const next = editingServerId === null
        ? await mcpApi.createServer(input)
        : await mcpApi.updateServer(editingServerId, input);
      applyDashboard(next);
      notice = editingServerId === null ? "MCP 已新增" : "MCP 已更新";
      dialog = null;
      resetDebugger();
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      saving = false;
    }
  }

  async function saveDefaultAgents() {
    if (!draftAgents.length) return;
    saving = true;
    error = "";
    try {
      applyDashboard(await mcpApi.setDefaultAgents(draftAgents));
      notice = "目标 Agent 已更新";
      dialog = null;
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      saving = false;
    }
  }

  async function toggleServer(server: McpServer) {
    togglingServerId = server.id;
    error = "";
    try {
      applyDashboard(await mcpApi.setEnabled(server.id, !server.enabled));
      notice = server.enabled ? "MCP 已停用" : "MCP 已启用";
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      togglingServerId = null;
    }
  }

  async function syncServers() {
    syncing = true;
    error = "";
    try {
      applyDashboard(await mcpApi.sync());
      notice = "MCP 已分发";
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      syncing = false;
    }
  }

  async function confirmDeleteServer() {
    if (!selectedServer) return;
    saving = true;
    try {
      applyDashboard(await mcpApi.deleteServer(selectedServer.id));
      notice = "MCP 已删除";
      dialog = null;
      resetDebugger();
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      saving = false;
    }
  }

  function toggleAgent(agent: string) {
    if (agent === "*") {
      draftAgents = ["*"];
      return;
    }
    const next = new Set(draftAgents.filter((item) => item !== "*"));
    next.has(agent) ? next.delete(agent) : next.add(agent);
    draftAgents = [...next];
  }

  async function loadTools() {
    if (!selectedServer) return;
    debugLoading = true;
    debugError = "";
    toolResult = null;
    try {
      const snapshot = await mcpApi.inspectTools(selectedServer.id);
      debugTools = snapshot.tools;
      selectedToolName = snapshot.tools[0]?.name ?? "";
      toolArguments = "{}";
    } catch (reason) {
      debugTools = [];
      selectedToolName = "";
      debugError = messageOf(reason);
    } finally {
      debugLoading = false;
    }
  }

  function chooseTool(name: string) {
    selectedToolName = name;
    toolArguments = "{}";
    toolResult = null;
    debugError = "";
  }

  async function callTool() {
    if (!selectedServer || !selectedTool) return;
    let parsed: unknown;
    try {
      parsed = JSON.parse(toolArguments);
    } catch (reason) {
      debugError = `解析调用参数失败：${messageOf(reason)}`;
      return;
    }
    if (!isRecord(parsed)) {
      debugError = "调用参数必须是 JSON 对象";
      return;
    }
    invokingTool = true;
    debugError = "";
    toolResult = null;
    try {
      toolResult = await mcpApi.callTool(selectedServer.id, selectedTool.name, parsed);
    } catch (reason) {
      debugError = messageOf(reason);
    } finally {
      invokingTool = false;
    }
  }

  function resetDebugger() {
    debugTools = [];
    debugError = "";
    selectedToolName = "";
    toolArguments = "{}";
    toolResult = null;
  }

  function objectToPairs(value: Record<string, string>): Pair[] {
    return Object.entries(value).map(([key, item]) => ({ key, value: item }));
  }

  function pairsToObject(pairs: Pair[]): Record<string, string> {
    return Object.fromEntries(pairs.map((pair) => [pair.key.trim(), pair.value]).filter(([key]) => key));
  }

  function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value);
  }

  function prettyJson(value: unknown) {
    return JSON.stringify(value, null, 2);
  }

  function messageOf(reason: unknown) {
    return reason instanceof Error ? reason.message : String(reason);
  }
</script>

<header class="mcp-topbar" data-tauri-drag-region>
  <div><h2>MCP 管理</h2></div>
  <div class="metrics"><div><strong>{dashboard?.servers.length ?? 0}</strong><span>MCP</span></div><div><strong>{enabledCount}</strong><span>启用</span></div></div>
  <div class="top-actions">
    <button class="secondary" onclick={openCreateServer}><Plus size={15} />新增 MCP</button>
    <button class="secondary" onclick={openAgentDialog}><Users size={15} />目标 Agent：{targetLabel}</button>
    <button class="primary" disabled={syncing || !dashboard?.servers.some((server) => server.enabled)} onclick={syncServers}>{#if syncing}<LoaderCircle class="spin" size={15} />{:else}<RefreshCw size={15} />{/if}{syncing ? "分发中" : "分发到 Agent"}</button>
  </div>
</header>

{#if error}<div class="message error"><AlertTriangle size={16} /><span>{error}</span><button aria-label="关闭" onclick={() => (error = "")}><X size={15} /></button></div>{/if}
{#if notice}<div class="message notice"><Check size={16} /><span>{notice}</span><button aria-label="关闭" onclick={() => (notice = "")}><X size={15} /></button></div>{/if}

{#if loading}
  <div class="state"><LoaderCircle class="spin" size={28} /><h3>正在读取 MCP 配置</h3></div>
{:else if !dashboard?.gaal.installed}
  <div class="state bordered"><Server size={30} /><h3>需要安装 GAAL</h3><p>请先在设置中安装 GAAL。</p></div>
{:else if !dashboard.servers.length}
  <div class="state bordered"><Server size={30} /><h3>还没有 MCP</h3><button class="primary" onclick={openCreateServer}><Plus size={15} />新增 MCP</button></div>
{:else}
  <section class="mcp-layout" bind:this={contentGrid} style:--list-width={`${listWidth}%`}>
    <section class="server-list-panel" bind:this={listSection}>
      <header>
        <label class="search"><Search size={16} /><input bind:value={query} placeholder="搜索 MCP" /></label>
        <button class="filter-toggle" class:on={enabledOnly} role="switch" aria-checked={enabledOnly} onclick={() => (enabledOnly = !enabledOnly)} title={enabledOnly ? "显示全部 MCP" : "只显示已启用 MCP"}><span>只看启用</span><i></i></button>
      </header>
      <div class="server-list">
        {#if !filteredServers.length}
          <div class="list-empty"><Search size={26} /><h3>{enabledOnly && !query ? "没有已启用 MCP" : "没有匹配结果"}</h3><button class="secondary" onclick={clearListFilters}>{enabledOnly ? "显示全部 MCP" : "清空搜索"}</button></div>
        {:else}
          {#each filteredServers as server (server.id)}
            <div class="server-row" class:selected={server.id === selectedServerId} role="button" tabindex="0" onclick={() => selectServer(server.id)} onkeydown={(event) => (event.key === "Enter" || event.key === " ") && selectServer(server.id)}>
              <span><strong>{server.name}</strong><small>{server.transport}</small></span>
              <button class="switch" class:on={server.enabled} aria-label={server.enabled ? "停用 MCP" : "启用 MCP"} disabled={togglingServerId !== null} onclick={(event) => { event.stopPropagation(); toggleServer(server); }}><span></span></button>
            </div>
          {/each}
        {/if}
      </div>
    </section>
    <div class="panel-divider"><VerticalResizeHandle label="调整 MCP 列表宽度" onResize={resizeList} onReset={() => (listWidth = 0)} /></div>
    <section class="detail-panel">
      {#if selectedServer}
        <header><div><h3>{selectedServer.name}</h3><span class:enabled={selectedServer.enabled}>{selectedServer.enabled ? "已启用" : "已停用"}</span></div><div><button class="icon-button" aria-label="编辑 MCP" title="编辑 MCP" onclick={() => openEditServer(selectedServer)}><Pencil size={15} /></button><button class="danger-icon" aria-label="删除 MCP" title="删除 MCP" onclick={() => (dialog = "delete-server")}><Trash2 size={15} /></button></div></header>
        <dl>
          <div><dt>传输</dt><dd>{selectedServer.transport}</dd></div>
          {#if selectedServer.transport === "stdio"}<div><dt>命令</dt><dd>{selectedServer.command}</dd></div><div><dt>参数</dt><dd>{selectedServer.args.join(" ") || "-"}</dd></div>{:else}<div><dt>URL</dt><dd>{selectedServer.url}</dd></div>{/if}
          <div><dt>环境变量</dt><dd>{Object.keys(selectedServer.env).join(", ") || "-"}</dd></div>
          <div><dt>请求头</dt><dd>{Object.keys(selectedServer.headers).join(", ") || "-"}</dd></div>
        </dl>
        <section class="debug-panel">
          <header><div><Bug size={16} /><h3>调试</h3></div><button class="secondary compact" disabled={debugLoading} onclick={loadTools}>{#if debugLoading}<LoaderCircle class="spin" size={14} />{:else}<Wrench size={14} />{/if}{debugLoading ? "读取中" : "读取 Tools"}</button></header>
          {#if debugError}<div class="debug-error"><AlertTriangle size={14} /><span>{debugError}</span></div>{/if}
          {#if debugTools.length}
            <div class="debug-workspace">
              <div class="tool-list" aria-label="Tools 列表">{#each debugTools as tool}<button class:selected={tool.name === selectedToolName} onclick={() => chooseTool(tool.name)}><strong>{tool.title || tool.name}</strong>{#if tool.title}<small>{tool.name}</small>{/if}</button>{/each}</div>
              {#if selectedTool}
                <div class="tool-call">
                  {#if selectedTool.description}<p>{selectedTool.description}</p>{/if}
                  <details><summary>输入 Schema</summary><pre>{prettyJson(selectedTool.inputSchema)}</pre></details>
                  <label><span>调用参数</span><textarea bind:value={toolArguments} rows="7" spellcheck="false"></textarea></label>
                  <div class="call-actions"><span>{toolResult ? `${toolResult.durationMs} ms` : ""}</span><button class="primary" disabled={invokingTool} onclick={callTool}>{#if invokingTool}<LoaderCircle class="spin" size={14} />{:else}<Play size={14} />{/if}{invokingTool ? "调用中" : "调用 Tool"}</button></div>
                  {#if toolResult}<div class="result"><strong>调用结果</strong><pre>{prettyJson(toolResult.result)}</pre></div>{/if}
                </div>
              {/if}
            </div>
          {:else if !debugLoading && !debugError}<div class="debug-empty">读取服务提供的 tools 后可模拟调用。</div>{/if}
        </section>
      {:else}<div class="state"><Server size={26} /><h3>选择一个 MCP</h3></div>{/if}
    </section>
  </section>
{/if}

{#if dialog === "server"}
  <div class="backdrop" role="presentation" onclick={(event) => event.currentTarget === event.target && !saving && (dialog = null)} onkeydown={(event) => event.key === "Escape" && !saving && (dialog = null)}>
    <form class="dialog server-dialog" onsubmit={(event) => { event.preventDefault(); saveServer(); }}>
      <header><h2>{editingServerId === null ? "新增 MCP" : "编辑 MCP"}</h2><button type="button" class="icon-button" aria-label="关闭" onclick={() => (dialog = null)}><X size={17} /></button></header>
      <div class="form-body two-column">
        <label class="wide"><span>名称</span><input bind:this={firstInput} bind:value={serverName} maxlength="80" required /></label>
        <fieldset class="transport-field wide"><legend>传输</legend><div class="segmented">{#each ["stdio", "http", "sse"] as option}<button type="button" class:active={transport === option} onclick={() => (transport = option as McpTransport)}>{option === "http" ? "HTTP" : option === "sse" ? "SSE" : "stdio"}</button>{/each}</div></fieldset>
        {#if transport === "stdio"}<label><span>命令</span><input bind:value={command} placeholder="npx / uvx / executable" required /></label><label><span>参数</span><textarea bind:value={argsText} rows="4" placeholder="每行一个参数"></textarea></label>{:else}<label class="wide"><span>URL</span><input bind:value={url} type="url" required /></label>{/if}
        <div class="pair-editor"><div><strong>环境变量</strong><button type="button" class="icon-button" aria-label="新增环境变量" onclick={() => (envPairs = [...envPairs, { key: "", value: "" }])}><Plus size={14} /></button></div>{#each envPairs as pair, index}<div class="pair-row"><input bind:value={pair.key} placeholder="KEY" /><input bind:value={pair.value} placeholder={'VALUE 或 ${VAR}'} /><button type="button" class="icon-button" aria-label="删除环境变量" onclick={() => (envPairs = envPairs.filter((_, item) => item !== index))}><X size={14} /></button></div>{/each}</div>
        {#if transport !== "stdio"}<div class="pair-editor"><div><strong>请求头</strong><button type="button" class="icon-button" aria-label="新增请求头" onclick={() => (headerPairs = [...headerPairs, { key: "", value: "" }])}><Plus size={14} /></button></div>{#each headerPairs as pair, index}<div class="pair-row"><input bind:value={pair.key} placeholder="Header" /><input bind:value={pair.value} placeholder="Value" /><button type="button" class="icon-button" aria-label="删除请求头" onclick={() => (headerPairs = headerPairs.filter((_, item) => item !== index))}><X size={14} /></button></div>{/each}</div>{/if}
      </div>
      <footer><button type="button" class="secondary" onclick={() => (dialog = null)}>取消</button><button class="primary" disabled={saving || !serverName.trim() || (transport === "stdio" ? !command.trim() : !url.trim())}>{#if saving}<LoaderCircle class="spin" size={14} />{/if}保存</button></footer>
    </form>
  </div>
{:else if dialog === "agents"}
  <div class="backdrop" role="presentation" onclick={(event) => event.currentTarget === event.target && !saving && (dialog = null)} onkeydown={(event) => event.key === "Escape" && !saving && (dialog = null)}>
    <form class="dialog agents-dialog" onsubmit={(event) => { event.preventDefault(); saveDefaultAgents(); }}>
      <header><h2>目标 Agent</h2><button type="button" class="icon-button" aria-label="关闭" onclick={() => (dialog = null)}><X size={17} /></button></header>
      <div class="agent-options">{#each dashboard?.availableAgents ?? [] as agent}<label><input type="checkbox" checked={draftAgents.includes(agent)} onchange={() => toggleAgent(agent)} /><span>{agent === "*" ? "全部" : agent}</span></label>{/each}</div>
      <footer><button type="button" class="secondary" onclick={() => (dialog = null)}>取消</button><button class="primary" disabled={saving || !draftAgents.length}>{#if saving}<LoaderCircle class="spin" size={14} />{/if}保存</button></footer>
    </form>
  </div>
{:else if dialog === "delete-server"}
  <div class="backdrop" role="presentation"><div class="dialog confirm"><header><h2>删除 MCP</h2></header><div class="confirm-body"><AlertTriangle size={22} /><span>确认删除“{selectedServer?.name}”？</span></div><footer><button class="secondary" onclick={() => (dialog = null)}>取消</button><button class="danger" disabled={saving} onclick={confirmDeleteServer}>{#if saving}<LoaderCircle class="spin" size={14} />{/if}删除</button></footer></div></div>
{/if}

<style>
  h2,h3,p{margin:0}button,input,textarea{font:inherit;letter-spacing:0}button{cursor:pointer}button:disabled{cursor:not-allowed;opacity:.5}.mcp-topbar{display:grid;grid-template-columns:minmax(140px,1fr) auto auto;gap:12px;align-items:center;padding:0 1px 6px;border-bottom:1px solid var(--border);user-select:none}.mcp-topbar h2{font-size:var(--font-page-title)}.metrics{display:flex;gap:12px}.metrics div{display:flex;align-items:baseline;gap:4px}.metrics strong{font-size:var(--font-metric)}.metrics span{color:var(--muted);font-size:var(--font-aux)}.top-actions{display:flex;gap:4px;justify-content:flex-end}.primary,.secondary,.danger,.filter-toggle{display:inline-flex;min-height:28px;align-items:center;justify-content:center;gap:4px;border-radius:6px;padding:4px 8px;font-size:var(--font-control);font-weight:650}.primary{border:1px solid #296bd6;color:white;background:#2563c7}.primary:hover{background:#1f56ad}.secondary{border:1px solid #cbd3dc;color:#46515d;background:white}.secondary:hover{background:#f5f7f9}.danger{border:1px solid #c64848;color:white;background:#b83d3d}.compact{min-height:26px;padding:3px 7px}.icon-button,.danger-icon{display:grid;width:26px;height:26px;place-items:center;border:1px solid transparent;border-radius:5px;background:transparent}.icon-button{color:#66717c}.icon-button:hover{border-color:#d3dae1;background:white}.danger-icon{color:#a33b3b}.danger-icon:hover{border-color:#e4bcbc;background:#fff2f2}.message{display:grid;grid-template-columns:16px 1fr 22px;gap:6px;align-items:center;margin-top:6px;border:1px solid;border-radius:6px;padding:4px 7px;font-size:var(--font-control)}.message.error{border-color:#efcaca;color:#922d2d;background:#fff3f3}.message.notice{border-color:#b9ddc6;color:#25603b;background:#f0faf3}.message>button{display:grid;width:22px;height:22px;place-items:center;border:0;color:inherit;background:transparent}.state,.list-empty{display:grid;min-height:180px;place-items:center;align-content:center;gap:7px;color:var(--muted);text-align:center}.state h3,.list-empty h3{color:#303b45;font-size:var(--font-panel-title)}.state p{max-width:430px;font-size:var(--font-aux)}.bordered{flex:1;margin-top:8px;border:1px solid var(--border);border-radius:6px;background:white}.mcp-layout{display:grid;min-height:0;flex:1;grid-template-columns:minmax(250px,min(var(--list-width,0%),calc(100% - 327px))) 7px minmax(320px,1fr);gap:0;margin-top:8px}.panel-divider{min-width:0}.server-list-panel,.detail-panel{min-height:0;overflow:hidden;border:1px solid var(--border);border-radius:6px;background:white}.server-list-panel{display:flex;min-width:0;flex-direction:column}.server-list-panel>header{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:6px;align-items:center;padding:6px 8px;border-bottom:1px solid var(--border);background:#fafbfc}.search{display:flex;min-width:0;align-items:center;gap:5px;border:1px solid #ccd3da;border-radius:5px;padding:0 7px;color:#7b8791;background:white}.search:focus-within{border-color:#7ea4ee;box-shadow:0 0 0 2px rgba(37,99,235,.1)}.search input{width:100%;height:26px;border:0;outline:0;background:transparent;font-size:var(--font-control)}.filter-toggle{border:1px solid #ccd3da;color:#35414c;background:white;white-space:nowrap}.filter-toggle i{position:relative;width:26px;height:14px;border-radius:999px;background:#c8cfd6;transition:background .16s}.filter-toggle i:after{position:absolute;top:2px;left:2px;width:10px;height:10px;border-radius:50%;background:white;box-shadow:0 1px 2px rgba(0,0,0,.18);content:"";transition:transform .16s}.filter-toggle.on{border-color:#a9c2ef;color:#174a9e;background:#f3f7ff}.filter-toggle.on i{background:var(--blue)}.filter-toggle.on i:after{transform:translateX(12px)}.server-list{min-height:0;overflow:auto}.server-row{display:grid;width:100%;grid-template-columns:minmax(0,1fr) 30px;gap:7px;align-items:center;min-height:44px;border-bottom:1px solid #e8ebee;padding:5px 8px;color:#35414c;background:white;text-align:left;cursor:pointer}.server-row:hover{background:#f7f9fb}.server-row.selected{color:#174a9e;background:#edf4ff}.server-row>span{display:grid;min-width:0;gap:2px}.server-row strong,.server-row small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.server-row strong{font-size:var(--font-body)}.server-row small{color:var(--muted);font-size:var(--font-aux)}.switch{position:relative;width:28px;height:16px;border:0;border-radius:999px;background:#c7cdd3}.switch span{position:absolute;top:2px;left:2px;width:12px;height:12px;border-radius:50%;background:white;box-shadow:0 1px 2px rgba(0,0,0,.2);transition:transform .16s}.switch.on{background:#2563c7}.switch.on span{transform:translateX(12px)}.detail-panel{min-width:0;overflow:auto}.detail-panel>header{display:flex;min-height:40px;align-items:center;justify-content:space-between;padding:4px 8px;border-bottom:1px solid var(--border);background:#fafbfc}.detail-panel>header>div{display:flex;align-items:center;gap:6px}.detail-panel>header span{border:1px solid #d3d9df;border-radius:999px;padding:2px 6px;color:#68737e;background:white;font-size:var(--font-aux)}.detail-panel>header span.enabled{border-color:#b7dec6;color:#24643d;background:#effaf3}.detail-panel dl{display:grid;margin:0}.detail-panel dl>div{display:grid;grid-template-columns:90px minmax(0,1fr);border-bottom:1px solid #eceff2;padding:7px 9px}.detail-panel dt{color:#66717c;font-size:var(--font-aux);font-weight:650}.detail-panel dd{margin:0;overflow-wrap:anywhere;color:#303b45;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:var(--font-control)}.debug-panel{border-top:1px solid #dfe4e8}.debug-panel>header{display:flex;min-height:40px;align-items:center;justify-content:space-between;padding:5px 8px;background:#fafbfc}.debug-panel>header>div{display:flex;align-items:center;gap:6px}.debug-error{display:flex;gap:6px;align-items:flex-start;border-top:1px solid #efcaca;padding:7px 9px;color:#922d2d;background:#fff3f3;font-size:var(--font-control)}.debug-workspace{display:grid;grid-template-columns:minmax(150px,30%) minmax(0,1fr);border-top:1px solid #e5e9ed}.tool-list{min-height:260px;max-height:480px;overflow:auto;border-right:1px solid #e5e9ed;background:#fafbfc}.tool-list button{display:grid;width:100%;gap:2px;border:0;border-bottom:1px solid #e8ebee;padding:6px 8px;color:#3d4853;background:transparent;text-align:left}.tool-list button:hover{background:#f2f5f7}.tool-list button.selected{color:#174a9e;background:#edf4ff}.tool-list strong,.tool-list small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.tool-list strong{font-size:var(--font-control)}.tool-list small{color:var(--muted);font-size:var(--font-aux)}.tool-call{display:grid;align-content:start;gap:7px;min-width:0;padding:8px}.tool-call>p{color:#5d6873;font-size:var(--font-control);line-height:1.45}.tool-call details{border:1px solid #dce1e6;border-radius:5px;background:#fafbfc}.tool-call summary{padding:5px 7px;color:#4e5964;font-size:var(--font-control);font-weight:650;cursor:pointer}.tool-call label{display:grid;gap:4px}.tool-call label span,.result>strong{font-size:var(--font-control);font-weight:650}.tool-call textarea{width:100%;min-width:0;resize:vertical;border:1px solid #cbd2d9;border-radius:5px;padding:6px 7px;outline:none;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:var(--font-control)}.tool-call textarea:focus{border-color:#7ea4ee;box-shadow:0 0 0 2px rgba(37,99,235,.1)}.call-actions{display:flex;align-items:center;justify-content:space-between}.call-actions>span{color:var(--muted);font-size:var(--font-aux)}pre{max-height:300px;margin:0;overflow:auto;padding:7px;color:#27323c;background:#f7f9fb;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:var(--font-aux);line-height:1.45;white-space:pre-wrap;word-break:break-word}.result{display:grid;gap:4px}.result pre{border:1px solid #dce1e6;border-radius:5px}.debug-empty{display:grid;min-height:100px;place-items:center;border-top:1px solid #e5e9ed;color:var(--muted);font-size:var(--font-control)}.backdrop{position:fixed;inset:0;z-index:30;display:grid;place-items:center;padding:16px;background:rgba(18,25,33,.42)}.dialog{width:min(500px,100%);max-height:calc(100vh - 32px);overflow:auto;border:1px solid #cfd5dc;border-radius:7px;background:white;box-shadow:0 20px 48px rgba(17,24,39,.2)}.server-dialog{width:min(700px,100%)}.agents-dialog{width:min(620px,100%)}.dialog>header{display:flex;min-height:42px;align-items:center;justify-content:space-between;gap:12px;padding:7px 11px;border-bottom:1px solid #e5e8eb}.dialog h2{font-size:var(--font-dialog-title)}.form-body{display:grid;gap:9px;padding:11px}.two-column{grid-template-columns:1fr 1fr}.wide{grid-column:1/-1}.form-body label{display:grid;gap:4px}.form-body label>span,.pair-editor strong{font-size:var(--font-control);font-weight:650}.form-body input,.form-body textarea{width:100%;min-width:0;border:1px solid #cbd2d9;border-radius:6px;padding:5px 7px;outline:none;color:#303942;background:white;font-size:var(--font-control)}.form-body input{height:30px}.form-body textarea{resize:vertical}.form-body input:focus,.form-body textarea:focus{border-color:#7ea4ee;box-shadow:0 0 0 2px rgba(37,99,235,.1)}.dialog footer{display:flex;justify-content:flex-end;gap:6px;padding:8px 11px;border-top:1px solid #e7eaed;background:#fafbfc}.transport-field{margin:0;border:0;padding:0}.transport-field legend{margin-bottom:4px;font-size:var(--font-control);font-weight:650}.segmented{display:grid;width:100%;grid-template-columns:repeat(3,1fr);border:1px solid #cbd2d9;border-radius:6px;overflow:hidden}.segmented button{min-height:30px;border:0;border-right:1px solid #cbd2d9;color:#53606c;background:white;font-size:var(--font-control);font-weight:650}.segmented button:last-child{border-right:0}.segmented button.active{color:#174a9e;background:#eaf2ff;box-shadow:inset 0 0 0 1px #8eb1ef}.pair-editor{display:grid;align-content:start;gap:4px}.pair-editor>div:first-child{display:flex;min-height:26px;align-items:center;justify-content:space-between}.pair-row{display:grid;grid-template-columns:minmax(80px,.7fr) minmax(100px,1.3fr) 26px;gap:4px}.agent-options{display:flex;flex-wrap:wrap;gap:5px;padding:11px}.agent-options label{display:flex;align-items:center;gap:4px;border:1px solid #d2d9df;border-radius:5px;padding:4px 7px;background:#fafbfc}.agent-options input{width:auto;height:auto}.agent-options span{font-size:var(--font-control);font-weight:550}.confirm{width:min(420px,100%)}.confirm-body{display:flex;gap:8px;align-items:flex-start;padding:14px 12px;color:#5f3333;font-size:var(--font-control)}:global(.spin){animation:spin .8s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}@media(max-width:900px){.mcp-topbar{grid-template-columns:1fr auto}.top-actions{grid-column:1/-1;flex-wrap:wrap}.mcp-layout{grid-template-columns:minmax(250px,min(var(--list-width,0%),calc(100% - 327px))) 7px minmax(320px,1fr)}.debug-workspace{grid-template-columns:1fr}.tool-list{display:flex;min-height:auto;max-height:140px;border-right:0;border-bottom:1px solid #e5e9ed}.tool-list button{min-width:150px;border-right:1px solid #e8ebee}}@media(max-width:620px){.mcp-topbar{grid-template-columns:1fr}.metrics{grid-row:2}.top-actions{justify-content:flex-start}.mcp-layout{display:flex;min-height:auto;flex:initial;flex-direction:column}.panel-divider{display:none}.server-list-panel{height:300px;flex:none}.server-list-panel>header{grid-template-columns:minmax(0,1fr) auto}.detail-panel{min-height:480px}.two-column{grid-template-columns:1fr}.wide{grid-column:1}.top-actions button{flex:1 1 auto}.pair-editor{grid-column:1}.debug-workspace{grid-template-columns:1fr}}@media(prefers-reduced-motion:reduce){:global(.spin),.switch span,.filter-toggle i,.filter-toggle i:after{animation:none;transition:none}}
</style>
