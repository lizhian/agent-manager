<script lang="ts">
  import {
    AlertTriangle, CaseSensitive, Check, ChevronDown, LoaderCircle, Search, Type, X,
  } from "@lucide/svelte";
  import { onMount, tick } from "svelte";
  import { settingsApi } from "./api";
  import type { FontSize } from "./types";

  type Props = {
    fontSize: FontSize;
    fontFamily: string;
    initialError?: string;
    onFontSizeChange: (fontSize: FontSize) => void;
    onFontFamilyChange: (fontFamily: string) => void;
  };

  const options: Array<{ value: FontSize; label: string; description: string }> = [
    { value: "extra-small", label: "特小", description: "正文 13px" },
    { value: "small", label: "小", description: "正文 14px" },
    { value: "standard", label: "标准", description: "默认字体大小，正文 15px" },
    { value: "large", label: "大", description: "正文 16px" },
    { value: "extra-large", label: "特大", description: "正文 17px" },
  ];

  let {
    fontSize, fontFamily, initialError = "", onFontSizeChange, onFontFamilyChange,
  }: Props = $props();
  let saving: FontSize | null = $state(null);
  let savingFontFamily = $state(false);
  let error = $state("");
  let systemFonts: string[] = $state([]);
  let fontsLoading = $state(true);
  let fontMenuOpen = $state(false);
  let fontQuery = $state("");
  let activeFontIndex = $state(0);
  let fontPicker: HTMLElement | null = $state(null);
  let fontInput: HTMLInputElement | null = $state(null);

  let fontOptions = $derived.by(() => {
    const options = [{ value: "", label: "跟随系统" }];
    const values = fontFamily && !systemFonts.includes(fontFamily)
      ? [fontFamily, ...systemFonts]
      : systemFonts;
    return options.concat(values.map((font) => ({ value: font, label: font })));
  });
  let filteredFontOptions = $derived.by(() => {
    const needle = fontQuery.trim().toLocaleLowerCase();
    const selectedLabel = fontFamily || "跟随系统";
    if (!fontMenuOpen || !needle || needle === selectedLabel.toLocaleLowerCase()) return fontOptions;
    return fontOptions.filter((option) => option.label.toLocaleLowerCase().includes(needle));
  });

  $effect(() => {
    if (initialError) error = initialError;
  });

  $effect(() => {
    if (!fontMenuOpen) fontQuery = fontFamily || "跟随系统";
  });

  onMount(loadSystemFonts);

  async function loadSystemFonts() {
    fontsLoading = true;
    try {
      systemFonts = await settingsApi.systemFonts();
    } catch (reason) {
      error = reason instanceof Error ? reason.message : String(reason);
    } finally {
      fontsLoading = false;
    }
  }

  async function selectFontSize(next: FontSize) {
    if (next === fontSize || saving) return;
    saving = next;
    error = "";
    try {
      const settings = await settingsApi.setFontSize(next);
      onFontSizeChange(settings.fontSize);
    } catch (reason) {
      error = reason instanceof Error ? reason.message : String(reason);
    } finally {
      saving = null;
    }
  }

  async function selectFontFamily(next: string) {
    if (savingFontFamily || next === fontFamily) {
      closeFontMenu();
      return;
    }
    savingFontFamily = true;
    error = "";
    try {
      const settings = await settingsApi.setFontFamily(next);
      onFontFamilyChange(settings.fontFamily);
      fontQuery = settings.fontFamily || "跟随系统";
      fontMenuOpen = false;
    } catch (reason) {
      error = reason instanceof Error ? reason.message : String(reason);
    } finally {
      savingFontFamily = false;
    }
  }

  function openFontMenu() {
    fontMenuOpen = true;
    activeFontIndex = Math.max(0, fontOptions.findIndex((option) => option.value === fontFamily));
    void tick().then(() => fontInput?.select());
  }

  function closeFontMenu() {
    fontMenuOpen = false;
    fontQuery = fontFamily || "跟随系统";
  }

  function toggleFontMenu() {
    if (fontMenuOpen) closeFontMenu();
    else openFontMenu();
    void tick().then(() => fontInput?.focus());
  }

  function handleFontInput() {
    fontMenuOpen = true;
    activeFontIndex = 0;
  }

  function handleFontKeydown(event: KeyboardEvent) {
    if (event.key === "Escape") {
      event.preventDefault();
      closeFontMenu();
      return;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!fontMenuOpen) openFontMenu();
      const direction = event.key === "ArrowDown" ? 1 : -1;
      const count = filteredFontOptions.length;
      if (count) activeFontIndex = (activeFontIndex + direction + count) % count;
      return;
    }
    if (event.key === "Enter" && fontMenuOpen) {
      event.preventDefault();
      const option = filteredFontOptions[activeFontIndex];
      if (option) void selectFontFamily(option.value);
    }
  }

  function handleWindowPointerDown(event: PointerEvent) {
    if (!fontMenuOpen || !(event.target instanceof Node) || fontPicker?.contains(event.target)) return;
    closeFontMenu();
  }

  function fontPreviewStyle(value: string) {
    return value ? `"${value.replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"` : "inherit";
  }
</script>

<svelte:window onpointerdown={handleWindowPointerDown} />

<header class="settings-topbar" data-tauri-drag-region>
  <h2>设置</h2>
</header>

{#if error}
  <div class="message error"><AlertTriangle size={17} /><span>{error}</span><button aria-label="关闭" onclick={() => (error = "")}><X size={16} /></button></div>
{/if}

<section class="settings-content">
  <section class="settings-panel" aria-labelledby="appearance-title">
    <header><h3 id="appearance-title">外观</h3></header>
    <div class="setting-row">
      <div class="setting-icon"><Type size={18} /></div>
      <div class="setting-copy"><strong>字体大小</strong><span>调整整个应用的文字大小，不改变界面间距。</span></div>
      <div class="segmented" aria-label="字体大小">
        {#each options as option}
          <button
            class:active={fontSize === option.value}
            aria-pressed={fontSize === option.value}
            title={option.description}
            disabled={saving !== null}
            onclick={() => selectFontSize(option.value)}
          >
            {#if saving === option.value}<LoaderCircle class="spin" size={13} />{:else if fontSize === option.value}<Check size={13} />{/if}
            {option.label}
          </button>
        {/each}
      </div>
    </div>
    <div class="setting-row font-family-row">
      <div class="setting-icon"><CaseSensitive size={19} /></div>
      <div class="setting-copy"><strong>字体</strong><span>自动读取本机已安装字体，支持输入名称搜索。</span></div>
      <div class="font-picker" bind:this={fontPicker}>
        <div class="font-combobox" class:open={fontMenuOpen}>
          <Search size={15} />
          <input
            bind:this={fontInput}
            bind:value={fontQuery}
            style:font-family={fontPreviewStyle(fontFamily)}
            role="combobox"
            aria-label="字体"
            aria-autocomplete="list"
            aria-expanded={fontMenuOpen}
            aria-controls="font-family-options"
            aria-activedescendant={fontMenuOpen && filteredFontOptions[activeFontIndex] ? `font-option-${activeFontIndex}` : undefined}
            disabled={fontsLoading || savingFontFamily}
            onfocus={openFontMenu}
            oninput={handleFontInput}
            onkeydown={handleFontKeydown}
          />
          {#if fontsLoading || savingFontFamily}<LoaderCircle class="spin" size={14} />{:else}<button type="button" aria-label="展开字体列表" tabindex="-1" onclick={toggleFontMenu}><ChevronDown size={15} /></button>{/if}
        </div>
        {#if fontMenuOpen}
          <div class="font-options" id="font-family-options" role="listbox" aria-label="系统字体">
            {#if filteredFontOptions.length}
              {#each filteredFontOptions as option, index (option.value)}
                <button
                  type="button"
                  id={`font-option-${index}`}
                  role="option"
                  aria-selected={fontFamily === option.value}
                  class:active={activeFontIndex === index}
                  class:selected={fontFamily === option.value}
                  style:font-family={fontPreviewStyle(option.value)}
                  onpointermove={() => (activeFontIndex = index)}
                  onclick={() => selectFontFamily(option.value)}
                >
                  <span>{option.label}</span>{#if fontFamily === option.value}<Check size={14} />{/if}
                </button>
              {/each}
            {:else}
              <div class="font-empty">没有匹配的字体</div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  </section>
</section>

<style>
  h2,h3{margin:0}button{font:inherit;letter-spacing:0;cursor:pointer}button:disabled{cursor:not-allowed;opacity:.52}
  .settings-topbar{display:flex;min-height:35px;align-items:center;padding:0 1px 6px;border-bottom:1px solid var(--border);user-select:none}.settings-topbar h2{font-size:var(--font-page-title);pointer-events:none}
  .message{display:grid;flex:none;grid-template-columns:15px 1fr 22px;gap:5px;align-items:center;margin-top:6px;border:1px solid;border-radius:6px;padding:4px 7px;font-size:var(--font-control)}.message.error{border-color:#efcaca;color:#922d2d;background:#fff3f3}.message button{display:grid;width:22px;height:22px;place-items:center;border:0;color:inherit;background:transparent}
  .settings-content{min-height:0;flex:1;padding-top:8px;overflow:auto}.settings-panel{position:relative;overflow:visible;border:1px solid var(--border);border-radius:6px;background:white}.settings-panel>header{padding:7px 10px;border-bottom:1px solid var(--border);border-radius:6px 6px 0 0;background:#fafbfc}.settings-panel h3{font-size:var(--font-panel-title)}.setting-row{display:grid;grid-template-columns:32px minmax(220px,1fr) auto;gap:8px;align-items:center;padding:10px}.setting-icon{display:grid;width:30px;height:30px;place-items:center;border-radius:6px;color:#53606c;background:#eef1f4}.setting-copy{display:grid;gap:2px}.setting-copy strong{font-size:var(--font-body)}.setting-copy span{color:var(--muted);font-size:var(--font-aux)}
  .segmented{display:flex;overflow:hidden;border:1px solid #cbd2d9;border-radius:6px;background:#f5f7f9}.segmented button{display:inline-flex;min-width:60px;min-height:28px;align-items:center;justify-content:center;gap:4px;border:0;border-right:1px solid #d8dde2;padding:4px 8px;color:#4c5864;background:transparent;font-size:var(--font-control);font-weight:650}.segmented button:last-child{border-right:0}.segmented button:hover{background:white}.segmented button.active{color:#174a9e;background:#eaf1ff}.segmented button:focus-visible{position:relative;z-index:1;outline:2px solid #7ea4ee;outline-offset:-2px}
  .font-family-row{border-top:1px solid #e7eaed}.font-picker{position:relative;width:300px}.font-combobox{display:grid;grid-template-columns:16px minmax(0,1fr) auto;gap:5px;align-items:center;height:32px;border:1px solid #cbd2d9;border-radius:6px;padding:0 5px 0 8px;color:#75808b;background:white}.font-combobox:focus-within,.font-combobox.open{border-color:#7ea4ee;box-shadow:0 0 0 2px rgba(37,99,235,.1)}.font-combobox input{min-width:0;height:28px;border:0;outline:0;background:transparent;color:#303942;font-size:var(--font-control)}.font-combobox>button{display:grid;width:24px;height:24px;place-items:center;border:0;border-radius:4px;color:#68737f;background:transparent}.font-combobox>button:hover{background:#eef1f4}.font-options{position:absolute;right:0;top:calc(100% + 4px);z-index:12;width:100%;max-height:280px;overflow:auto;border:1px solid #cbd2d9;border-radius:6px;padding:4px;background:white;box-shadow:0 12px 28px rgba(25,34,43,.16)}.font-options button{display:flex;width:100%;min-height:30px;align-items:center;justify-content:space-between;gap:8px;border:0;border-radius:4px;padding:5px 7px;color:#35414c;background:white;text-align:left;font-size:var(--font-control)}.font-options button:hover,.font-options button.active{background:#eef4ff}.font-options button.selected{color:#174a9e;font-weight:650}.font-options button span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.font-empty{padding:16px 8px;color:var(--muted);font-size:var(--font-aux);text-align:center}
  :global(.spin){animation:spin .8s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}
  @media(max-width:680px){.setting-row{grid-template-columns:32px 1fr}.segmented,.font-picker{grid-column:1/-1;width:100%}.segmented button{min-width:0;flex:1}}
  @media(prefers-reduced-motion:reduce){:global(.spin){animation:none}}
</style>
