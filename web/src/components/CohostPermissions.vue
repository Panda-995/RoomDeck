<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { api, errorCode } from "../api";
const props = defineProps<{
  room: string;
  member: string;
  permissions: string[];
  disabled?: boolean;
}>();
const selected = ref([...props.permissions]);
watch(
  () => props.permissions,
  (v) => {
    selected.value = [...v];
  },
);
const emit = defineEmits<{ changed: [] }>();
const { t } = useI18n();
const busy = ref(false),
  error = ref("");
async function toggle(scope: string, enabled: boolean) {
  busy.value = true;
  error.value = "";
  try {
    const result = await api<{ permissions: string[] }>(
      `/rooms/${props.room}/participants/${props.member}/permissions`,
      "PUT",
      { scope, enabled },
    );
    selected.value = result.permissions;
    emit("changed");
  } catch (e) {
    error.value = errorCode(e);
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <details class="cohost-permissions">
    <summary>
      {{ t("cohost.title") }} <span>{{ selected.length }} / 4</span>
    </summary>
    <p class="muted small">{{ t("cohost.help") }}</p>
    <div class="cohost-scopes">
      <label
        v-for="scope in ['content', 'screen', 'games', 'display']"
        :key="scope"
        ><input
          type="checkbox"
          :checked="selected.includes(scope)"
          :disabled="busy || disabled"
          @change="toggle(scope, ($event.target as HTMLInputElement).checked)"
        />{{ t("cohost." + scope) }}</label
      >
    </div>
    <p v-if="error" role="alert">{{ t("error." + error) }}</p>
  </details>
</template>
