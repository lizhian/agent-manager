<script lang="ts">
  import {
    AlertTriangle, Check, Eye, FileCode2, FolderOpen, GripVertical, LoaderCircle,
    Pencil, Plus, Search, Trash2, X,
  } from "@lucide/svelte";
  import { onMount, tick } from "svelte";
  import { openUrl } from "@tauri-apps/plugin-opener";
  import Markdown from "svelte-exmarkdown";
  import { gfmPlugin } from "svelte-exmarkdown/gfm";
  import rehypeSlug from "rehype-slug";
  import "github-markdown-css/github-markdown-light.css";
  import VerticalResizeHandle from "$lib/components/VerticalResizeHandle.svelte";
  import { agentsMdApi } from "./api";
  import type { AgentsMdDashboard, AgentsMdFragment } from "./types";

  type EditorMode = "preview" | "combined" | "create" | "edit";
  type DropPosition = "before" | "after";
  const markdownPlugins = [gfmPlugin(), { rehypePlugin: rehypeSlug }];

  let dashboard: AgentsMdDashboard | null = $state(null);
  let loading = $state(true);
  let saving = $state(false);
  let togglingId = $state<number | null>(null);
  let error = $state("");
  let notice = $state("");
  let query = $state("");
  let enabledOnly = $state(false);
  let selectedId = $state<number | null>(null);
  let editorMode: EditorMode = $state("combined");
  let draftTitle = $state("");
  let draftContent = $state("");
  let deleteTarget: AgentsMdFragment | null = $state(null);
  let draggedId = $state<number | null>(null);
  let dragPointerId = $state<number | null>(null);
  let dropTargetId = $state<number | null>(null);
  let dropPosition: DropPosition | null = $state(null);
  let dragX = $state(0);
  let dragY = $state(0);
  let dragWidth = $state(0);
  let listScroller: HTMLDivElement | null = $state(null);
  let listWidth = $state(0);
  let contentGrid: HTMLElement | null = $state(null);
  let listSection: HTMLElement | null = $state(null);
  let titleInput: HTMLInputElement | null = $state(null);

  let enabledCount = $derived.by(() => {
    const fragments: AgentsMdFragment[] = dashboard ? dashboard.fragments : [];
    return fragments.filter((item) => item.enabled).length;
  });
  let selected = $derived.by(() => {
    const fragments: AgentsMdFragment[] = dashboard ? dashboard.fragments : [];
    return fragments.find((item) => item.id === selectedId) ?? null;
  });
  let draggedFragment = $derived.by(() => {
    const fragments: AgentsMdFragment[] = dashboard ? dashboard.fragments : [];
    return fragments.find((item) => item.id === draggedId) ?? null;
  });
  let filteredFragments = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    return (dashboard?.fragments ?? []).filter((fragment) =>
      (!enabledOnly || fragment.enabled) &&
      (!needle || fragment.title.toLocaleLowerCase().includes(needle) ||
      fragment.content.toLocaleLowerCase().includes(needle)),
    );
  });

  onMount(loadDashboard);

  async function loadDashboard() {
    loading = true;
    error = "";
    try {
      applyDashboard(await agentsMdApi.dashboard());
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      loading = false;
    }
  }

  function applyDashboard(next: AgentsMdDashboard, preferredId: number | null = selectedId) {
    dashboard = next;
    const exists = preferredId !== null && next.fragments.some((item) => item.id === preferredId);
    selectedId = exists ? preferredId : (next.fragments[0]?.id ?? null);
  }

  function selectFragment(id: number) {
    selectedId = id;
    editorMode = "preview";
  }

  function openCombinedPreview() {
    selectedId = null;
    editorMode = "combined";
  }

  function clearListFilters() {
    query = "";
    enabledOnly = false;
  }

  function resizeList(delta: number) {
    if (!contentGrid || !listSection) return;
    const gridWidth = contentGrid.getBoundingClientRect().width;
    if (!gridWidth) return;
    const currentWidth = listSection.getBoundingClientRect().width;
    const nextWidth = Math.min(
      Math.max(270, gridWidth - 327),
      Math.max(270, currentWidth + delta),
    );
    listWidth = nextWidth / gridWidth * 100;
  }

  function startCreate() {
    draftTitle = "";
    draftContent = "# ";
    editorMode = "create";
    void tick().then(() => titleInput?.focus());
  }

  function startEdit(fragment: AgentsMdFragment) {
    selectedId = fragment.id;
    draftTitle = fragment.title;
    draftContent = fragment.content;
    editorMode = "edit";
    void tick().then(() => titleInput?.focus());
  }

  function cancelEdit() {
    editorMode = "preview";
    draftTitle = "";
    draftContent = "";
  }

  async function saveFragment() {
    if (!draftTitle.trim() || !draftContent.trim()) return;
    saving = true;
    error = "";
    try {
      if (editorMode === "create") {
        const next = await agentsMdApi.create(draftTitle, draftContent);
        const created = next.fragments[next.fragments.length - 1];
        applyDashboard(next, created?.id ?? null);
        notice = "片段已新增";
      } else if (editorMode === "edit" && selected) {
        applyDashboard(await agentsMdApi.update(selected.id, draftTitle, draftContent), selected.id);
        notice = "片段已保存，AGENTS.md 已同步";
      }
      cancelEdit();
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      saving = false;
    }
  }

  async function toggleEnabled(event: MouseEvent, fragment: AgentsMdFragment) {
    event.stopPropagation();
    togglingId = fragment.id;
    error = "";
    try {
      applyDashboard(
        await agentsMdApi.setEnabled(fragment.id, !fragment.enabled),
        fragment.id,
      );
      notice = fragment.enabled ? "片段已禁用，AGENTS.md 已重新生成" : "片段已启用，AGENTS.md 已重新生成";
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      togglingId = null;
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    saving = true;
    error = "";
    try {
      applyDashboard(await agentsMdApi.delete(deleteTarget.id));
      notice = "片段已删除，AGENTS.md 已同步";
      deleteTarget = null;
      editorMode = "preview";
    } catch (reason) {
      error = messageOf(reason);
    } finally {
      saving = false;
    }
  }

  function beginPointerDrag(event: PointerEvent, id: number) {
    if (event.button !== 0 || query || enabledOnly || saving || togglingId !== null) return;
    event.preventDefault();
    event.stopPropagation();
    const handle = event.currentTarget;
    if (!(handle instanceof HTMLElement)) return;
    const row = handle.closest<HTMLElement>(".fragment-row");
    draggedId = id;
    dragPointerId = event.pointerId;
    dragX = event.clientX;
    dragY = event.clientY;
    dragWidth = row?.getBoundingClientRect().width ?? 240;
  }

  function updatePointerDrag(event: PointerEvent) {
    if (draggedId === null || dragPointerId !== event.pointerId) return;
    event.preventDefault();
    dragX = event.clientX;
    dragY = event.clientY;
    autoScrollDuringDrag(event.clientY);

    const row = document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>(".fragment-row");
    const id = Number(row?.dataset.fragmentId);
    if (!row || !Number.isInteger(id) || id === draggedId) {
      dropTargetId = null;
      dropPosition = null;
      return;
    }
    const bounds = row.getBoundingClientRect();
    dropTargetId = id;
    dropPosition = event.clientY < bounds.top + bounds.height / 2 ? "before" : "after";
  }

  async function finishPointerDrag(event: PointerEvent) {
    if (dragPointerId !== event.pointerId) return;
    event.preventDefault();
    const sourceId = draggedId;
    const targetId = dropTargetId;
    const position = dropPosition;
    clearDragState();
    if (sourceId === null || targetId === null || sourceId === targetId || position === null) return;
    await moveFragment(sourceId, targetId, position);
  }

  async function moveFragment(sourceId: number, targetId: number, position: DropPosition) {
    if (!dashboard) return;
    const fragments = dashboard.fragments;
    const from = fragments.findIndex((item) => item.id === sourceId);
    if (from < 0) return;
    const reordered = [...fragments];
    const [moved] = reordered.splice(from, 1);
    const targetIndex = reordered.findIndex((item) => item.id === targetId);
    if (targetIndex < 0) return;
    reordered.splice(targetIndex + (position === "after" ? 1 : 0), 0, moved);
    const previous = dashboard;
    dashboard = { ...previous, fragments: reordered };
    saving = true;
    error = "";
    try {
      applyDashboard(await agentsMdApi.reorder(reordered.map((item) => item.id)), sourceId);
      notice = "片段顺序已保存，AGENTS.md 已重新生成";
    } catch (reason) {
      dashboard = previous;
      error = messageOf(reason);
    } finally {
      saving = false;
    }
  }

  async function moveWithKeyboard(fragment: AgentsMdFragment, direction: -1 | 1) {
    if (query || enabledOnly) return;
    const fragments = dashboard?.fragments ?? [];
    const index = fragments.findIndex((item) => item.id === fragment.id);
    const target = fragments[index + direction];
    if (target) await moveFragment(fragment.id, target.id, direction < 0 ? "before" : "after");
  }

  function clearDragState() {
    draggedId = null;
    dragPointerId = null;
    dropTargetId = null;
    dropPosition = null;
    dragWidth = 0;
  }

  function cancelPointerDrag(event: PointerEvent) {
    if (dragPointerId === event.pointerId) clearDragState();
  }

  function autoScrollDuringDrag(pointerY: number) {
    if (!listScroller) return;
    const bounds = listScroller.getBoundingClientRect();
    if (pointerY < bounds.top + 36) listScroller.scrollBy({ top: -12 });
    else if (pointerY > bounds.bottom - 36) listScroller.scrollBy({ top: 12 });
  }

  async function openFolder() {
    error = "";
    try {
      await agentsMdApi.openFolder();
    } catch (reason) {
      error = `打开 AGENTS.md 文件夹失败：${messageOf(reason)}`;
    }
  }

  function markdownLinks(node: HTMLElement) {
    const handleClick = async (event: MouseEvent) => {
      const target = event.target instanceof Element ? event.target.closest("a") : null;
      if (!(target instanceof HTMLAnchorElement)) return;
      const href = target.getAttribute("href")?.trim();
      if (!href) return;
      if (href.startsWith("#")) {
        event.preventDefault();
        const heading = node.querySelector<HTMLElement>(`#${CSS.escape(decodeAnchor(href.slice(1)))}`);
        heading?.scrollIntoView({
          behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth",
          block: "start",
        });
      } else if (/^https?:\/\//i.test(href)) {
        event.preventDefault();
        await openUrl(href);
      }
    };
    node.addEventListener("click", handleClick);
    return { destroy: () => node.removeEventListener("click", handleClick) };
  }

  function decodeAnchor(value: string) {
    try { return decodeURIComponent(value); } catch { return value; }
  }

  function formatTime(value: string) {
    if (!value) return "";
    const numeric = Number(value);
    const date = Number.isFinite(numeric) ? new Date(numeric * 1000) : new Date(value);
    return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", {
      month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit",
    }).format(date);
  }

  function formatCount(value: number) {
    return new Intl.NumberFormat("zh-CN").format(value);
  }

  function messageOf(reason: unknown) {
    return reason instanceof Error ? reason.message : String(reason);
  }
</script>

<svelte:window
  onpointermove={updatePointerDrag}
  onpointerup={(event) => void finishPointerDrag(event)}
  onpointercancel={cancelPointerDrag}
  onblur={clearDragState}
/>

<header class="topbar" data-tauri-drag-region>
  <div class="title"><h2>AGENTS.md</h2></div>
  <div class="metrics">
    <div><strong>{dashboard?.fragments.length ?? 0}</strong><span>个片段</span></div>
    <div><strong>{enabledCount}</strong><span>个已启用 · 共 {formatCount(dashboard?.enabledContentChars ?? 0)} 字符</span></div>
    <div><strong class:generated={enabledCount > 0}>{enabledCount > 0 ? "已生成" : "未生成"}</strong><span>全局文件</span></div>
  </div>
  <div class="top-actions">
    <button class="primary" onclick={startCreate} disabled={saving}><Plus size={16} />新增片段</button>
    <button class="secondary" onclick={openFolder}><FolderOpen size={16} />AGENTS.md 文件夹</button>
  </div>
</header>

{#if error}<div class="message error"><AlertTriangle size={17} /><span>{error}</span><button aria-label="关闭" onclick={() => (error = "")}><X size={16} /></button></div>{/if}
{#if notice}<div class="message success"><Check size={17} /><span>{notice}</span><button aria-label="关闭" onclick={() => (notice = "")}><X size={16} /></button></div>{/if}

<section class="content-grid" bind:this={contentGrid} style:--list-width={`${listWidth}%`}>
  <section class="list-section" bind:this={listSection}>
    <header>
      <label class="search"><Search size={16} /><input bind:value={query} placeholder="搜索片段" /></label>
      <button class="filter-toggle" class:on={enabledOnly} role="switch" aria-checked={enabledOnly} onclick={() => (enabledOnly = !enabledOnly)} title={enabledOnly ? "显示全部片段" : "只显示已启用片段"}><span>只看启用</span><i></i></button>
      <button class="secondary preview-button" class:active={editorMode === "combined"} onclick={openCombinedPreview}><Eye size={15} />预览</button>
    </header>
    <div class="list-scroll" bind:this={listScroller}>
      {#if loading}
        <div class="state"><LoaderCircle class="spin" size={24} /><p>正在读取片段…</p></div>
      {:else if !dashboard?.fragments.length}
        <div class="state"><FileCode2 size={30} /><h3>还没有片段</h3><p>新增 Markdown 片段后，可按顺序组合为全局 AGENTS.md。</p><button class="primary" onclick={startCreate}><Plus size={16} />新增第一个片段</button></div>
      {:else if !filteredFragments.length}
        <div class="state"><Search size={28} /><h3>{enabledOnly && !query ? "没有已启用片段" : "没有匹配结果"}</h3><button class="secondary" onclick={clearListFilters}>{enabledOnly ? "显示全部片段" : "清空搜索"}</button></div>
      {:else}
        <div class="fragment-list">
          {#each filteredFragments as fragment (fragment.id)}
            <div
              class="fragment-row"
              class:selected={selectedId === fragment.id}
              class:dragging={draggedId === fragment.id}
              class:drop-before={dropTargetId === fragment.id && dropPosition === "before"}
              class:drop-after={dropTargetId === fragment.id && dropPosition === "after"}
              data-fragment-id={fragment.id}
              role="button"
              tabindex="0"
              onclick={() => selectFragment(fragment.id)}
              onkeydown={(event) => (event.key === "Enter" || event.key === " ") && selectFragment(fragment.id)}
            >
              <button
                class="drag-handle"
                class:dragging={draggedId === fragment.id}
                title={query || enabledOnly ? "显示全部片段并清空搜索后可调整顺序" : "拖动调整顺序；方向键可移动"}
                aria-label={`调整 ${fragment.title} 的顺序`}
                disabled={!!query || enabledOnly || saving}
                onclick={(event) => event.stopPropagation()}
                onpointerdown={(event) => beginPointerDrag(event, fragment.id)}
                onkeydown={(event) => {
                  if (event.key === "ArrowUp") { event.preventDefault(); void moveWithKeyboard(fragment, -1); }
                  if (event.key === "ArrowDown") { event.preventDefault(); void moveWithKeyboard(fragment, 1); }
                }}
              ><GripVertical size={15} /></button>
              <span class="fragment-copy"><strong>{fragment.title}</strong><span>{formatTime(fragment.updatedAt)}</span></span>
              <button
                class="fragment-switch"
                class:on={fragment.enabled}
                role="switch"
                aria-checked={fragment.enabled}
                aria-label={`${fragment.title}${fragment.enabled ? "禁用" : "启用"}`}
                disabled={saving || togglingId !== null}
                onclick={(event) => toggleEnabled(event, fragment)}
              >{#if togglingId === fragment.id}<LoaderCircle class="spin" size={11} />{/if}</button>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </section>

  <div class="panel-divider"><VerticalResizeHandle label="调整片段列表宽度" onResize={resizeList} onReset={() => (listWidth = 0)} /></div>

  <section class="preview-panel">
    {#if editorMode === "create" || editorMode === "edit"}
      <form class="editor" onsubmit={(event) => { event.preventDefault(); void saveFragment(); }}>
        <header>
          <div><h3>{editorMode === "create" ? "新增片段" : "修改片段"}</h3><span>Markdown</span></div>
          <div><button type="button" class="secondary" onclick={cancelEdit}>取消</button><button type="submit" class="primary" disabled={saving || !draftTitle.trim() || !draftContent.trim()}>{#if saving}<LoaderCircle class="spin" size={14} />{/if}保存</button></div>
        </header>
        <label><span>片段名称</span><input bind:this={titleInput} bind:value={draftTitle} maxlength="120" placeholder="例如：通用代码规范" /></label>
        <label class="content-field"><span>Markdown 内容</span><textarea bind:value={draftContent} spellcheck="false" placeholder="# 标题&#10;&#10;在这里输入规则…"></textarea></label>
      </form>
    {:else if editorMode === "combined"}
      <header class="preview-header combined-header">
        <div class="preview-title"><h3>合并后的 AGENTS.md</h3><span>按当前启用状态和片段顺序生成</span></div>
        <span class="preview-count">{enabledCount} 个片段</span>
      </header>
      <div class="preview-scroll">
        {#if dashboard?.combinedContent}
          <div class="markdown markdown-body" role="document" use:markdownLinks><Markdown md={dashboard.combinedContent} plugins={markdownPlugins} /></div>
        {:else}
          <div class="state"><FileCode2 size={30} /><h3>没有可预览的内容</h3><p>启用片段后，合并后的 AGENTS.md 将显示在这里。</p></div>
        {/if}
      </div>
    {:else if selected}
      <header class="preview-header">
        <div class="preview-title"><h3>{selected.title}</h3><span>{selected.enabled ? "已加入全局 AGENTS.md" : "未启用"}</span></div>
        <time>{formatTime(selected.updatedAt)}</time>
        <button class="secondary compact" onclick={() => startEdit(selected)} disabled={saving}><Pencil size={14} />修改</button>
        <button class="danger compact" onclick={() => (deleteTarget = selected)} disabled={saving}><Trash2 size={14} />删除</button>
      </header>
      <div class="preview-scroll">
        <div class="markdown markdown-body" role="document" use:markdownLinks><Markdown md={selected.content} plugins={markdownPlugins} /></div>
      </div>
    {:else}
      <div class="state"><FileCode2 size={30} /><h3>选择一个片段</h3><p>Markdown 内容将在这里显示。</p></div>
    {/if}
  </section>
</section>

{#if draggedFragment}
  <div
    class="drag-preview"
    style:left={`${dragX + 12}px`}
    style:top={`${dragY + 10}px`}
    style:width={`${Math.min(dragWidth, 320)}px`}
    aria-hidden="true"
  ><GripVertical size={15} /><strong>{draggedFragment.title}</strong></div>
{/if}

{#if deleteTarget}
  <div class="backdrop" role="presentation" onclick={(event) => event.currentTarget === event.target && !saving && (deleteTarget = null)} onkeydown={(event) => event.key === "Escape" && !saving && (deleteTarget = null)}>
    <div class="dialog" role="alertdialog" aria-modal="true" aria-labelledby="delete-fragment-title">
      <header><div><h2 id="delete-fragment-title">删除片段</h2><p>删除后会立即重新生成全局 AGENTS.md。</p></div><button class="icon-button" aria-label="关闭" onclick={() => (deleteTarget = null)} disabled={saving}><X size={18} /></button></header>
      <div class="delete-copy"><AlertTriangle size={20} /><p>确定删除“<strong>{deleteTarget.title}</strong>”吗？此操作无法撤销。</p></div>
      <footer><button class="secondary" onclick={() => (deleteTarget = null)} disabled={saving}>取消</button><button class="danger" onclick={confirmDelete} disabled={saving}>{#if saving}<LoaderCircle class="spin" size={14} />{/if}确认删除</button></footer>
    </div>
  </div>
{/if}

<style>
  h2,h3,p{margin:0}button,input,textarea{font:inherit;letter-spacing:0}button{cursor:pointer}button:disabled{cursor:not-allowed;opacity:.52}
  .topbar{display:grid;grid-template-columns:minmax(140px,1fr) auto auto;gap:12px;align-items:center;padding:0 1px 6px;border-bottom:1px solid var(--border);user-select:none}.topbar>.title,.topbar>.metrics{pointer-events:none}.title h2{font-size:17px}.metrics{display:flex;gap:12px}.metrics div{display:flex;gap:3px;align-items:baseline;white-space:nowrap}.metrics strong{font-size:15px}.metrics strong.generated{color:#24643d}.metrics span{color:var(--muted);font-size:9px}.top-actions{display:flex;gap:4px}.primary,.secondary,.danger{display:inline-flex;min-height:var(--control-height);align-items:center;justify-content:center;gap:4px;border-radius:5px;padding:4px 8px;font-size:10px;font-weight:650;white-space:nowrap}.primary{border:1px solid #1f58d8;color:white;background:var(--blue)}.secondary{border:1px solid #ccd3da;color:#35414c;background:white}.danger{border:1px solid #d13b3b;color:white;background:#c93434}.compact{min-height:26px;padding:3px 6px}.preview-header .danger{border-color:#e1bcbc;color:#a42b2b;background:#fff7f7}
  .message{display:grid;flex:none;grid-template-columns:15px 1fr 22px;gap:5px;align-items:center;margin-top:6px;border:1px solid;border-radius:6px;padding:4px 7px;font-size:10px}.message.error{border-color:#efcaca;color:#922d2d;background:#fff3f3}.message.success{border-color:#bfe1cd;color:#24643d;background:#effaf3}.message button{display:grid;width:22px;height:22px;place-items:center;border:0;color:inherit;background:transparent}
  .content-grid{display:grid;min-height:0;flex:1;grid-template-columns:minmax(270px,min(var(--list-width,0%),calc(100% - 327px))) 7px minmax(320px,1fr);gap:0;margin-top:var(--panel-gap)}.panel-divider{min-width:0}.list-section,.preview-panel{min-height:0;overflow:hidden;border:1px solid var(--border);border-radius:6px;background:white}.list-section{display:flex;flex-direction:column}.list-section>header{display:grid;grid-template-columns:minmax(0,1fr) auto auto;gap:6px;align-items:center;padding:6px 8px;border-bottom:1px solid var(--border);background:#fafbfc}.search{display:flex;min-width:0;align-items:center;gap:5px;border:1px solid #ccd3da;border-radius:5px;padding:0 7px;color:#7b8791;background:white}.search:focus-within{border-color:#7ea4ee;box-shadow:0 0 0 2px rgba(37,99,235,.1)}.search input{width:100%;height:26px;border:0;outline:0;background:transparent;font-size:10px}.filter-toggle{display:inline-flex;min-height:var(--control-height);align-items:center;justify-content:center;gap:4px;border:1px solid #ccd3da;border-radius:5px;padding:4px 7px;color:#35414c;background:white;font-weight:650;white-space:nowrap}.filter-toggle i{position:relative;width:26px;height:14px;border-radius:999px;background:#c8cfd6;transition:background .16s}.filter-toggle i:after{position:absolute;top:2px;left:2px;width:10px;height:10px;border-radius:50%;background:white;box-shadow:0 1px 2px rgba(0,0,0,.18);content:"";transition:transform .16s}.filter-toggle.on{border-color:#a9c2ef;color:#174a9e;background:#f3f7ff}.filter-toggle.on i{background:var(--blue)}.filter-toggle.on i:after{transform:translateX(12px)}.preview-button.active{border-color:#a9c2ef;color:#174a9e;background:#eaf1ff}.list-scroll{min-height:0;flex:1;overflow:auto}
  .fragment-row{position:relative;display:grid;grid-template-columns:26px minmax(0,1fr) 28px;gap:4px;align-items:center;min-height:40px;padding:3px 6px;border-bottom:1px solid #e8ebee;background:white;cursor:pointer;transition:background .12s,opacity .12s}.fragment-row:hover{background:#f7f9fb}.fragment-row.selected{color:#174a9e;background:#edf4ff}.fragment-row.dragging{opacity:.38;background:#f0f4f8}.fragment-row.drop-before:before,.fragment-row.drop-after:after{position:absolute;z-index:2;right:6px;left:6px;height:2px;border-radius:2px;background:var(--blue);box-shadow:0 0 0 1px rgba(255,255,255,.9);content:""}.fragment-row.drop-before:before{top:-1px}.fragment-row.drop-after:after{bottom:-1px}.fragment-row:focus-visible{outline:2px solid #7ea4ee;outline-offset:-2px}.drag-handle{display:grid;width:26px;height:26px;place-items:center;border:0;border-radius:5px;color:#8a949e;background:transparent;cursor:grab;touch-action:none}.drag-handle:hover{color:#4c5965;background:#e9edf1}.drag-handle:active,.drag-handle.dragging{color:#174a9e;background:#dbe8fb;cursor:grabbing}.drag-preview{position:fixed;z-index:40;display:flex;min-width:160px;max-width:320px;align-items:center;gap:6px;overflow:hidden;border:1px solid #8fb0ea;border-radius:6px;padding:7px 9px;color:#174a9e;background:rgba(247,250,255,.96);box-shadow:0 8px 22px rgba(25,55,100,.18);pointer-events:none}.drag-preview strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:var(--font-body)}.fragment-copy{display:grid;min-width:0;gap:1px}.fragment-copy strong,.fragment-copy span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.fragment-copy strong{font-size:10px}.fragment-copy span{color:#7a858f;font-size:8px}.fragment-switch{position:relative;display:grid;width:26px;height:14px;place-items:center;border:0;border-radius:999px;color:white;background:#c8cfd6}.fragment-switch:after{position:absolute;left:2px;width:10px;height:10px;border-radius:50%;background:white;box-shadow:0 1px 2px rgba(0,0,0,.18);content:"";transition:transform .16s}.fragment-switch.on{background:var(--blue)}.fragment-switch.on:after{transform:translateX(12px)}
  .preview-panel{display:flex;flex-direction:column}.preview-header{display:grid;flex:none;grid-template-columns:minmax(110px,1fr) auto auto auto;gap:6px;align-items:center;min-height:38px;padding:4px 8px;border-bottom:1px solid var(--border);background:#fafbfc}.combined-header{grid-template-columns:minmax(110px,1fr) auto}.preview-title{display:flex;min-width:0;align-items:baseline;gap:5px}.preview-title h3,.preview-title span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.preview-title h3{font-size:13px}.preview-title span,.preview-header time{color:var(--muted);font-size:8px;white-space:nowrap}.preview-count{border:1px solid #d6dce2;border-radius:999px;padding:2px 6px;color:#66717c;background:white;white-space:nowrap}.preview-scroll{min-height:0;flex:1;overflow:auto}.markdown{padding:10px 12px;color:#303942;background:transparent;font-family:inherit;font-size:12px}
  .editor{display:flex;min-height:0;flex:1;flex-direction:column}.editor>header{display:flex;flex:none;justify-content:space-between;align-items:center;gap:8px;min-height:38px;padding:4px 8px;border-bottom:1px solid var(--border);background:#fafbfc}.editor header>div{display:flex;align-items:baseline;gap:5px}.editor h3{font-size:13px}.editor header span{color:var(--muted);font-size:8px}.editor label{display:grid;flex:none;gap:4px;padding:7px 9px 0}.editor label>span{font-size:9px;font-weight:650}.editor input,.editor textarea{width:100%;border:1px solid #ccd3da;border-radius:5px;padding:6px 8px;outline:0;background:white;font-size:11px}.editor input:focus,.editor textarea:focus{border-color:#7ea4ee;box-shadow:0 0 0 2px rgba(37,99,235,.1)}.editor .content-field{grid-template-rows:auto minmax(0,1fr);min-height:0;flex:1;padding-bottom:9px}.editor textarea{min-height:0;height:100%;resize:none;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;line-height:1.5}
  .state{display:grid;min-height:180px;place-items:center;align-content:center;gap:6px;color:var(--muted);text-align:center}.state h3{color:#303b45;font-size:13px}.state p{max-width:380px;font-size:9px}.backdrop{position:fixed;inset:0;z-index:24;display:grid;place-items:center;padding:16px;background:rgba(18,25,33,.42)}.dialog{width:min(440px,100%);overflow:hidden;border:1px solid #cfd5dc;border-radius:7px;background:white;box-shadow:0 20px 48px rgba(17,24,39,.2)}.dialog>header{display:flex;justify-content:space-between;gap:12px;padding:10px 12px 8px;border-bottom:1px solid #e5e8eb}.dialog h2{font-size:15px}.dialog header p{margin-top:2px;color:var(--muted);font-size:9px}.icon-button{display:grid;width:26px;height:26px;place-items:center;border:1px solid transparent;border-radius:5px;color:#65717c;background:transparent}.delete-copy{display:flex;gap:8px;align-items:flex-start;padding:13px 12px;color:#6f3333;font-size:11px}.delete-copy :global(svg){flex:none}.dialog footer{display:flex;justify-content:flex-end;gap:6px;padding:8px 12px;border-top:1px solid #e7eaed;background:#fafbfc}
  .title h2{font-size:var(--font-page-title)}.metrics strong{font-size:var(--font-metric)}.metrics span{font-size:var(--font-aux)}.primary,.secondary,.danger,.filter-toggle{font-size:var(--font-control)}.message{font-size:var(--font-control)}.search input{font-size:var(--font-control)}.fragment-copy strong{font-size:var(--font-body)}.fragment-copy span{font-size:var(--font-aux)}.preview-title h3,.editor h3{font-size:calc(var(--font-panel-title) + 2px)}.preview-title span,.preview-header time,.editor header span,.preview-count{font-size:var(--font-aux)}.markdown{font-size:var(--font-body)}.editor label>span{font-size:var(--font-aux)}.editor input,.editor textarea{font-size:var(--font-editor)}.state h3{font-size:calc(var(--font-panel-title) + 2px)}.state p{font-size:var(--font-aux)}.dialog h2{font-size:calc(var(--font-dialog-title) - 1px)}.dialog header p{font-size:var(--font-aux)}.delete-copy{font-size:var(--font-editor)}
  :global(.spin){animation:spin .8s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}
  @media(max-width:1040px){.topbar{grid-template-columns:1fr auto}.metrics{grid-column:1/-1;grid-row:2}.top-actions{grid-column:2;grid-row:1}}
  @media(max-width:760px){.content-grid{display:flex;min-height:auto;flex:initial;flex-direction:column}.panel-divider{display:none}.list-section{height:290px;flex:none}.preview-panel{min-height:480px}.topbar{grid-template-columns:1fr}.top-actions{grid-column:1;grid-row:3;flex-wrap:wrap}.metrics{grid-column:1;grid-row:2}.preview-header{grid-template-columns:minmax(100px,1fr) auto}.preview-header .compact{grid-row:2}.preview-header .compact:last-child{grid-column:2}}
  @media(max-width:520px){.metrics{gap:9px}.top-actions{display:grid;grid-template-columns:1fr 1fr}.list-section>header{grid-template-columns:minmax(0,1fr) auto}.preview-button{grid-column:1/-1}.markdown{padding:10px}.editor>header{align-items:flex-start;flex-direction:column}}
  @media(prefers-reduced-motion:reduce){:global(.spin){animation:none}}
</style>
