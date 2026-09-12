import type { Directive } from "vue";

export const enterEase = "cubic-bezier(0.22, 1, 0.36, 1)";
const preference = matchMedia("(prefers-reduced-motion: reduce)");
export function motionAllowed() {
  return (
    !preference.matches &&
    document.documentElement.dataset.input !== "keyboard" &&
    !document.hidden
  );
}

export function installMotionPreferences() {
  const stop = () =>
    document.getAnimations().forEach((animation) => animation.cancel());
  document.addEventListener(
    "keydown",
    (event) => {
      if (["Shift", "Control", "Alt", "Meta"].includes(event.key)) return;
      document.documentElement.dataset.input = "keyboard";
      stop();
    },
    { capture: true },
  );
  document.addEventListener(
    "pointerdown",
    () => {
      document.documentElement.dataset.input = "pointer";
    },
    { capture: true, passive: true },
  );
  preference.addEventListener("change", () => {
    if (preference.matches) stop();
  });
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) stop();
  });
}

const running = new WeakMap<HTMLElement, Animation>();
function reveal(element: HTMLElement, direction = 0) {
  running.get(element)?.cancel();
  if (!motionAllowed() || !element.getClientRects().length) return;
  const animation = element.animate(
    [
      {
        opacity: 0,
        transform: direction
          ? `translateX(${direction * 10}px)`
          : "translateY(8px)",
      },
      { opacity: 1, transform: "translate(0, 0)" },
    ],
    { duration: 240, easing: enterEase },
  );
  running.set(element, animation);
  void animation.finished
    .catch(() => {})
    .finally(() => {
      if (running.get(element) === animation) running.delete(element);
    });
}

// Semantic keys change for navigation, never for routine server snapshots.
type RevealState =
  | string
  | number
  | boolean
  | undefined
  | { key: string | boolean; direction: number };
function state(value: RevealState | null) {
  return value !== null && typeof value === "object"
    ? value
    : { key: value, direction: 0 };
}
export const motionReveal: Directive<HTMLElement, RevealState> = {
  mounted(element, binding) {
    const value = state(binding.value);
    if (value.key !== false) reveal(element, value.direction);
  },
  updated(element, binding) {
    const value = state(binding.value);
    if (value.key !== state(binding.oldValue).key && value.key !== false)
      reveal(element, value.direction);
  },
  beforeUnmount(element) {
    running.get(element)?.cancel();
  },
};
