<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  ImagePlus,
  FileUp,
  StickyNote,
  Link,
  ChartNoAxesColumn,
} from "lucide-vue-next";
import Modal from "./Modal.vue";
import { api, errorCode, notify } from "../api";
import UploadTasks from "./UploadTasks.vue";
import type { Room } from "../types";
const props = defineProps<{ room: Room; host: boolean; identity: string }>();
const emit = defineEmits<{ close: []; shared: [] }>();
const { t } = useI18n();
const kind = ref(""),
  title = ref(""),
  body = ref(""),
  busy = ref(false),
  options = ref(""),
  multiple = ref(false),
  maxChoices = ref(2),
  hideResults = ref(false),
  minutes = ref(30);

const allowed = computed(() =>
  props.room.settings.modules.filter(
    (m) => m !== "display" && (props.host || m !== "poll"),
  ),
);
const icons = {
  photo: ImagePlus,
  file: FileUp,
  note: StickyNote,
  link: Link,
  poll: ChartNoAxesColumn,
};
kind.value = allowed.value[0] || "";
const draftKey = `roomdeck:draft:${props.room.id}:${props.identity}`;
try {
  const draft = JSON.parse(sessionStorage.getItem(draftKey) || "null");
  if (draft && allowed.value.includes(draft.kind)) {
    kind.value = draft.kind;
    title.value = String(draft.title || "").slice(0, 120);
    body.value = String(draft.body || "").slice(0, 2048);
    options.value = String(draft.options || "").slice(0, 2000);
    multiple.value = draft.multiple === true;
    hideResults.value = draft.hideResults === true;
    maxChoices.value = Math.max(1, Math.min(10, Number(draft.maxChoices) || 2));
    minutes.value = Math.max(1, Math.min(1440, Number(draft.minutes) || 30));
  }
} catch {
  /* Storage may be unavailable in private browsing. */
}
watch(
  [kind, title, body, options, multiple, hideResults, maxChoices, minutes],
  () => {
    try {
      sessionStorage.setItem(
        draftKey,
        JSON.stringify({
          kind: kind.value,
          title: title.value,
          body: body.value,
          options: options.value,
          multiple: multiple.value,
          hideResults: hideResults.value,
          maxChoices: maxChoices.value,
          minutes: minutes.value,
        }),
      );
    } catch {}
  },
);

async function post() {
  busy.value = true;
  try {
    await api("/rooms/" + props.room.id + "/contents", "POST", {
      kind: kind.value,
      title: title.value,
      body: body.value,
      ...(kind.value === "poll"
        ? {
            poll: {
              options: options.value
                .split("\n")
                .map((s) => s.trim())
                .filter(Boolean),
              multiple: multiple.value,
              max_choices: multiple.value ? maxChoices.value : 1,
              hide_results: hideResults.value,
              closed: false,
              closes_at: Math.floor(Date.now() / 1000) + minutes.value * 60,
            },
          }
        : {}),
    });
    try {
      sessionStorage.removeItem(draftKey);
    } catch {}
    emit("shared");
    emit("close");
    notify(t("room.uploaded"));
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
function select(m: string) {
  kind.value = m;
}
</script>
<template>
  <Modal :busy="busy" :title="t('room.share')" @close="emit('close')"
    ><div class="share-kinds">
      <button
        v-for="m in allowed"
        :key="m"
        :class="{ chosen: kind === m }"
        :aria-pressed="kind === m"
        :disabled="busy"
        @click="select(m)"
      >
        <component :is="icons[m as keyof typeof icons]" :size="23" /><span>{{
          t("module." + m)
        }}</span>
      </button>
    </div>
    <template v-if="kind === 'photo' || kind === 'file'"
      ><p class="muted small">{{ t("room.uploadHelp") }}</p>
      <div class="notice">
        {{
          t(
            props.room.settings.auto_display
              ? "room.uploadAutoPrivacy"
              : "room.uploadPrivacy",
          )
        }}
      </div>
      <UploadTasks :room="room.id" :identity="identity" :kind="kind" />
    </template>
    <form v-else-if="kind" class="form-stack" @submit.prevent="post">
      <label
        >{{ t(kind === "poll" ? "room.pollQuestion" : "room.titleOptional")
        }}<input
          v-model="title"
          :required="kind === 'poll'"
          maxlength="120" /></label
      ><label v-if="kind !== 'poll'"
        >{{ t(kind === "link" ? "room.url" : "room.body")
        }}<input
          v-if="kind === 'link'"
          v-model="body"
          type="url"
          placeholder="https://"
          required
          maxlength="2048"
        /><textarea
          v-else
          v-model="body"
          required
          maxlength="2000"
          rows="5"
        ></textarea></label
      ><template v-else
        ><label
          >{{ t("room.pollOptions")
          }}<textarea v-model="options" required rows="4"></textarea></label
        ><label class="check-label"
          ><input v-model="multiple" type="checkbox" />{{
            t("room.multiple")
          }}</label
        ><label v-if="multiple"
          >{{ t("room.maxChoices")
          }}<input
            v-model.number="maxChoices"
            type="number"
            min="1"
            max="8"
            required /></label
        ><label class="check-label"
          ><input v-model="hideResults" type="checkbox" />{{
            t("room.hideResults")
          }}</label
        ><label
          >{{ t("room.pollDuration")
          }}<input
            v-model.number="minutes"
            type="number"
            min="1"
            max="10080"
            required /></label
      ></template>
      <p class="muted small">
        {{ t(kind === "poll" ? "room.votePrivacy" : "room.plainText") }}
      </p>
      <button class="primary" :disabled="busy">
        {{ t(busy ? "app.working" : "room.publish") }}
      </button>
    </form></Modal
  >
</template>
