<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { MonitorUp, MonitorPlay, Square, Volume2 } from "lucide-vue-next";
import {
  Room,
  RoomEvent,
  Track,
  type LocalTrack,
  type RemoteTrack,
} from "livekit-client";
import { api, errorCode } from "../api";
import ScreenQueue from "./ScreenQueue.vue";
const queueMode = ref("free"),
  offer = ref("");
interface Stage {
  id: string;
  owner: string;
  name: string;
  expires: number;
}
interface Connection {
  client: string;
  token: string;
  url: string;
  screen: Stage;
}
const props = defineProps<{
  roomId: string;
  myId?: string;
  host?: boolean;
  display?: boolean;
  closed?: boolean;
}>();
const { t } = useI18n();
const emit = defineEmits<{ active: [value: boolean] }>();
const enabled = ref(false),
  stage = ref<Stage | null>(null),
  busy = ref(false),
  error = ref(""),
  state = ref("idle"),
  publishing = ref(false),
  media = ref<HTMLElement>(),
  statusLoaded = ref(false);
let room: Room | undefined;
watch(stage, (value) => emit("active", !!value));
let client = "";
let joinedStage = "";
let timer: ReturnType<typeof setInterval>;
let disposed = false;
let polling = false;
let disconnecting = false;
let localTracks: LocalTrack[] = [];
let lastHeartbeat = 0;
let failures = 0;
const supported = computed(
  () => window.isSecureContext && !!navigator.mediaDevices?.getDisplayMedia,
);
const endpoint = computed(
  () => `/rooms/${props.roomId}/screen${props.display ? "?display=1" : ""}`,
);
async function command(action: string, extra: Record<string, unknown> = {}) {
  return api<Connection>(endpoint.value, "POST", {
    action,
    offer: offer.value,
    ...extra,
  });
}
function attach(track: RemoteTrack | LocalTrack) {
  const el = track.attach();
  el.autoplay = true;
  if (el instanceof HTMLVideoElement) {
    el.playsInline = true;
    el.controls = true;
    if (publishing.value) el.muted = true;
  }
  if (track.kind === Track.Kind.Audio && publishing.value) {
    track.detach(el);
    return;
  }
  media.value?.append(el);
}
async function disconnect() {
  if (disconnecting) return;
  disconnecting = true;
  const previous = room;
  room = undefined;
  state.value = "idle";
  const oldClient = client;
  client = "";
  joinedStage = "";
  for (const track of localTracks) track.stop();
  localTracks = [];
  publishing.value = false;
  await previous?.disconnect();
  media.value?.replaceChildren();
  if (oldClient) void command("leave", { client: oldClient }).catch(() => {});
  disconnecting = false;
}
async function stop() {
  const wasPublisher = publishing.value;
  const screenID = stage.value?.id;
  await disconnect();
  if (wasPublisher || props.host) {
    try {
      await command("stop", { screen_id: screenID });
    } catch (e) {
      error.value = errorCode(e);
    }
  }
  await poll();
}
async function connect(publish = false) {
  if (busy.value || disposed) return;
  busy.value = true;
  error.value = "";
  let claimed = false;
  let claimedID = "";
  try {
    // Invoke capture directly in the click event, before waiting for network requests.
    let stream: MediaStream | undefined;
    if (publish) {
      if (!supported.value) {
        error.value = "SCREEN_UNSUPPORTED";
        return;
      }
      stream = await navigator.mediaDevices.getDisplayMedia({
        video: {
          width: { ideal: 1920 },
          height: { ideal: 1080 },
          frameRate: { ideal: 15, max: 30 },
        },
        audio: true,
      });
    }
    try {
      await disconnect();
      if (disposed) {
        stream?.getTracks().forEach((t) => t.stop());
        return;
      }
      const config = await command(publish ? "start" : "watch");
      claimed = publish;
      claimedID = config.screen.id;
      client = config.client;
      joinedStage = config.screen.id;
      stage.value = config.screen;
      publishing.value = publish;
      const current = new Room({ adaptiveStream: true, dynacast: true });
      room = current;
      current.on(RoomEvent.TrackSubscribed, attach);
      current.on(RoomEvent.TrackUnsubscribed, (track) =>
        track.detach().forEach((el) => el.remove()),
      );
      current.on(RoomEvent.Reconnecting, () => {
        state.value = "reconnecting";
      });
      current.on(RoomEvent.Reconnected, () => {
        state.value = "connected";
      });
      current.on(RoomEvent.Disconnected, () => {
        if (!disconnecting) {
          const wasPublisher = publishing.value;
          void disconnect().then(() => {
            if (wasPublisher)
              return command("stop", { screen_id: config.screen.id }).catch(
                () => {},
              );
          });
        }
      });
      state.value = "connecting";
      const url = new URL(config.url, location.origin);
      url.protocol = location.protocol === "https:" ? "wss:" : "ws:";
      await current.connect(url.toString(), config.token);
      if (disposed) {
        stream?.getTracks().forEach((t) => t.stop());
        await disconnect();
        if (claimed)
          await command("stop", { screen_id: claimedID }).catch(() => {});
        return;
      }
      if (stream) {
        for (const track of stream.getTracks()) {
          const pub = await current.localParticipant.publishTrack(track, {
            source:
              track.kind === "video"
                ? Track.Source.ScreenShare
                : Track.Source.ScreenShareAudio,
            videoEncoding: { maxBitrate: 3_000_000, maxFramerate: 15 },
            simulcast: true,
          });
          if (pub.track) {
            localTracks.push(pub.track);
            attach(pub.track);
          }
          if (track.kind === "video")
            track.addEventListener(
              "ended",
              () => {
                if (!disconnecting) void stop();
              },
              { once: true },
            );
        }
      }
      state.value = "connected";
      lastHeartbeat = Date.now();
      failures = 0;
    } catch (e) {
      stream?.getTracks().forEach((t) => t.stop());
      throw e;
    }
  } catch (e) {
    error.value =
      e instanceof DOMException
        ? e.name === "NotAllowedError"
          ? "SCREEN_DENIED"
          : "SCREEN_UNSUPPORTED"
        : errorCode(e) === "INTERNAL_ERROR"
          ? "MEDIA_UNAVAILABLE"
          : errorCode(e);
    await disconnect();
    if (claimed)
      await command("stop", { screen_id: claimedID }).catch(() => {});
  } finally {
    busy.value = false;
  }
}
async function poll() {
  if (polling || disposed) return;
  polling = true;
  try {
    const result = await api<{ enabled: boolean; screen: Stage | null }>(
      endpoint.value,
    );
    if (disposed) return;
    enabled.value = result.enabled;
    stage.value = result.screen;
    statusLoaded.value = true;
    if (client && joinedStage !== stage.value?.id) {
      await disconnect();
    }
    if (client && Date.now() - lastHeartbeat > 10_000) {
      await command("heartbeat", { client });
      lastHeartbeat = Date.now();
    }
    failures = 0;
    if (props.display && stage.value && !client && !busy.value && !props.closed)
      void connect();
  } catch (e) {
    failures++;
    if (failures >= 3) {
      await disconnect();
      error.value = errorCode(e);
    }
  } finally {
    polling = false;
  }
}
async function audio() {
  try {
    await room?.startAudio();
    await room?.startVideo();
  } catch {
    error.value = "MEDIA_UNAVAILABLE";
  }
}
watch(
  () => props.closed,
  (closed) => {
    if (closed) void disconnect();
  },
);
onMounted(() => {
  void poll();
  timer = setInterval(poll, 2000);
});
onBeforeUnmount(() => {
  emit('active',false);
  disposed = true;
  clearInterval(timer);
  const owned = publishing.value;
  const screenID = stage.value?.id;
  void disconnect().then(() => {
    if (owned) return command("stop", { screen_id: screenID }).catch(() => {});
  });
});
</script>
<template>
  <section
    v-show="!display || stage"
    class="screen-sharing"
    :class="{ 'screen-display': display }"
  >
    <header class="activity-heading">
      <div>
        <h2>{{ t("activity.screen") }}</h2>
        <p v-if="stage">{{ t("screen.sharing", { name: stage.name }) }}</p>
        <p v-else class="muted">{{ t("screen.help") }}</p>
      </div>
      <MonitorUp :size="28" />
    </header>
    <p v-if="!statusLoaded">{{ t("app.loading") }}</p>
    <p v-else-if="!enabled" class="activity-notice">
      {{ t("screen.notConfigured") }}
    </p>
    <p v-if="error" role="alert">{{ t("error." + error) }}</p>
    <ScreenQueue
      v-if="!display && enabled"
      :room="roomId"
      :my-id="myId"
      :host="host"
      :closed="closed"
      @offer="offer = $event"
      @mode="queueMode = $event"
    />
    <div
      ref="media"
      class="screen-media"
      :class="{ empty: state === 'idle' }"
    ></div>
    <div
      v-if="!stage && !display && state === 'idle' && !offer"
      class="screen-empty"
    >
      <span class="screen-empty-symbol"
        ><MonitorUp :size="38" stroke-width="1.3"
      /></span>
      <h3>{{ t("ui.screenEmpty") }}</h3>
      <p class="muted">{{ t("ui.screenEmptyHelp") }}</p>
    </div>
    <p v-if="state !== 'idle'" role="status">
      {{ t("screen.state." + state) }}
    </p>
    <div class="row wrap screen-actions">
      <button
        v-if="!display && !stage"
        class="primary"
        :disabled="
          busy ||
          !enabled ||
          closed ||
          !supported ||
          (queueMode !== 'free' && !offer)
        "
        @click="connect(true)"
      >
        <MonitorUp :size="18" />{{ t("screen.start") }}
      </button>
      <button
        v-if="stage && state === 'idle'"
        class="primary"
        :disabled="busy || closed"
        @click="connect()"
      >
        <MonitorPlay :size="18" />{{ t("screen.watch") }}
      </button>
      <button
        v-if="stage && !display && (host || stage.owner === myId)"
        :disabled="busy"
        @click="stop"
      >
        <Square :size="16" />{{ t("screen.stop") }}
      </button>
      <button v-if="state !== 'idle' && !publishing" @click="audio">
        <Volume2 :size="16" />{{ t("screen.audio") }}
      </button>
      <button
        v-if="state !== 'idle' && !display && !publishing"
        @click="disconnect"
      >
        {{ t("screen.leave") }}
      </button>
    </div>
    <p v-if="!display && !supported" class="muted">
      {{ t("screen.unsupported") }}
    </p>
    <p v-if="!display" class="muted small">{{ t("screen.privacy") }}</p>
  </section>
</template>
