<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import type { Room } from "../types";
import { api, errorCode, notify } from "../api";
import Modal from "./Modal.vue";
const props = defineProps<{ room: Room }>();
const emit = defineEmits<{ close: []; changed: [] }>();
const { t } = useI18n();
const draft = ref(JSON.parse(JSON.stringify(props.room)) as Room),
  quota = ref(props.room.settings.quota / 1024 ** 3),
  busy = ref(false);
async function save() {
  busy.value = true;
  try {
    draft.value.settings.quota = Math.round(quota.value * 1024 ** 3);
    await api("/rooms/" + props.room.id, "PATCH", {
      name: draft.value.name,
      settings: draft.value.settings,
      retention: draft.value.retention,
      version: draft.value.version,
    });
    emit("changed");
    emit("close");
    notify(t("app.saved"));
  } catch (e) {
    notify(t("error." + errorCode(e)));
    emit("changed");
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <Modal :busy="busy" :title="t('settings.title')" @close="emit('close')"
    ><form class="form-stack" @submit.prevent="save">
      <label
        >{{ t("home.roomName")
        }}<input v-model="draft.name" maxlength="40" required /></label
      ><label
        >{{ t("home.retention")
        }}<select v-pretty-select v-model="draft.retention">
          <option v-for="n in [1, 7, 30]" :key="n" :value="n * 86400">
            {{ t("home.days", { n }) }}
          </option></select
        ><small>{{ t("settings.retentionHint") }}</small></label
      >
      <fieldset>
        <legend>{{ t("settings.permissions") }}</legend>
        <label class="switch-row"
          ><span>{{ t("settings.joinLocked") }}</span
          ><input
            v-model="draft.settings.join_locked"
            type="checkbox"
            role="switch" /></label
        ><label class="switch-row"
          ><span>{{ t("settings.uploadsPaused") }}</span
          ><input
            v-model="draft.settings.uploads_paused"
            type="checkbox"
            role="switch" /></label
        ><label class="switch-row"
          ><span>{{ t("settings.originals") }}</span
          ><input
            v-model="draft.settings.download_original"
            type="checkbox"
            role="switch" /></label
        ><small>{{ t("settings.originalHint") }}</small
        ><label class="switch-row"
          ><span>{{ t("settings.autoDisplay") }}</span
          ><input
            v-model="draft.settings.auto_display"
            type="checkbox"
            role="switch" /></label
        ><small>{{ t("settings.autoHint") }}</small>
      </fieldset>
      <div class="form-two">
        <label
          >{{ t("settings.quota")
          }}<input
            v-model.number="quota"
            type="number"
            min="0.02"
            max="100"
            step="0.01"
            required /></label
        ><label
          >{{ t("settings.maxMembers")
          }}<input
            v-model.number="draft.settings.max_members"
            type="number"
            min="1"
            max="200"
            required
        /></label>
      </div>
      <fieldset>
        <legend>{{ t("home.modules") }}</legend>
        <div class="module-checks">
          <label
            v-for="m in ['photo', 'file', 'note', 'link', 'poll', 'display']"
            :key="m"
            ><input
              v-model="draft.settings.modules"
              type="checkbox"
              :value="m"
            />{{ t("module." + m) }}</label
          >
        </div>
        <small>{{ t("settings.moduleHint") }}</small>
      </fieldset>
      <button
        class="primary"
        :disabled="busy || !draft.settings.modules.length"
      >
        {{ t(busy ? "app.working" : "app.save") }}
      </button>
    </form></Modal
  >
</template>
