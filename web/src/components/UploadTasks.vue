<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  addUpload,
  roomUploads,
  restoreUploads,
  pauseUpload,
  resumeUpload,
  cancelUpload,
  type UploadTask,
} from "../uploadTasks";
import { errorCode, notify } from "../api";
const props = defineProps<{ room: string; identity: string; kind: string }>();
const { t } = useI18n();
const tasks = computed(() => roomUploads(props.room, props.identity));
const input = ref<HTMLInputElement>(),
  reselect = ref<UploadTask>();
const dragging = ref(false);
onMounted(async () => {
  try {
    await restoreUploads(props.room, props.identity);
  } catch (e) {
    notify(t("error." + errorCode(e)));
  }
});
function add(files: File[]) {
  if (files.length > 20) {
    notify(t("room.uploadHelp"));
    return;
  }
  for (const file of files) {
    if (
      file.size <= 0 ||
      file.size > (props.kind === "photo" ? 20 : 200) * 1024 * 1024
    ) {
      notify(t("error.FILE_TOO_LARGE"));
      continue;
    }
    addUpload(props.room, props.identity, file, props.kind);
  }
}
function choose(event: Event) {
  const el = event.target as HTMLInputElement;
  const files = Array.from(el.files || []);
  el.value = "";
  if (reselect.value) {
    if (files[0]) resumeUpload(reselect.value, files[0]);
    reselect.value = undefined;
  } else add(files);
}
async function remove(task: UploadTask) {
  try {
    await cancelUpload(task);
  } catch (e) {
    notify(t("error." + errorCode(e)));
  }
}
</script>
<template>
  <input
    ref="input"
    type="file"
    hidden
    :multiple="!reselect"
    :accept="kind === 'photo' ? 'image/jpeg,image/png,image/webp' : undefined"
    @change="choose"
  />
  <button
    class="upload-drop full"
    :class="{ dragging }"
    @click="
      reselect = undefined;
      input?.click();
    "
    @dragover.prevent="dragging = true"
    @dragleave.prevent="dragging = false"
    @drop.prevent="
      dragging = false;
      add(Array.from($event.dataTransfer?.files || []));
    "
  >
    {{ t("room.chooseFiles") }}<small>{{ t("ui.dropFiles") }}</small>
  </button>
  <p class="muted">{{ t("uploads.help") }}</p>
  <ul class="upload-list" v-if="tasks.length">
    <li v-for="task in tasks" :key="task.key">
      <div>
        <strong>{{ task.filename }}</strong
        ><small
          >{{ t("uploads." + task.state) }}
          <span v-if="task.state === 'uploading'"
            >{{ task.percent }}%</span
          ></small
        ><small v-if="task.error && task.state === 'failed'" class="error">{{
          t("error." + task.error)
        }}</small>
        <progress
          v-if="task.state === 'uploading'"
          :value="task.percent"
          max="100"
          :aria-label="task.filename"
        />
      </div>
      <button
        v-if="
          ['uploading', 'preparing', 'queued', 'waiting'].includes(task.state)
        "
        @click="pauseUpload(task)"
      >
        {{ t("room.pause") }}
      </button>
      <button
        v-if="['paused', 'failed', 'waiting'].includes(task.state) && task.file"
        :disabled="task.running"
        @click="resumeUpload(task)"
      >
        {{ t("app.retry") }}
      </button>
      <button
        v-if="task.state === 'reselect' || task.state === 'failed'"
        :disabled="task.running"
        @click="
          reselect = task;
          input?.click();
        "
      >
        {{ t("uploads.reselect") }}
      </button>
      <button
        v-if="task.state !== 'ready'"
        :disabled="task.running"
        @click="remove(task)"
      >
        {{ t("app.cancel") }}
      </button>
    </li>
  </ul>
</template>
