<script lang="ts">
  import { AlertTriangle, Bot, Check, Circle, Copy, LoaderCircle, RefreshCw, Search, X } from "@lucide/svelte";
  import { onMount } from "svelte";
  import { agentsApi } from "./api";
  import type { AgentRecord, AgentsDashboard } from "./types";

  let dashboard: AgentsDashboard | null = $state(null);
  let loading = $state(true);
  let refreshing = $state(false);
  let error = $state("");
  let notice = $state("");
  let query = $state("");

  let installedCount = $derived.by(() => {
    const agents: AgentRecord[] = dashboard ? dashboard.agents : [];
    return agents.filter((agent) => agent.installed).length;
  });

  let filteredAgents = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle) return dashboard?.agents ?? [];
    return (dashboard?.agents ?? []).filter((agent) =>
      searchableValues(agent).some((value) => value.toLocaleLowerCase().includes(needle)),
    );
  });

  onMount(() => void loadDashboard(false));

  async function loadDashboard(isRefresh: boolean) {
    if (isRefresh) refreshing = true;
    else loading = true;
    error = "";
    try {
      dashboard = await agentsApi.dashboard();
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      loading = false;
      refreshing = false;
    }
  }

  function searchableValues(agent: AgentRecord) {
    return [
      agent.name,
      agent.source,
      agent.projectSkillsDir,
      agent.globalSkillsDir,
      agent.projectMcpConfigFile,
      agent.globalMcpConfigFile,
      String(agent.supportsGenericProject),
      String(agent.supportsGenericGlobal),
    ];
  }

  async function copyPath(value: string) {
    if (!value) return;
    try {
      await navigator.clipboard.writeText(value);
      notice = "路径已复制";
      error = "";
    } catch (reason) {
      error = `复制路径失败：${messageOf(reason)}`;
    }
  }

  function messageOf(reason: unknown) {
    return reason instanceof Error ? reason.message : String(reason);
  }
</script>

<header class="agents-topbar" data-tauri-drag-region>
  <div class="title"><h2>本地 Agents</h2></div>
  <div class="metrics">
    <div><strong>{dashboard?.agents.length ?? 0}</strong><span>个 Agents</span></div>
    <div><strong>{installedCount}</strong><span>个已安装</span></div>
  </div>
  <div class="top-actions">
    <button class="secondary" onclick={() => void loadDashboard(true)} disabled={loading || refreshing}>
      {#if refreshing}<LoaderCircle class="spin" size={16} />{:else}<RefreshCw size={16} />{/if}
      刷新
    </button>
  </div>
</header>

{#if error}
  <div class="message error" role="alert">
    <AlertTriangle size={17} /><span>{error}</span>
    <button aria-label="关闭错误" onclick={() => (error = "")}><X size={16} /></button>
  </div>
{/if}
{#if notice}
  <div class="message success" role="status">
    <Check size={17} /><span>{notice}</span>
    <button aria-label="关闭提示" onclick={() => (notice = "")}><X size={16} /></button>
  </div>
{/if}

<section class="agents-panel">
  <header>
    <label class="search">
      <Search size={16} />
      <input bind:value={query} placeholder="搜索名称、Skills 或 MCP 路径" aria-label="搜索本地 Agents" />
      {#if query}<button aria-label="清空搜索" title="清空搜索" onclick={() => (query = "")}><X size={14} /></button>{/if}
    </label>
  </header>

  {#if loading}
    <div class="state"><LoaderCircle class="spin" size={24} /><p>正在检测本地 Agents…</p></div>
  {:else if !dashboard?.gaal.installed}
    <div class="state"><Bot size={30} /><h3>尚未安装 GAAL</h3><p>请先在设置中安装 GAAL，再刷新本地 Agents。</p></div>
  {:else if !dashboard.agents.length}
    <div class="state"><Bot size={30} /><h3>没有可用的 Agent</h3><p>GAAL 当前没有返回已注册的编码 Agent。</p></div>
  {:else if !filteredAgents.length}
    <div class="state"><Search size={28} /><h3>没有匹配结果</h3><button class="secondary" onclick={() => (query = "")}>清空搜索</button></div>
  {:else}
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th scope="col">名称</th>
            <th scope="col">安装状态</th>
            <th scope="col">来源</th>
            <th scope="col">项目 Skills 目录</th>
            <th scope="col">全局 Skills 目录</th>
            <th scope="col">项目 MCP 配置</th>
            <th scope="col">全局 MCP 配置</th>
            <th scope="col">使用通用项目目录</th>
            <th scope="col">使用通用全局目录</th>
          </tr>
        </thead>
        <tbody>
          {#each filteredAgents as agent (agent.name)}
            <tr>
              <th scope="row"><span class="agent-name"><Bot size={15} />{agent.name}</span></th>
              <td>
                <span class="status" class:installed={agent.installed}>
                  {#if agent.installed}<Check size={13} />已安装{:else}<Circle size={12} />未安装{/if}
                </span>
              </td>
              <td><code>{agent.source}</code></td>
              <td>{@render PathCell(agent.projectSkillsDir, copyPath)}</td>
              <td>{@render PathCell(agent.globalSkillsDir, copyPath)}</td>
              <td>{@render PathCell(agent.projectMcpConfigFile, copyPath)}</td>
              <td>{@render PathCell(agent.globalMcpConfigFile, copyPath)}</td>
              <td><code class="boolean">{agent.supportsGenericProject}</code></td>
              <td><code class="boolean">{agent.supportsGenericGlobal}</code></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

{#snippet PathCell(value: string, onCopy: (value: string) => Promise<void>)}
  {#if value}
    <button class="copy-path" title={`复制 ${value}`} aria-label={`复制路径 ${value}`} onclick={() => void onCopy(value)}>
      <code>{value}</code><Copy size={13} />
    </button>
  {:else}
    <code class="empty-path">-</code>
  {/if}
{/snippet}

<style>
  :global(*){box-sizing:border-box}:global(button),:global(input){font:inherit;letter-spacing:0}button{cursor:pointer}button:disabled{cursor:not-allowed;opacity:.52}
  .agents-topbar{display:grid;grid-template-columns:minmax(140px,1fr) auto auto;gap:12px;align-items:center;padding:0 1px 6px;border-bottom:1px solid var(--border);user-select:none}.title h2{margin:0;font-size:var(--font-page-title)}.metrics{display:flex;gap:12px}.metrics div{display:flex;align-items:baseline;gap:4px}.metrics strong{font-size:var(--font-metric)}.metrics span{color:var(--muted);font-size:var(--font-aux)}.top-actions{display:flex;justify-content:flex-end}.secondary{display:inline-flex;min-height:28px;align-items:center;justify-content:center;gap:4px;border:1px solid #cbd3dc;border-radius:6px;padding:4px 8px;color:#46515d;background:white;font-size:var(--font-control);font-weight:650;white-space:nowrap}.secondary:hover{background:#f5f7f9}
  .message{display:grid;grid-template-columns:16px 1fr 22px;gap:6px;align-items:center;margin-top:6px;border:1px solid;border-radius:6px;padding:4px 7px;font-size:var(--font-control)}.message.error{border-color:#efcaca;color:#922d2d;background:#fff3f3}.message.success{border-color:#b9ddc6;color:#25603b;background:#f0faf3}.message>button{display:grid;width:22px;height:22px;place-items:center;border:0;color:inherit;background:transparent}
  .agents-panel{display:flex;min-height:0;flex:1;flex-direction:column;margin-top:8px;overflow:hidden;border:1px solid var(--border);border-radius:6px;background:white}.agents-panel>header{display:flex;min-height:40px;align-items:center;padding:6px 8px;border-bottom:1px solid var(--border);background:#fafbfc}.search{display:flex;width:min(420px,100%);min-width:0;align-items:center;gap:5px;border:1px solid #ccd3da;border-radius:5px;padding:0 7px;color:#7b8791;background:white}.search:focus-within{border-color:#7ea4ee;box-shadow:0 0 0 2px rgba(37,99,235,.1)}.search input{width:100%;height:26px;border:0;outline:0;background:transparent;font-size:var(--font-control)}.search button{display:grid;width:22px;height:22px;flex:none;place-items:center;border:0;border-radius:4px;color:#707c87;background:transparent}.search button:hover{background:#eef1f4}
  .table-scroll{min-height:0;flex:1;overflow:auto}table{width:1430px;border-collapse:collapse;table-layout:fixed;color:#34404b}thead{position:sticky;top:0;z-index:1;background:#f5f7f9}th,td{height:40px;border-bottom:1px solid #e5e9ed;padding:6px 9px;text-align:left;vertical-align:middle}thead th{height:36px;color:#65717c;font-size:var(--font-aux);font-weight:700;white-space:nowrap}tbody th{font-weight:650}tbody tr:hover{background:#f8fafb}thead th:nth-child(1){width:150px}thead th:nth-child(2){width:105px}thead th:nth-child(3){width:90px}thead th:nth-child(4){width:180px}thead th:nth-child(5){width:200px}thead th:nth-child(6){width:200px}thead th:nth-child(7){width:230px}thead th:nth-child(8),thead th:nth-child(9){width:135px}.agent-name,.status{display:inline-flex;align-items:center;gap:5px;white-space:nowrap}.agent-name{font-size:var(--font-body)}.agent-name :global(svg){color:#52616e}.status{border:1px solid #d5dbe1;border-radius:999px;padding:2px 6px;color:#68737e;background:#f7f8f9;font-size:var(--font-aux)}.status.installed{border-color:#b9ddc6;color:#25603b;background:#f0faf3}code{display:block;overflow:hidden;color:#46515d;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:var(--font-control);line-height:1.45;text-overflow:ellipsis;white-space:nowrap}.copy-path{display:grid;width:100%;grid-template-columns:minmax(0,1fr) 15px;gap:4px;align-items:center;border:0;border-radius:4px;padding:3px 4px;color:#46515d;background:transparent;text-align:left}.copy-path:hover{color:#174a9e;background:#eaf1fb}.copy-path:focus-visible{outline:2px solid #7ea4ee;outline-offset:1px}.copy-path :global(svg){color:#78838d}.copy-path:hover :global(svg){color:#174a9e}.empty-path{color:#8a949e}.boolean{color:#174a9e}
  .state{display:grid;min-height:180px;flex:1;place-items:center;align-content:center;gap:7px;color:var(--muted);text-align:center}.state h3,.state p{margin:0}.state h3{color:#303b45;font-size:var(--font-panel-title)}.state p{max-width:430px;font-size:var(--font-aux)}:global(.spin){animation:spin .8s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}
  @media(max-width:620px){.agents-topbar{grid-template-columns:1fr auto}.metrics{grid-column:1/-1;grid-row:2}.agents-panel{min-height:520px;flex:none}.search{width:100%}}
  @media(prefers-reduced-motion:reduce){:global(.spin){animation:none}}
</style>
