<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue";
import { useI18n } from "vue-i18n";
import { api, errorCode } from "../api";
const props = defineProps<{
  room: string;
  myId?: string;
  host?: boolean;
  closed?: boolean;
}>();
const emit = defineEmits<{ offer: [id: string]; mode: [mode: string] }>();
interface Request {
  id: string;
  owner: string;
  name: string;
  state: string;
  expires_at: number;
}
const { t } = useI18n();
const mode = ref("free"),
  requests = ref<Request[]>([]),
  busy = ref(false),
  error = ref("");
const own = computed(() => requests.value.find((r) => r.owner === props.myId));
let timer: ReturnType<typeof setInterval> | undefined,
  disposed = false,
  fetching = false;
async function refresh() {
  if (fetching || disposed) return;
  fetching = true;
  try {
    const result = await api<{ mode: string; requests: Request[] }>(
      `/rooms/${props.room}/screen/queue`,
    );
    if (disposed) return;
    mode.value = result.mode;
    requests.value = result.requests;
    emit("mode", mode.value);
    emit("offer", own.value?.state === "offered" ? own.value.id : "");
  } catch (e) {
    if (!disposed) error.value = errorCode(e);
  } finally {
    fetching = false;
  }
}
async function action(action: string, id = "", value = "") {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    await api(`/rooms/${props.room}/screen/queue`, "POST", {
      action,
      id,
      mode: value,
    });
    await refresh();
  } catch (e) {
    error.value = errorCode(e);
  } finally {
    busy.value = false;
  }
}
onMounted(() => {
  void refresh();
  timer = setInterval(refresh, 2000);
});
onBeforeUnmount(() => {
  disposed = true;
  clearInterval(timer);
});
</script>
<template>
  <div class="screen-queue">
    <label v-if="host"
      >{{ t("queue.mode") }}
      <select v-pretty-select
        :value="mode"
        :disabled="busy || closed"
        @change="action('mode', '', ($event.target as HTMLSelectElement).value)"
      >
        <option value="free">{{ t("queue.free") }}</option>
        <option value="queue">{{ t("queue.queue") }}</option>
        <option value="approval">{{ t("queue.approval") }}</option>
      </select></label
    >
    <p v-if="error" role="alert">{{ t("error." + error) }}</p>
    <template v-if="mode !== 'free'">
      <p v-if="own" role="status">
        {{ t("queue." + own.state)
        }}<span v-if="own.state === 'offered'">
          · {{ t("queue.offerHelp") }}</span
        >
      </p>
      <p v-else class="muted">{{ t("queue.help") }}</p>
      <button v-if="!own" :disabled="busy || closed" @click="action('request')">
        {{ t("queue.request") }}
      </button>
      <button
        v-else-if="own.state !== 'presenting'"
        :disabled="busy"
        @click="action('cancel', own.id)"
      >
        {{ t("queue.withdraw") }}
      </button>
      <ul class="queue-list">
        <li v-for="r in requests" :key="r.id">
          <span>{{ r.name }} · {{ t("queue." + r.state) }}</span
          ><span v-if="host" class="row wrap"
            ><button
              v-if="r.state === 'pending'"
              :disabled="busy"
              @click="action('approve', r.id)"
            >
              {{ t("queue.approve") }}</button
            ><button
              v-if="r.state !== 'presenting'"
              :disabled="busy"
              @click="action('reject', r.id)"
            >
              {{ t("queue.remove") }}
            </button></span
          >
        </li>
      </ul>
      <button v-if="host" :disabled="busy || closed" @click="action('next')">
        {{ t("queue.next") }}
      </button>
    </template>
  </div>
</template>
