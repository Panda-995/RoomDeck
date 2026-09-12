<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Palette, Undo2, Trash2, Download, Eye, EyeOff } from "lucide-vue-next";
import { api, errorCode } from "../api";
type Point = { x: number; y: number };
type Stroke = {
  id: string;
  owner: string;
  color: string;
  width: number;
  points: Point[];
};
type Player = { id: string; name: string; score: number; guessed: boolean };
interface Board {
  version: number;
  epoch: number;
  phase: string;
  language: string;
  strokes: Stroke[];
  players: Player[];
  guesses: { name: string; text: string; correct: boolean }[];
  turn: number;
  deadline: number;
  word: string;
  artist: string;
  cancelled: boolean;
  server_time: number;
}
const props = withDefaults(
  defineProps<{
    roomId: string;
    myId?: string;
    host?: boolean;
    closed?: boolean;
    visible?: boolean;
    display?: boolean;
  }>(),
  { visible: true },
);
const { t, locale } = useI18n();
const board = ref<Board>();
const busy = ref(false),
  reading = ref(false),
  failed = ref(""),
  sending = ref(false),
  confirmation = ref(""),
  answer = ref(""),
  reveal = ref(false);
const color = ref("#263b33"),
  width = ref(7),
  language = ref(locale.value.startsWith("zh") ? "zh" : "en");
const colors = [
  "#263b33",
  "#30664d",
  "#cc554d",
  "#d09a35",
  "#4c7db5",
  "#9369aa",
  "#ffffff",
];
const svg = ref<SVGSVGElement>();
const current = ref<Point[]>([]);
const pending = ref<Stroke[]>([]);
const now = ref(Date.now());
let timer: ReturnType<typeof setInterval>,
  disposed = false,
  pointer: number | undefined,
  offset = 0,
  lastSync = 0;
const me = computed(() =>
  board.value?.players.find((p) => p.id === props.myId),
);
const artist = computed(() =>
  board.value?.players.find((p) => p.id === board.value?.artist),
);
const gameActive = computed(() =>
  ["lobby", "drawing", "result"].includes(board.value?.phase || ""),
);
const canDraw = computed(
  () =>
    !props.display &&
    !props.closed &&
    !failed.value &&
    pending.value.length < 40 &&
    now.value - lastSync < 5000 &&
    (board.value?.phase === "free" ||
      (board.value?.phase === "drawing" && board.value.artist === props.myId)),
);
const seconds = computed(() =>
  Math.max(
    0,
    Math.ceil(
      ((board.value?.deadline || 0) * 1000 - now.value - offset) / 1000,
    ),
  ),
);
const strokes = computed(() => [
  ...(board.value?.strokes || []),
  ...pending.value.filter(
    (s) => !board.value?.strokes.some((v) => v.id === s.id),
  ),
]);
const myLast = computed(() =>
  [...(board.value?.strokes || [])]
    .reverse()
    .find((s) => s.owner === props.myId),
);
function points(v: Point[]) {
  return v.length === 1
    ? `${v[0]!.x},${v[0]!.y} ${v[0]!.x + 0.01},${v[0]!.y}`
    : v.map((p) => `${p.x},${p.y}`).join(" ");
}
async function refresh(force = false) {
  if (disposed || reading.value || !props.visible || document.hidden) return;
  reading.value = true;
  try {
    const result = await api<Board & { unchanged?: boolean }>(
      `/rooms/${props.roomId}/board?${props.display ? "display=1&" : ""}since=${force ? -1 : (board.value?.version ?? -1)}`,
    );
    if (disposed) return;
    lastSync = Date.now();
    offset = result.server_time * 1000 - Date.now();
    if (
      !result.unchanged &&
      (!board.value || result.version >= board.value.version)
    ) {
      if (board.value && result.epoch !== board.value.epoch) {
        current.value = [];
        pointer = undefined;
        pending.value = [];
        reveal.value = false;
        answer.value = "";
      }
      board.value = result;
    }
    if (
      !pending.value.length &&
      ["NETWORK_ERROR", "NETWORK_TIMEOUT"].includes(failed.value)
    )
      failed.value = "";
  } catch (e) {
    if (!disposed) {
      failed.value = errorCode(e);
      reveal.value = false;
    }
  } finally {
    reading.value = false;
  }
}
async function act(action: string, extra: Record<string, unknown> = {}) {
  if (!board.value || busy.value) return;
  busy.value = true;
  failed.value = "";
  confirmation.value = "";
  try {
    await api(`/rooms/${props.roomId}/board`, "POST", {
      action,
      version: board.value.version,
      epoch: board.value.epoch,
      language: language.value,
      ...extra,
    });
    if (action === "guess") answer.value = "";
    await refresh(true);
  } catch (e) {
    failed.value = errorCode(e);
    await refresh(true);
  } finally {
    busy.value = false;
  }
}
function point(e: PointerEvent): Point {
  const r = svg.value!.getBoundingClientRect();
  return {
    x: Math.round(
      Math.max(0, Math.min(1000, ((e.clientX - r.left) / r.width) * 1000)),
    ),
    y: Math.round(
      Math.max(0, Math.min(625, ((e.clientY - r.top) / r.height) * 625)),
    ),
  };
}
function start(e: PointerEvent) {
  if (!canDraw.value || pointer !== undefined || e.button !== 0) return;
  e.preventDefault();
  pointer = e.pointerId;
  svg.value?.setPointerCapture(pointer);
  current.value = [point(e)];
}
function move(e: PointerEvent) {
  if (pointer !== e.pointerId) return;
  if (!canDraw.value) {
    end(e);
    return;
  }
  current.value.push(point(e));
  if (current.value.length >= 32) {
    const tail = current.value.at(-1)!;
    commit();
    current.value = [tail];
  }
}
function end(e: PointerEvent) {
  if (pointer !== e.pointerId) return;
  commit();
  pointer = undefined;
  try {
    svg.value?.releasePointerCapture(e.pointerId);
  } catch {}
}
function commit() {
  if (!current.value.length) return;
  pending.value.push({
    id: `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`,
    owner: props.myId || "",
    color: color.value,
    width: width.value,
    points: [...current.value],
  });
  current.value = [];
  void flush();
}
async function flush() {
  if (sending.value || !board.value || disposed) return;
  sending.value = true;
  try {
    while (pending.value.length && !disposed) {
      const stroke = pending.value[0]!;
      const epoch = board.value!.epoch;
      await api(`/rooms/${props.roomId}/board`, "POST", {
        action: "stroke",
        epoch,
        stroke,
      });
      if (
        board.value!.epoch === epoch &&
        !board.value!.strokes.some((s) => s.id === stroke.id)
      )
        board.value!.strokes.push(stroke);
      pending.value = pending.value.filter((s) => s.id !== stroke.id);
    }
    failed.value = "";
    await refresh(true);
  } catch (e) {
    failed.value = errorCode(e);
    if (
      ["BOARD_CHANGED", "FORBIDDEN", "ROOM_CLOSED", "BOARD_FULL"].includes(
        failed.value,
      )
    ) {
      pending.value = [];
      await refresh(true);
    }
  } finally {
    sending.value = false;
  }
}
async function download() {
  if (!svg.value) return;
  const source = svg.value.cloneNode(true) as SVGSVGElement;
  source.setAttribute("xmlns", "http://www.w3.org/2000/svg");
  source.setAttribute("width", "1600");
  source.setAttribute("height", "1000");
  const url = URL.createObjectURL(
    new Blob([new XMLSerializer().serializeToString(source)], {
      type: "image/svg+xml",
    }),
  );
  try {
    const img = new Image();
    img.src = url;
    await img.decode();
    const canvas = document.createElement("canvas");
    canvas.width = 1600;
    canvas.height = 1000;
    canvas.getContext("2d")!.drawImage(img, 0, 0);
    canvas.toBlob((blob) => {
      if (!blob) return;
      const link = document.createElement("a");
      const output = URL.createObjectURL(blob);
      link.href = output;
      link.download = "RoomDeck-board.png";
      link.click();
      setTimeout(() => URL.revokeObjectURL(output), 1000);
    }, "image/png");
  } catch {
    failed.value = "NETWORK_ERROR";
  } finally {
    URL.revokeObjectURL(url);
  }
}
function conceal() {
  reveal.value = false;
  current.value = [];
  pointer = undefined;
}
function wake() {
  if (!document.hidden) {
    void refresh(true);
    void flush();
  } else conceal();
}
watch(
  () => props.visible,
  (visible) => {
    conceal();
    if (visible) void refresh(true);
  },
);
watch(
  () => props.closed,
  () => conceal(),
);
onMounted(() => {
  void refresh(true);
  timer = setInterval(() => {
    now.value = Date.now();
    void refresh();
  }, 800);
  window.addEventListener("online", wake);
  window.addEventListener("blur", conceal);
  document.addEventListener("visibilitychange", wake);
});
onBeforeUnmount(() => {
  disposed = true;
  clearInterval(timer);
  window.removeEventListener("online", wake);
  window.removeEventListener("blur", conceal);
  document.removeEventListener("visibilitychange", wake);
});
</script>
<template>
  <section
    class="board-panel"
    :class="{ 'board-display': display }"
    :aria-label="t('board.title')"
  >
    <header class="board-heading">
      <div>
        <span class="eyebrow"
          ><Palette :size="16" /> {{ t("board.title") }}</span
        >
        <h2>
          {{
            gameActive || board?.phase === "finished"
              ? t("board.guessTitle")
              : t("board.freeTitle")
          }}
        </h2>
        <p v-if="!display" class="muted">{{ t("board.help") }}</p>
      </div>
      <button
        v-if="!display && board"
        class="icon-button"
        :aria-label="t('board.download')"
        @click="download"
      >
        <Download :size="20" />
      </button>
    </header>
    <p v-if="failed" role="alert">
      {{ t("error." + failed) }}
      <button
        v-if="!display"
        @click="
          refresh(true);
          flush();
        "
      >
        {{ t("activity.retry") }}
      </button>
    </p>
    <p v-if="!board">{{ t("app.loading") }}</p>
    <template v-if="board">
      <div v-if="host && !display" class="board-management row wrap">
        <template v-if="!gameActive"
          ><label
            >{{ t("game.language")
            }}<select v-pretty-select v-model="language">
              <option value="zh">简体中文</option>
              <option value="en">English</option>
            </select></label
          ><button
            :disabled="busy || closed || sending"
            @click="confirmation = 'create'"
          >
            {{ t("board.create") }}</button
          ><button
            v-if="board.phase !== 'free'"
            :disabled="busy || closed"
            @click="confirmation = 'free'"
          >
            {{ t("board.backFree") }}
          </button></template
        >
        <button
          v-if="board.phase === 'lobby'"
          class="primary"
          :disabled="busy || closed || board.players.length < 2"
          @click="act('start')"
        >
          {{ t("board.start") }}
        </button>
        <button
          v-if="board.phase === 'result'"
          class="primary"
          :disabled="busy || closed"
          @click="act('next')"
        >
          {{
            t(
              board.turn + 1 >= board.players.length
                ? "board.results"
                : "board.next",
            )
          }}
        </button>
        <button
          v-if="gameActive"
          :disabled="busy || closed"
          @click="confirmation = 'finish'"
        >
          {{ t("board.finish") }}
        </button>
      </div>
      <div
        v-if="confirmation"
        class="board-confirm"
        role="group"
        :aria-label="t('board.confirmTitle')"
      >
        <p>{{ t("board.confirmHelp") }}</p>
        <button class="primary" :disabled="busy" @click="act(confirmation)">
          {{ t("board.confirm") }}</button
        ><button @click="confirmation = ''">{{ t("app.cancel") }}</button>
      </div>
      <div v-if="board.phase !== 'free'" class="board-round">
        <span>{{ t("board." + board.phase) }}</span
        ><strong v-if="board.phase === 'drawing'"
          >{{ t("board.artist", { name: artist?.name }) }} ·
          {{ seconds }}s</strong
        >
        <span v-if="board.phase === 'result'"
          >{{ t("board.answer") }} <strong>{{ board.word }}</strong></span
        >
        <template
          v-if="!display && board.phase === 'drawing' && board.artist === myId"
          ><button @click="reveal = !reveal">
            <EyeOff v-if="reveal" :size="16" /><Eye v-else :size="16" />{{
              t(reveal ? "board.hide" : "board.reveal")
            }}</button
          ><strong v-if="reveal" class="board-secret">{{
            board.word
          }}</strong></template
        >
        <button
          v-if="!display && board.phase === 'lobby'"
          :disabled="busy || closed"
          @click="act(me ? 'leave' : 'join')"
        >
          {{ t(me ? "board.leave" : "board.join") }}
        </button>
      </div>
      <div
        v-if="
          !display &&
          (board.phase === 'free' ||
            (board.phase === 'drawing' && board.artist === myId))
        "
        class="board-tools"
      >
        <div class="board-colors" role="group" :aria-label="t('board.colors')">
          <button
            v-for="(c, i) in colors"
            :key="c"
            :aria-label="t('board.color' + i)"
            :aria-pressed="color === c"
            :disabled="!canDraw"
            @click="color = c"
          >
            <span :style="{ background: c }" />
          </button>
        </div>
        <label
          >{{ t("board.width")
          }}<select v-pretty-select v-model.number="width" :disabled="!canDraw">
            <option v-for="n in [3, 7, 14]" :key="n" :value="n">{{ n }}</option>
          </select></label
        >
        <button
          :disabled="!canDraw || sending || busy || !myLast"
          :aria-label="t('board.undo')"
          @click="act('undo', { id: myLast?.id })"
        >
          <Undo2 :size="18" />
        </button>
        <button
          v-if="host"
          :disabled="closed || sending || busy || !board.strokes.length"
          :aria-label="t('board.clear')"
          @click="confirmation = 'clear'"
        >
          <Trash2 :size="18" />
        </button>
      </div>
      <div class="board-canvas-wrap">
        <svg
          ref="svg"
          viewBox="0 0 1000 625"
          class="board-canvas"
          :class="{ 'can-draw': canDraw }"
          role="img"
          :aria-label="t('board.canvas')"
          @pointerdown="start"
          @pointermove="move"
          @pointerup="end"
          @pointercancel="end"
          @lostpointercapture="end"
        >
          <rect width="1000" height="625" fill="#ffffff" />
          <polyline
            v-for="s in strokes"
            :key="s.id"
            :points="points(s.points)"
            fill="none"
            :stroke="s.color"
            :stroke-width="s.width"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
          <polyline
            v-if="current.length"
            :points="points(current)"
            fill="none"
            :stroke="color"
            :stroke-width="width"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </div>
      <div v-if="!display" class="board-caption">
        <span>{{
          t(sending || pending.length ? "board.saving" : "board.saved")
        }}</span
        ><span>{{ t("board.capacity", { n: board.strokes.length }) }}</span>
      </div>
      <form
        v-if="
          !display &&
          board.phase === 'drawing' &&
          me &&
          board.artist !== myId &&
          !me.guessed
        "
        class="board-guess"
        @submit.prevent="act('guess', { text: answer })"
      >
        <label class="grow"
          >{{ t("board.yourGuess")
          }}<input
            v-model="answer"
            maxlength="60"
            autocomplete="off"
            :disabled="closed || busy"
            required /></label
        ><button class="primary" :disabled="closed || busy || !answer.trim()">
          {{ t("board.submit") }}
        </button>
      </form>
      <p
        v-if="!display && board.phase === 'drawing' && me?.guessed"
        role="status"
      >
        {{ t("board.correct") }}
      </p>
      <div
        v-if="board.players.length"
        class="board-scores"
        :aria-label="t('board.scores')"
      >
        <span
          v-for="player in [...board.players].sort((a, b) => b.score - a.score)"
          :key="player.id"
          ><strong>{{ player.name }}</strong> {{ player.score }}
          <small v-if="player.guessed">✓</small></span
        >
      </div>
      <ul
        v-if="!display && board.guesses.length"
        class="board-guesses"
        aria-live="polite"
      >
        <li v-for="(g, i) in board.guesses" :key="i">
          <strong>{{ g.name }}</strong>
          {{ g.correct ? t("board.correct") : g.text }}
        </li>
      </ul>
      <p v-if="board.cancelled" class="muted">{{ t("board.cancelled") }}</p>
    </template>
  </section>
</template>
