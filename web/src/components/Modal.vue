<script setup lang="ts">
import { onMounted, onUnmounted, useTemplateRef, useId } from "vue";
import { X } from "lucide-vue-next";
import { useI18n } from "vue-i18n";
import { notification } from "../api";
import { modalStack } from "../modalState";
import { enterEase, motionAllowed } from "../motion";
import { registerDialogLeave } from "../dialogMotion";
const props = defineProps<{ title: string; wide?: boolean; busy?: boolean }>();
const emit = defineEmits<{ close: [] }>();
const { t } = useI18n();
const titleId = useId();
const dialog = useTemplateRef<HTMLDialogElement>("dialog");
onMounted(() => {
  if (!dialog.value) return;
  const element = dialog.value;
  registerDialogLeave(element, (done) => leave(element, done));
  enter(element, () => {});
});
let opener: HTMLElement | null = null;
let activeAnimation: Animation | undefined;
let surfaceElement: HTMLDialogElement | undefined;
let released = false;
function close() {
  if (!props.busy) emit("close");
}
function enter(element: Element, done: () => void) {
  const surface = element as HTMLDialogElement;
  surfaceElement = surface;
  opener =
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
  modalStack.value.push(titleId);
  document.documentElement.classList.add("modal-open");
  surface.showModal();
  if (!motionAllowed()) {
    done();
    return;
  }
  const mobile = matchMedia("(max-width: 640px)").matches;
  activeAnimation = surface.animate(
    [
      {
        opacity: 0,
        transform: mobile ? "translateY(32px)" : "translateY(12px) scale(.975)",
      },
      { opacity: 1, transform: "none" },
    ],
    { duration: 260, easing: enterEase },
  );
  void activeAnimation.finished.catch(() => {}).finally(done);
}
function release() {
  if (released) return;
  released = true;
  const wasTop = modalStack.value.at(-1) === titleId;
  modalStack.value = modalStack.value.filter((id) => id !== titleId);
  if (!modalStack.value.length)
    document.documentElement.classList.remove("modal-open");
  if (wasTop && opener?.isConnected) opener.focus({ preventScroll: true });
}
function leave(element: Element, done: () => void) {
  const surface = element as HTMLDialogElement;
  const finish = () => {
    surface.close();
    release();
    done();
  };
  const current = getComputedStyle(surface);
  const start = { opacity: current.opacity, transform: current.transform };
  activeAnimation?.cancel();
  if (!motionAllowed()) {
    finish();
    return;
  }
  surface.dataset.leaving = "true";
  activeAnimation = surface.animate(
    [
      start,
      {
        opacity: 0,
        transform: matchMedia("(max-width: 640px)").matches
          ? "translateY(24px)"
          : "translateY(8px) scale(.985)",
      },
    ],
    { duration: 180, easing: enterEase },
  );
  void activeAnimation.finished.catch(() => {}).finally(finish);
}
onUnmounted(() => {
  if (!surfaceElement?.isConnected) release();
});
</script>
<template>
  <dialog
    ref="dialog"
    :class="{ wide }"
    :aria-labelledby="titleId"
    :aria-busy="busy || undefined"
    @cancel.self.prevent="close"
  >
    <header class="modal-header">
      <h2 :id="titleId">{{ title }}</h2>
      <button
        class="icon-button"
        :aria-label="t('app.close')"
        :disabled="busy"
        @click="close"
      >
        <X :size="20" />
      </button>
    </header>
    <section class="modal-body">
      <slot />
    </section>
    <div
      v-if="notification && modalStack.at(-1) === titleId"
      class="modal-notification"
      role="status"
    >
      {{ notification }}
    </div>
  </dialog>
</template>
