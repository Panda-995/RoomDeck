<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { api, errorCode, notify } from "../api";
import { interactionStore } from "../interactions";
const props = defineProps<{
  room: string;
  content: string;
  disabled: boolean;
}>();
const { t } = useI18n();
const open = ref(false),
  busy = ref(false);
const emoji: Record<string, string> = {
  heart: "❤️",
  like: "👍",
  laugh: "😄",
  wow: "😮",
  clap: "👏",
  party: "🎉",
};
const reactions = computed(() =>
  interactionStore(props.room).reactions.filter(
    (r) => r.content === props.content,
  ),
);
async function choose(value: string) {
  if (busy.value || props.disabled) return;
  busy.value = true;
  try {
    const selected = reactions.value.find((r) => r.emoji === value)?.mine;
    await api(
      `/rooms/${props.room}/contents/${props.content}/reaction`,
      "PUT",
      { emoji: selected ? "" : value },
    );
    const result = await api<{
      reactions: ReturnType<typeof interactionStore>["reactions"];
    }>(`/rooms/${props.room}/interactions`);
    interactionStore(props.room).reactions = result.reactions;
    open.value = false;
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <div class="reaction-bar">
    <button
      v-for="r in reactions"
      :key="r.emoji"
      :aria-pressed="!!r.mine"
      :aria-label="t('reaction.' + r.emoji) + ': ' + r.count"
      :disabled="disabled || busy"
      @click="choose(r.emoji)"
    >
      {{ emoji[r.emoji] }} {{ r.count }}</button
    ><button
      :disabled="disabled || busy"
      :aria-expanded="open"
      @click="open = !open"
    >
      {{ t("reaction.add") }}
    </button>
    <div v-if="open" class="reaction-picker">
      <button
        v-for="(icon, key) in emoji"
        :key="key"
        :aria-label="t('reaction.' + key)"
        :disabled="busy"
        @click="choose(key)"
      >
        {{ icon }}
      </button>
    </div>
  </div>
</template>
