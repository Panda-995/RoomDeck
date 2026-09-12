<script setup lang="ts">
import { nextTick, onMounted, onBeforeUnmount, ref, watch } from "vue";
import { enterEase, motionAllowed } from "../motion";
const props = defineProps<{ activeKey: string; label: string }>();
const nav = ref<HTMLElement>();
const highlight = ref<HTMLElement>();
let observer: ResizeObserver;
let textObserver: MutationObserver;
let animation: Animation | undefined;
let positioned = false;
function place(animate = false) {
  const root = nav.value,
    ink = highlight.value;
  const active = root?.querySelector<HTMLElement>(
    'button[aria-pressed="true"]',
  );
  if (!root || !ink || !active || !root.getClientRects().length) return;
  const previous = ink.getBoundingClientRect();
  const parent = root.getBoundingClientRect();
  const target = active.getBoundingClientRect();
  const x = target.left - parent.left + root.scrollLeft;
  const y = target.top - parent.top + root.scrollTop;
  animation?.cancel();
  Object.assign(ink.style, {
    width: `${target.width}px`,
    height: `${target.height}px`,
    transform: `translate(${x}px, ${y}px)`,
  });
  root.dataset.indicatorReady = "true";
  if (animate && positioned && motionAllowed()) {
    animation = ink.animate(
      [
        {
          transform: `translate(${previous.left - parent.left + root.scrollLeft}px, ${previous.top - parent.top + root.scrollTop}px) scale(${previous.width / target.width}, ${previous.height / target.height})`,
        },
        { transform: `translate(${x}px, ${y}px) scale(1)` },
      ],
      { duration: 240, easing: enterEase },
    );
  }
  positioned = true;
}
watch(
  () => props.activeKey,
  () => {
    void nextTick(() => place(true));
  },
);
onMounted(() => {
  place();
  observer = new ResizeObserver(() => place());
  observer.observe(nav.value!);
  textObserver = new MutationObserver(() => place());
  textObserver.observe(nav.value!, {
    childList: true,
    subtree: true,
    characterData: true,
  });
});
onBeforeUnmount(() => {
  observer?.disconnect();
  textObserver?.disconnect();
  animation?.cancel();
});
</script>
<template>
  <nav ref="nav" class="motion-nav" :aria-label="label">
    <span ref="highlight" class="nav-highlight" aria-hidden="true"></span>
    <slot />
  </nav>
</template>
