<script lang="ts">
  type Props = {
    label: string;
    onResize: (delta: number) => void;
    onReset?: () => void;
    onResizeEnd?: () => void;
    orientation?: "vertical" | "horizontal";
  };

  let {
    label, onResize, onReset = () => {}, onResizeEnd = () => {}, orientation = "vertical",
  }: Props = $props();
  let pointerId: number | null = $state(null);
  let lastPosition = $state(0);

  function pointerPosition(event: PointerEvent) {
    return orientation === "vertical" ? event.clientX : event.clientY;
  }

  function startResize(event: PointerEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    event.stopPropagation();
    pointerId = event.pointerId;
    lastPosition = pointerPosition(event);
  }

  function updateResize(event: PointerEvent) {
    if (pointerId !== event.pointerId) return;
    event.preventDefault();
    const nextPosition = pointerPosition(event);
    const delta = nextPosition - lastPosition;
    if (!delta) return;
    lastPosition = nextPosition;
    onResize(delta);
  }

  function stopResize(event: PointerEvent) {
    if (pointerId !== event.pointerId) return;
    pointerId = null;
    onResizeEnd();
  }

  function cancelResize() {
    if (pointerId === null) return;
    pointerId = null;
    onResizeEnd();
  }

  function handleKeydown(event: KeyboardEvent) {
    const decreaseKey = orientation === "vertical" ? "ArrowLeft" : "ArrowUp";
    const increaseKey = orientation === "vertical" ? "ArrowRight" : "ArrowDown";
    if (event.key === decreaseKey || event.key === increaseKey) {
      event.preventDefault();
      onResize(event.key === decreaseKey ? -16 : 16);
      onResizeEnd();
    } else if (event.key === "Home") {
      event.preventDefault();
      onReset();
    }
  }
</script>

<svelte:window
  onpointermove={updateResize}
  onpointerup={stopResize}
  onpointercancel={stopResize}
  onblur={cancelResize}
/>

<button
  type="button"
  class="resize-handle"
  class:horizontal={orientation === "horizontal"}
  class:active={pointerId !== null}
  aria-label={label}
  title={orientation === "vertical" ? "拖动调整宽度，双击恢复默认" : "拖动调整高度，双击恢复默认"}
  onpointerdown={startResize}
  ondblclick={onReset}
  onkeydown={handleKeydown}
></button>

<style>
  .resize-handle{position:relative;width:100%;height:100%;min-height:40px;border:0;padding:0;outline:0;background:transparent;cursor:col-resize;touch-action:none;user-select:none}
  .resize-handle:before{position:absolute;top:50%;left:50%;width:3px;height:54px;border-radius:999px;background:#c4cbd2;content:"";transform:translate(-50%,-50%);transition:width .14s,height .14s,background .14s,box-shadow .14s}
  .resize-handle:hover:before{width:4px;height:62px;background:#8da9d6}
  .resize-handle:focus:before,.resize-handle.active:before{width:4px;height:66px;background:var(--blue);box-shadow:0 0 0 3px rgba(37,99,235,.13)}
  .resize-handle.horizontal{min-width:40px;min-height:0;cursor:row-resize}.resize-handle.horizontal:before{width:54px;height:3px}.resize-handle.horizontal:hover:before{width:62px;height:4px}.resize-handle.horizontal:focus:before,.resize-handle.horizontal.active:before{width:66px;height:4px}
  @media(prefers-reduced-motion:reduce){.resize-handle:before{transition:none}}
</style>
