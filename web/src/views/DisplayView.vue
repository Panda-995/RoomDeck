<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { Maximize, WifiOff, Image, PanelsTopLeft } from "lucide-vue-next";
import QRCode from "qrcode";
import { api, assetURL, errorCode, notify } from "../api";
import { useRoomSync } from "../roomSync";
import type { Status } from "../types";
import BoardPanel from "../components/BoardPanel.vue";
import DisplayDanmaku from "../components/DisplayDanmaku.vue";
const screenActive = ref(false);
const ScreenShare = defineAsyncComponent(
  () => import("../components/ScreenShare.vue"),
);
const route = useRoute(),
  router = useRouter();
const { t } = useI18n();
const routeID = String(route.params.id || "");
const {
  snapshot,
  connected,
  state: connectionState,
  error,
  lastSync,
  start,
} = useRoomSync(routeID, true);
const localError = ref(""),
  qr = ref(""),
  linkQR = ref(""),
  activePhotoID = ref(""),
  now = ref(Date.now());
let interval: ReturnType<typeof setInterval> | undefined;
let advanced = Date.now();
const stale = computed(
  () => lastSync.value > 0 && now.value - lastSync.value > 60000,
);
const room = computed(() => snapshot.value?.room);
const photos = computed(
  () => snapshot.value?.contents.filter((c) => c.kind === "photo") || [],
);
const current = computed(
  () =>
    photos.value.find((photo) => photo.id === activePhotoID.value) ||
    photos.value[0],
);
const index = computed(() =>
  Math.max(
    0,
    photos.value.findIndex((photo) => photo.id === current.value?.id),
  ),
);
watch(
  () => photos.value.map((photo) => photo.id).join(","),
  () => {
    if (!photos.value.some((photo) => photo.id === activePhotoID.value)) {
      activePhotoID.value = photos.value[0]?.id || "";
      advanced = Date.now();
    }
  },
);
const pinned = computed(
  () =>
    snapshot.value?.contents.filter(
      (c) => c.kind === "note" || c.kind === "link",
    ) || [],
);
const polls = computed(
  () => snapshot.value?.contents.filter((c) => c.kind === "poll") || [],
);
const selected = computed(() => {
  const list =
    room.value?.settings.display_mode === "poll" ? polls.value : pinned.value;
  return (
    list.find((c) => c.id === room.value?.settings.display_target) ||
    (room.value?.settings.display_mode === "poll" ? undefined : list[0])
  );
});
watch(
  () => selected.value?.body,
  async (value) => {
    linkQR.value =
      selected.value?.kind === "link" && value
        ? await QRCode.toDataURL(value, { width: 320, margin: 4 })
        : "";
  },
);
watch(
  () => room.value?.code,
  async (code) => {
    if (!code) return;
    const status = await api<Status>("/status");
    qr.value = await QRCode.toDataURL(
      (status.base_url || location.origin) + "/join?code=" + code,
      { width: 350, margin: 4 },
    );
  },
);
onMounted(async () => {
  try {
    const token = new URLSearchParams(location.hash.slice(1)).get("pair");
    if (token) {
      history.replaceState(null, "", location.pathname);
      const res = await api<{ room_id: string }>("/display-exchange", "POST", {
        token,
      });
      await router.replace("/display/" + res.room_id);
      return;
    }
    if (!routeID) {
      localError.value = "PAIR_INVALID";
      return;
    }
    await start();
    interval = setInterval(() => {
      now.value = Date.now();
      if (
        !room.value?.settings.display_paused &&
        Date.now() - advanced >= (room.value?.settings.interval || 10) * 1000
      ) {
        activePhotoID.value =
          photos.value[(index.value + 1) % Math.max(1, photos.value.length)]
            ?.id || "";
        advanced = Date.now();
      }
    }, 1000);
  } catch (e) {
    localError.value = errorCode(e);
  }
});
onBeforeUnmount(() => clearInterval(interval));
async function fullscreen() {
  try {
    await document.documentElement.requestFullscreen();
  } catch {
    notify(t("room.unsupportedFullscreen"));
  }
}
</script>
<template>
  <main class="display-view">
    <ScreenShare
      v-if="
        room?.state === 'active' &&
        room.settings.display_source !== 'content' &&
        room.settings.display_mode !== 'blank' &&
        !stale
      "
      :room-id="room.id"
      @active="screenActive = $event"
      display
    />
    <div v-if="localError || (error && !snapshot)" class="display-empty">
      <WifiOff :size="48" />
      <h1>{{ t("error." + (localError || error)) }}</h1>
    </div>
    <div v-else-if="!room" class="display-empty">
      <p>{{ t("app.loading") }}</p>
    </div>
    <div v-else-if="stale" class="display-empty">
      <WifiOff :size="48" />
      <h1>{{ t("room.displayOffline") }}</h1>
    </div>
    <div
      v-else-if="
        room.state === 'active' && room.settings.display_mode === 'blank'
      "
      class="blank-screen"
    ></div>
    <div v-else-if="room.state !== 'active'" class="display-empty">
      <h1>{{ room.name }}</h1>
      <p>{{ t("room.displayEnd") }}</p>
    </div>
    <template v-else
      ><div class="display-topline">
        <span class="display-live">{{
          connected ? t("app.connected") : t("connection." + connectionState)
        }}</span
        ><span>{{ t("app.online", { n: snapshot?.online || 0 }) }}</span>
      </div>
      <div class="display-content">
        <section class="display-canvas">
          <BoardPanel
            v-if="room.settings.display_mode === 'board'"
            :room-id="room.id"
            display
          />
          <template
            v-else-if="room.settings.display_mode === 'photos' && current"
            ><img
              class="display-photo"
              :src="assetURL(room.id, current.id, 'preview', true)"
              :alt="current.filename"
            />
            <div class="display-caption">
              <span>{{ current.author_name }}</span
              ><span
                >{{ (index % photos.length) + 1 }} / {{ photos.length }}</span
              >
            </div></template
          >
          <div
            v-else-if="
              room.state === 'active' &&
              room.settings.display_mode === 'pinned' &&
              selected
            "
            class="display-note"
          >
            <span>{{ t("room.pinned") }}</span>
            <h1>{{ selected.title }}</h1>
            <p>{{ selected.body }}</p>
            <img
              v-if="linkQR"
              :src="linkQR"
              :alt="t('room.postLink')"
              width="220"
              height="220"
            />
          </div>
          <div
            v-else-if="
              room.state === 'active' &&
              room.settings.display_mode === 'poll' &&
              selected?.poll
            "
            class="display-poll"
          >
            <span>{{ t("module.poll") }}</span>
            <h1>{{ selected.title }}</h1>
            <div class="display-poll-options">
              <div
                v-for="(option, i) in selected.poll.options"
                :key="i"
                class="display-poll-option"
              >
                <div class="row between">
                  <span>{{ option }}</span
                  ><strong v-if="selected.counts">{{
                    selected.counts[i]
                  }}</strong>
                </div>
                <div v-if="selected.counts" class="poll-bar">
                  <span
                    :style="{
                      width:
                        (selected.voters
                          ? (selected.counts[i]! / selected.voters) * 100
                          : 0) + '%',
                    }"
                  ></span>
                </div>
              </div>
            </div>
            <p v-if="selected.counts">
              {{ t("room.votes", { n: selected.voters }) }}
              <span v-if="selected.poll.multiple">
                · {{ t("displayControl.multiple") }}</span
              >
            </p>
            <p v-else>{{ t("room.resultsHidden") }}</p>
          </div>
          <div v-else class="display-empty">
            <span
              v-if="room.settings.display_mode === 'welcome'"
              class="display-welcome-mark"
              aria-hidden="true"
              ><PanelsTopLeft :size="36" stroke-width="1.4"
            /></span>
            <Image
              v-if="room.settings.display_mode !== 'welcome'"
              :size="64"
              stroke-width="1"
            />
            <h1>
              {{
                room.settings.display_mode === "welcome"
                  ? room.name
                  : t("room.displayWaiting")
              }}
            </h1>
            <p>
              {{
                t(
                  room.settings.display_mode === "welcome"
                    ? "app.tagline"
                    : "room.displayWaitingHelp",
                )
              }}
            </p>
          </div>
        </section>
        <aside class="display-invitation">
          <h2>{{ room.name }}</h2>
          <template v-if="!room.settings.join_locked"
            ><p>{{ t("room.scan") }}</p>
            <img
              v-if="qr"
              :src="qr"
              :alt="t('room.scan')"
              class="display-qr"
            /><strong class="room-code"
              >{{ room.code.slice(0, 3) }} {{ room.code.slice(3) }}</strong
            ><small>{{ t("room.roomCode") }}</small></template
          >
          <p v-else>{{ t("settings.joinLocked") }}</p>
        </aside>
      </div>
      <DisplayDanmaku
        v-if="
          room.state === 'active' &&
          room.settings.danmaku_mode &&
          room.settings.danmaku_mode !== 'off' &&
          (room.settings.display_source === 'content' || !screenActive) &&
          ['welcome', 'photos', 'poll'].includes(room.settings.display_mode)
        "
        :room="room.id"
        :stationary="room.settings.display_mode === 'poll'"
      />
      <footer class="display-footer">
        <span>{{
          room.settings.display_mode === "photos"
            ? t(
                room.settings.auto_display
                  ? "room.autoOn"
                  : "room.selectedOnly",
              )
            : room.name
        }}</span
        ><button @click="fullscreen">
          <Maximize :size="19" />{{ t("room.fullscreen") }}
        </button>
      </footer></template
    >
  </main>
</template>
