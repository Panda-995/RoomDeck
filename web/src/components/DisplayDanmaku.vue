<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from "vue";
import { useInteractions, seenMessages, type Message } from "../interactions";
const props = defineProps<{ room: string; stationary?: boolean }>();
const { store } = useInteractions(props.room, true);
const showing = ref<Message[]>([]);
const seen = seenMessages(props.room);
const until = new Map<string, number>();
let timer: ReturnType<typeof setInterval>;
function tick() {
  const now = Date.now();
  showing.value = showing.value.filter(
    (m) =>
      (until.get(m.id) || 0) > now && store.messages.some((v) => v.id === m.id),
  );
  for (const m of store.messages) {
    if (showing.value.length >= 2) break;
    if (m.state !== "approved" || seen.has(m.id) || m.expires_at * 1000 <= now)
      continue;
    seen.set(m.id, m.expires_at * 1000 + 60000);
    until.set(m.id, Math.min(now + 8000, m.expires_at * 1000));
    showing.value.push(m);
  }
  for (const [id, end] of seen)
    if (end < now) {
      seen.delete(id);
      until.delete(id);
    }
}
onMounted(() => {
  timer = setInterval(tick, 300);
});
onBeforeUnmount(() => clearInterval(timer));
</script>
<template>
  <div class="display-danmaku" :class="{ stationary }" aria-live="off">
    <div v-for="m in showing" :key="m.id" class="danmaku-line">
      <span>{{ m.name }} · {{ m.body }}</span>
    </div>
  </div>
</template>
