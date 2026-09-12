<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Dices, Fingerprint, Users, Check, Eye, EyeOff } from "lucide-vue-next";
import { api, errorCode, notify } from "../api";
interface Player {
  id: string;
  name: string;
  alive: boolean;
  score: number;
  submitted: boolean;
  word?: string;
  spy?: boolean;
  guess?: number;
}
interface Game {
  id: string;
  kind: string;
  phase: string;
  language: string;
  version: number;
  round: number;
  turn: number;
  players: Player[];
  candidates: string[];
  tie: boolean;
  result: string;
  die: number;
  mine?: { word: string; guess: number; vote: string };
}
const props = defineProps<{
  roomId: string;
  myId: string;
  host: boolean;
  closed: boolean;
  visible?: boolean;
}>();
const { t, locale } = useI18n();
const game = ref<Game | null>(null),
  busy = ref(false),
  loaded = ref(false),
  failed = ref(false),
  reveal = ref(false),
  language = ref(locale.value.startsWith("zh") ? "zh" : "en");
const me = computed(() => game.value?.players.find((p) => p.id === props.myId));
const selectedGuess = ref(0);
const selectedVote = ref("");
watch(
  [
    () => game.value?.id,
    () => game.value?.phase,
    () => game.value?.round,
    () => game.value?.tie,
  ],
  () => {
    selectedGuess.value = 0;
    selectedVote.value = "";
    reveal.value = false;
  },
);
watch(
  () => props.visible,
  () => {
    reveal.value = false;
  },
);
function conceal() {
  reveal.value = false;
}
let timer: ReturnType<typeof setInterval>;
let disposed = false;
let reading = false;
async function refresh() {
  if (reading || busy.value) return;
  reading = true;
  try {
    const result = await api<Game | null>(`/rooms/${props.roomId}/game`);
    if (!disposed) {
      if (!game.value || (result && result.version >= game.value.version))
        game.value = result;
      loaded.value = true;
      failed.value = false;
    }
  } catch {
    failed.value = true;
  } finally {
    reading = false;
  }
}
async function act(action: string, extra: Record<string, unknown> = {}) {
  if (busy.value) return;
  busy.value = true;
  reveal.value = false;
  try {
    const g = game.value;
    game.value = await api<Game>(`/rooms/${props.roomId}/game`, "POST", {
      action,
      version: g?.version || 0,
      game_id: g?.id || "",
      round: g?.round || 0,
      phase: g?.phase || "",
      tie: g?.tie || false,
      ...extra,
    });
  } catch (e) {
    const code = errorCode(e);
    notify(t("error." + (code === "VERSION_CONFLICT" ? "GAME_PHASE" : code)));
  } finally {
    busy.value = false;
    await refresh();
  }
}
onMounted(() => {
  window.addEventListener("blur", conceal);
  document.addEventListener("visibilitychange", conceal);
  void refresh();
  timer = setInterval(refresh, 1500);
});
onBeforeUnmount(() => {
  window.removeEventListener("blur", conceal);
  document.removeEventListener("visibilitychange", conceal);
  disposed = true;
  clearInterval(timer);
});
const active = computed(() => game.value && game.value.phase !== "finished");
</script>
<template>
  <section class="game-center" :aria-label="t('activity.games')">
    <header class="activity-heading">
      <div>
        <h2>{{ t("activity.games") }}</h2>
        <p class="muted">{{ t("game.subtitle") }}</p>
      </div>
      <Users :size="28" />
    </header>
    <p v-if="failed" role="status">
      {{ t("app.offline") }}
      <button @click="refresh">{{ t("activity.retry") }}</button>
    </p>
    <p v-if="!loaded">{{ t("app.loading") }}</p>
    <template v-if="loaded && !active">
      <label v-if="host" class="inline-label"
        >{{ t("game.language")
        }}<select v-pretty-select v-model="language">
          <option value="zh">简体中文</option>
          <option value="en">English</option>
        </select></label
      >
      <div class="game-library">
        <article>
          <Fingerprint :size="36" />
          <div>
            <h3>{{ t("game.undercover") }}</h3>
            <p>{{ t("game.undercoverHelp") }}</p>
            <small>{{ t("game.undercoverRules") }}</small>
          </div>
          <button
            v-if="host"
            class="primary"
            :disabled="busy || closed"
            @click="act('create', { kind: 'undercover', language })"
          >
            {{ t("game.create") }}
          </button>
        </article>
        <article>
          <Dices :size="36" />
          <div>
            <h3>{{ t("game.dice") }}</h3>
            <p>{{ t("game.diceHelp") }}</p>
            <small>{{ t("game.diceRules") }}</small>
          </div>
          <button
            v-if="host"
            class="primary"
            :disabled="busy || closed"
            @click="act('create', { kind: 'dice', language })"
          >
            {{ t("game.create") }}
          </button>
        </article>
      </div>
      <p v-if="!host" class="muted">{{ t("game.waitHost") }}</p>
    </template>
    <section v-if="game" class="game-session">
      <div class="section-head">
        <h3>{{ t("game." + game.kind) }}</h3>
        <span class="tag">{{ t("game.phase." + game.phase) }}</span>
      </div>
      <p v-if="game.round" class="muted">
        {{ t("game.round", { n: game.round }) }}
      </p>
      <p v-if="closed" role="status">{{ t("game.closed") }}</p>
      <template v-if="game.phase === 'lobby'"
        ><p>{{ t("game.seats", { n: game.players.length }) }}</p>
        <div class="row wrap">
          <button v-if="!me" :disabled="busy || closed" @click="act('join')">
            {{ t("game.join") }}</button
          ><button v-else :disabled="busy || closed" @click="act('leave')">
            {{ t("game.leave") }}</button
          ><button
            v-if="host"
            class="primary"
            :disabled="
              busy ||
              closed ||
              game.players.length < (game.kind === 'dice' ? 2 : 4)
            "
            @click="act('start')"
          >
            {{ t("game.start") }}
          </button>
        </div></template
      >
      <div v-if="game.mine?.word && active" class="secret-word">
        <button :aria-expanded="reveal" @click="reveal = !reveal">
          <EyeOff v-if="reveal" :size="18" /><Eye v-else :size="18" />{{
            t(reveal ? "game.hideWord" : "game.showWord")
          }}</button
        ><strong v-if="reveal">{{ game.mine.word }}</strong
        ><small>{{ t("game.secretHelp") }}</small>
      </div>
      <div v-if="game.phase === 'describe'" class="turn-panel">
        <h3>{{ t("game.turn", { name: game.players[game.turn]?.name }) }}</h3>
        <p>{{ t("game.describeHelp") }}</p>
        <button
          v-if="host || game.players[game.turn]?.id === myId"
          :disabled="busy || closed"
          class="primary"
          @click="act('next')"
        >
          {{ t("game.described") }}
        </button>
      </div>
      <div v-if="game.phase === 'vote'">
        <h3>{{ t(game.tie ? "game.tie" : "game.vote") }}</h3>
        <p>{{ t("game.voteHelp") }}</p>
        <div v-if="me?.alive && !game.mine?.vote" class="vote-choice">
          <p class="muted small">{{ t("ui.choiceHelp") }}</p>
          <div class="row wrap">
            <button
              v-for="p in game.players.filter(
                (p) =>
                  p.alive &&
                  p.id !== myId &&
                  (!game!.candidates.length || game!.candidates.includes(p.id)),
              )"
              :key="p.id"
              :disabled="busy || closed"
              :class="{ chosen: selectedVote === p.id }"
              :aria-pressed="selectedVote === p.id"
              @click="selectedVote = p.id"
            >
              {{ p.name }}
            </button>
          </div>
          <button
            class="primary"
            :disabled="busy || closed || !selectedVote"
            @click="act('vote', { target: selectedVote })"
          >
            {{ t("ui.confirmVote") }}
          </button>
        </div>
        <p v-else-if="game.mine?.vote">{{ t("game.submitted") }}</p>
      </div>
      <div v-if="game.phase === 'guess'">
        <h3>{{ t("game.guess") }}</h3>
        <template v-if="me && !game.mine?.guess">
          <p class="muted small">{{ t("ui.choiceHelp") }}</p>
          <div class="dice-options">
            <button
              v-for="n in 6"
              :key="n"
              :disabled="busy || closed"
              :aria-label="t('game.point', { n })"
              :class="{ chosen: selectedGuess === n }"
              :aria-pressed="selectedGuess === n"
              @click="selectedGuess = n"
            >
              <span>{{ ["⚀", "⚁", "⚂", "⚃", "⚄", "⚅"][n - 1] }}</span
              ><small>{{ n }}</small>
            </button>
          </div>
          <button
            class="primary"
            :disabled="busy || closed || !selectedGuess"
            @click="act('guess', { guess: selectedGuess })"
          >
            {{ t("ui.confirmGuess") }}
          </button>
        </template>
        <p v-else-if="game.mine?.guess">
          {{ t("game.myGuess", { n: game.mine.guess }) }}
        </p>
        <p v-else>{{ t("game.spectating") }}</p>
      </div>
      <div v-if="game.phase === 'result'" class="dice-result" role="status">
        <span aria-hidden="true">{{
          ["⚀", "⚁", "⚂", "⚃", "⚄", "⚅"][game.die - 1]
        }}</span>
        <h3>{{ t("game.rolled", { n: game.die }) }}</h3>
        <button
          v-if="host"
          :disabled="busy || closed"
          class="primary"
          @click="act('round')"
        >
          {{ t("game.nextRound") }}
        </button>
      </div>
      <p v-if="game.phase === 'finished'" class="game-outcome" role="status">
        {{ t("game.result." + game.result) }}
      </p>
      <ul class="player-list">
        <li v-for="(p, i) in game.players" :key="p.id">
          <span class="seat-number">{{ i + 1 }}</span>
          <div>
            <strong
              >{{ p.name }}
              <small v-if="p.id === myId">{{ t("game.you") }}</small></strong
            ><small v-if="game.kind === 'undercover' && !p.alive">{{
              t("game.eliminated")
            }}</small
            ><small v-if="p.word"
              >{{ p.word }} ·
              {{ t(p.spy ? "game.spy" : "game.civilian") }}</small
            ><small v-if="game.phase === 'result'">{{
              t("game.myGuess", { n: p.guess || 0 })
            }}</small>
          </div>
          <span v-if="game.kind === 'dice'">{{
            t("game.score", { n: p.score })
          }}</span
          ><Check
            v-if="p.submitted && ['guess', 'vote'].includes(game.phase)"
            :size="19"
            :aria-label="t('game.submitted')"
          />
        </li>
      </ul>
      <details v-if="host && active" class="game-controls">
        <summary>{{ t("game.manage") }}</summary>
        <p>{{ t("game.cancelHelp") }}</p>
        <button :disabled="busy || closed" @click="act('cancel')">
          {{ t("game.cancel") }}
        </button>
      </details>
    </section>
  </section>
</template>
