<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import QRCode from "qrcode";
import { Copy, RefreshCw, Monitor } from "lucide-vue-next";
import Modal from "./Modal.vue";
import { api, errorCode, notify } from "../api";
import type { Snapshot, Status } from "../types";
const props = defineProps<{ snapshot: Snapshot }>();
const emit = defineEmits<{ close: []; changed: [] }>();
const { t } = useI18n();
const qr = ref(""),
  link = ref(""),
  displayLink = ref(""),
  busy = ref(false),
  base = ref("");
onMounted(async () => {
  try {
    const s = await api<Status>("/status");
    base.value = s.base_url || location.origin;
    await makeQR();
  } catch (e) {
    notify(t("error." + errorCode(e)));
  }
});
async function makeQR() {
  link.value = base.value + "/join#invite=" + props.snapshot.invite;
  qr.value = await QRCode.toDataURL(link.value, {
    width: 300,
    margin: 4,
    errorCorrectionLevel: "M",
  });
}
async function copy(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    notify(t("app.copied"));
  } catch {
    notify(t("app.manualCopy"));
  }
}
async function rotate() {
  busy.value = true;
  try {
    await api("/rooms/" + props.snapshot.room.id + "/invite", "POST");
    emit("changed");
    emit("close");
    notify(t("app.saved"));
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
async function pair() {
  busy.value = true;
  try {
    const p = await api<{ token: string }>(
      "/rooms/" + props.snapshot.room.id + "/display-session",
      "POST",
    );
    displayLink.value = base.value + "/display#pair=" + p.token;
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <Modal :busy="busy" :title="t('room.invite')" @close="emit('close')"
    ><div class="invite-hero">
      <img v-if="qr" :src="qr" :alt="t('room.scan')" width="250" height="250" />
      <h2 class="room-code">
        {{ snapshot.room.code.slice(0, 3) }} {{ snapshot.room.code.slice(3) }}
      </h2>
      <p>{{ snapshot.room.name }}</p>
    </div>
    <label class="form-label"
      >{{ t("room.copyInvite")
      }}<input
        :value="link"
        readonly
        @focus="(e) => (e.target as HTMLInputElement).select()" /></label
    ><button class="primary full" :disabled="!link" @click="copy(link)">
      <Copy :size="17" />{{ t("room.copyInvite") }}
    </button>
    <p class="notice small">{{ t("room.addressHint") }}</p>
    <details>
      <summary>{{ t("room.rotate") }}</summary>
      <p class="muted small">{{ t("room.rotateHelp") }}</p>
      <button :disabled="busy" @click="rotate">
        <RefreshCw :size="16" />{{ t("room.rotate") }}
      </button>
    </details>
    <div
      v-if="snapshot.room.settings.modules.includes('display')"
      class="pair-section"
    >
      <button :disabled="busy" @click="pair">
        <Monitor :size="17" />{{ t("room.displayLink") }}</button
      ><template v-if="displayLink"
        ><p class="muted small">{{ t("room.displayPairHint") }}</p>
        <input
          :aria-label="t('room.displayLink')"
          :value="displayLink"
          readonly
          @focus="(e) => (e.target as HTMLInputElement).select()"
        /><button @click="copy(displayLink)">
          {{ t("app.copy") }}
        </button></template
      >
    </div></Modal
  >
</template>
