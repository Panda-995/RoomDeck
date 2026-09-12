<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import { api, errorCode, notify } from "../api";
import { useInteractions } from "../interactions";
import type { Room, Settings } from "../types";
const props = defineProps<{ room: Room; host: boolean }>();
const emit = defineEmits<{ settings: [value: Partial<Settings>] }>();
const { t } = useI18n();
const { store, refresh } = useInteractions(props.room.id);
const body = ref(""),
  busy = ref(false);
async function send() {
  if (busy.value) return;
  busy.value = true;
  try {
    const r = await api<{ state: string }>(
      `/rooms/${props.room.id}/danmaku`,
      "POST",
      { body: body.value },
    );
    body.value = "";
    notify(t("danmaku." + r.state));
    await refresh();
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
async function moderate(action: string, id = "") {
  if (busy.value) return;
  busy.value = true;
  try {
    await api(`/rooms/${props.room.id}/danmaku/moderate`, "POST", {
      action,
      id,
    });
    await refresh();
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <details class="interaction-panel">
    <summary>{{ t("danmaku.title") }}</summary>
    <div v-if="host" class="interaction-settings">
      <label
        >{{ t("danmaku.mode")
        }}<select v-pretty-select
          :value="room.settings.danmaku_mode || 'off'"
          @change="
            emit('settings', {
              danmaku_mode: ($event.target as HTMLSelectElement).value,
            })
          "
        >
          <option value="off">{{ t("danmaku.off") }}</option>
          <option value="direct">{{ t("danmaku.direct") }}</option>
          <option value="approval">{{ t("danmaku.approval") }}</option>
        </select></label
      ><label class="row"
        ><input
          type="checkbox"
          :checked="!room.settings.reactions_disabled"
          @change="
            emit('settings', {
              reactions_disabled: !($event.target as HTMLInputElement).checked,
            })
          "
        />{{ t("reaction.enabled") }}</label
      ><button :disabled="busy" @click="moderate('clear')">
        {{ t("danmaku.clear") }}
      </button>
    </div>
    <form
      v-if="room.settings.danmaku_mode && room.settings.danmaku_mode !== 'off'"
      @submit.prevent="send"
    >
      <label
        >{{ t("danmaku.write")
        }}<input v-model="body" required maxlength="60" /></label
      ><button class="primary" :disabled="busy || room.state !== 'active'">
        {{ t("danmaku.send") }}
      </button>
    </form>
    <p v-else class="muted">{{ t("danmaku.off") }}</p>
    <ul class="danmaku-list">
      <li v-for="m in store.messages" :key="m.id">
        <span
          ><strong>{{ m.name }}</strong> {{ m.body }}
          <small>{{ t("danmaku." + m.state) }}</small></span
        ><span v-if="host && m.state === 'pending'" class="row"
          ><button :disabled="busy" @click="moderate('approve', m.id)">
            {{ t("queue.approve") }}</button
          ><button :disabled="busy" @click="moderate('reject', m.id)">
            {{ t("danmaku.reject") }}
          </button></span
        >
      </li>
    </ul>
  </details>
</template>
