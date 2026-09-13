<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import {
  Plus,
  ArrowLeft,
  ArrowUpRight,
  Monitor,
  Users,
  Settings,
  Image,
  LayoutGrid,
  Dices,
  Pause,
  Play,
  Square,
  Pin,
  Download,
  DoorClosed,
  ScanLine,
  Trash2,
  Check,
  WifiOff,
} from "lucide-vue-next";
import { api, errorCode, notify, assetURL } from "../api";
import type { Settings as RoomSettings } from "../types";
import { useFormat } from "../format";
import { useRoomSync } from "../roomSync";
import ContentCard from "../components/ContentCard.vue";
import Modal from "../components/Modal.vue";
import ShareModal from "../components/ShareModal.vue";
import SettingsModal from "../components/SettingsModal.vue";
import InviteModal from "../components/InviteModal.vue";
import MotionNav from "../components/MotionNav.vue";
import BoardPanel from "../components/BoardPanel.vue";
import CohostPermissions from "../components/CohostPermissions.vue";
import GameCenter from "../components/GameCenter.vue";
import InteractionPanel from "../components/InteractionPanel.vue";
import { roomUploads, restoreUploads } from "../uploadTasks";
const ScreenShare = defineAsyncComponent(
  () => import("../components/ScreenShare.vue"),
);
const { t } = useI18n();
const { date, bytes } = useFormat();
const route = useRoute(),
  router = useRouter();
const id = String(route.params.id);
const {
  snapshot,
  connected,
  state: connectionState,
  error,
  refresh,
  start,
} = useRoomSync(id);
const filter = ref("all"),
  panel = ref(""),
  busy = ref(false),
  confirmName = ref("");
const screenOpened = ref(false),
  gamesOpened = ref(false);
const motionDirection = ref(0);
watch(filter, (value, previous) => {
  const position = (key: string) =>
    key === "screen" ? 1 : key === "games" ? 2 : 0;
  motionDirection.value = Math.sign(position(value) - position(previous));
  if (value === "screen") screenOpened.value = true;
  if (value === "games") gamesOpened.value = true;
});
const host = computed(() => snapshot.value?.role === "host");
const gameArea = ref("party");
const can = (scope: string) =>
  host.value || !!snapshot.value?.permissions?.includes(scope);
const room = computed(() => snapshot.value?.room);
const photos = computed(
  () =>
    snapshot.value?.contents.filter((c) => c.kind === "photo" && c.visible) ||
    [],
);
const selectedPhoto = computed(() =>
  photos.value.find((c) => c.selected || room.value?.settings.auto_display),
);
const items = computed(
  () =>
    snapshot.value?.contents.filter(
      (c) =>
        filter.value === "all" ||
        (filter.value === "resources"
          ? c.kind !== "photo"
          : c.kind === filter.value),
    ) || [],
);
const controlColumn = ref<HTMLElement>();
function showControls() {
  controlColumn.value?.scrollIntoView({
    block: "start",
    behavior: matchMedia("(prefers-reduced-motion: reduce)").matches
      ? "instant"
      : "smooth",
  });
}
const latestJob = computed(() => snapshot.value?.jobs?.[0]);
const pendingUploads = computed(
  () =>
    roomUploads(id, snapshot.value?.participant_id || "").filter(
      (task) => task.state !== "ready",
    ).length,
);
const displayPolls = computed(
  () =>
    snapshot.value?.contents.filter((c) => c.kind === "poll" && c.visible) ||
    [],
);
async function selectDisplayPoll(target: string) {
  if (!room.value || busy.value) return;
  busy.value = true;
  try {
    await api(`/rooms/${id}/display/control`, "POST", {
      mode: "poll",
      target,
      source: "content",
      expected_version: room.value.version,
    });
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    await refresh();
    busy.value = false;
  }
}
onMounted(async () => {
  await start();
  if (snapshot.value)
    void restoreUploads(id, snapshot.value.participant_id).catch(() => {});
  if (route.query.invite === "1" && host.value) panel.value = "invite";
});
async function changeSettings(changes: Partial<RoomSettings>) {
  if (!room.value) return;
  busy.value = true;
  try {
    await api("/rooms/" + id, "PATCH", {
      name: room.value.name,
      settings: { ...room.value.settings, ...changes },
      retention: room.value.retention,
      version: room.value.version,
    });
    await refresh();
  } catch (e) {
    notify(t("error." + errorCode(e)));
    await refresh();
  } finally {
    busy.value = false;
  }
}
async function display() {
  const displayWindow = window.open("about:blank", "_blank");
  if (!displayWindow) {
    notify(t("ui.popupBlocked"));
    return;
  }
  displayWindow.opener = null;
  displayWindow.document.title = "RoomDeck";
  busy.value = true;
  try {
    const result = await api<{ token: string }>(
      "/rooms/" + id + "/display-session",
      "POST",
    );
    displayWindow.location.replace("/display#pair=" + result.token);
  } catch (e) {
    displayWindow.close();
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
async function closeRoom() {
  busy.value = true;
  try {
    await api("/rooms/" + id + "/close", "POST");
    panel.value = "";
    await refresh();
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
async function removeRoom() {
  busy.value = true;
  try {
    await api("/rooms/" + id, "DELETE", { name: confirmName.value });
    await router.push("/");
    notify(t("home.deleted"));
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
async function exportRoom() {
  busy.value = true;
  try {
    await api("/rooms/" + id + "/exports", "POST");
    await refresh();
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
async function member(idMember: string, revoked: boolean, muted: boolean) {
  try {
    await api("/rooms/" + id + "/participants/" + idMember, "PATCH", {
      revoked,
      muted,
    });
    await refresh();
  } catch (e) {
    notify(t("error." + errorCode(e)));
  }
}
</script>
<template>
  <main v-if="error && !snapshot" class="error-page">
    <WifiOff :size="40" />
    <h1>{{ t("error." + error) }}</h1>
    <RouterLink class="button" to="/join">{{ t("home.join") }}</RouterLink
    ><RouterLink class="text-link" to="/">{{ t("home.title") }}</RouterLink>
  </main>
  <main v-else-if="!snapshot || !room" class="home-container">
    <div class="skeleton block"></div>
  </main>
  <main v-else class="workspace" :class="{ 'guest-workspace': !host }">
    <aside v-if="host" class="sidebar">
      <RouterLink to="/" class="sidebar-back"
        ><ArrowLeft :size="16" />{{ t("home.title") }}</RouterLink
      >
      <div class="sidebar-room">
        <span class="room-initial">{{ room.name.slice(0, 1) }}</span
        ><strong>{{ room.name }}</strong
        ><small class="mono">{{ room.code }}</small>
      </div>
      <button @click="showControls">
        <Monitor :size="18" />{{ t("room.hostControl") }}
      </button>
      <button @click="panel = 'members'">
        <Users :size="18" />{{ t("room.members")
        }}<span class="count">{{ snapshot.participants_total }}</span></button
      ><button @click="panel = 'settings'">
        <Settings :size="18" />{{ t("app.settings") }}
      </button>
      <div class="sidebar-bottom">
        <div class="row">
          <span class="avatar">{{ snapshot.name.slice(0, 1) }}</span>
          <div>
            <strong>{{ snapshot.name }}</strong
            ><small>{{ t("app.host") }}</small>
          </div>
        </div>
        <div class="quota">
          <span
            :style="{
              width:
                Math.min(100, (room.used / room.settings.quota) * 100) + '%',
            }"
          ></span>
        </div>
        <small>{{ bytes(room.used) }} / {{ bytes(room.settings.quota) }}</small>
      </div>
    </aside>
    <section class="workspace-main">
      <header v-motion class="room-heading">
        <div>
          <div class="row wrap">
            <h1>{{ room.name }}</h1>
            <span class="tag" :class="{ quiet: room.state !== 'active' }">{{
              t(room.state === "active" ? "room.live" : "room.closed")
            }}</span>
          </div>
          <div class="room-meta">
            <span class="live-status" :class="{ disconnected: !connected }">{{
              connected
                ? t("app.online", { n: snapshot.online })
                : t("connection." + connectionState)
            }}</span
            ><span>{{
              t(room.state === "active" ? "home.ends" : "home.deletes", {
                time: date(
                  room.state === "active" ? room.ends_at : room.delete_at,
                ),
              })
            }}</span
            ><span class="mono">{{ room.code }}</span>
          </div>
        </div>
        <div v-if="host" class="row heading-actions">
          <button v-if="room.state === 'active'" @click="panel = 'invite'">
            <ScanLine :size="17" />{{ t("room.invite") }}</button
          ><button
            v-if="room.settings.modules.includes('display')"
            class="primary"
            :disabled="busy"
            @click="display"
          >
            <Monitor :size="17" />{{ t("room.openDisplay")
            }}<ArrowUpRight :size="16" />
          </button>
        </div>
      </header>
      <section v-if="room.state !== 'active'" class="ended-banner">
        <Check :size="26" />
        <div>
          <h2>{{ t("room.endedTitle") }}</h2>
          <p>{{ t("room.endedHelp", { time: date(room.delete_at) }) }}</p>
        </div>
        <button v-if="host" @click="panel = 'export'">
          <Download :size="17" />{{ t("room.export") }}
        </button>
      </section>
      <nav v-if="host" class="host-mobile-tools" :aria-label="t('room.manage')">
        <button @click="showControls">
          <Monitor :size="17" />{{ t("room.hostControl") }}
        </button>
        <button @click="panel = 'members'">
          <Users :size="17" />{{ t("room.members") }}
        </button>
        <button @click="panel = 'settings'">
          <Settings :size="17" />{{ t("app.settings") }}
        </button>
      </nav>
      <div class="workspace-columns">
        <section class="feed">
          <MotionNav
            class="tabs"
            :active-key="['screen', 'games'].includes(filter) ? filter : 'all'"
            :label="t('ui.roomNavigation')"
          >
            <button
              :class="{ active: !['screen', 'games'].includes(filter) }"
              :aria-pressed="!['screen', 'games'].includes(filter)"
              @click="filter = 'all'"
            >
              <LayoutGrid :size="18" />{{ t("ui.content") }}
            </button>
            <button
              :class="{ active: filter === 'screen' }"
              :aria-pressed="filter === 'screen'"
              @click="filter = 'screen'"
            >
              <Monitor :size="18" />{{ t("activity.screen") }}
            </button>
            <button
              :class="{ active: filter === 'games' }"
              :aria-pressed="filter === 'games'"
              @click="filter = 'games'"
            >
              <Dices :size="18" />{{ t("activity.games") }}
            </button>
          </MotionNav>
          <div
            v-if="!host && snapshot.permissions?.length"
            class="cohost-banner"
          >
            <strong>{{ t("cohost.you") }}</strong
            ><span v-for="scope in snapshot.permissions" :key="scope">{{
              t("cohost." + scope)
            }}</span
            ><button v-if="can('display')" @click="showControls">
              {{ t("room.hostControl") }}
            </button>
          </div>
          <InteractionPanel
            v-show="!['screen', 'games'].includes(filter)"
            :room="room"
            :host="can('content')"
            @settings="changeSettings"
          />
          <button
            v-if="pendingUploads"
            class="upload-task-entry"
            @click="panel = 'share'"
          >
            {{ t("uploads.tasks", { n: pendingUploads }) }}
          </button>
          <div
            v-if="!['screen', 'games'].includes(filter)"
            class="section-head"
          >
            <div>
              <h2>{{ t("room.title") }}</h2>
              <p class="muted small">{{ t("room.subtitle") }}</p>
            </div>
            <button
              v-if="room.state === 'active'"
              class="share-top"
              :disabled="!connected"
              @click="panel = 'share'"
            >
              <Plus :size="18" />{{ t("room.share") }}
            </button>
          </div>
          <div v-if="!['screen', 'games'].includes(filter)" class="feed-stats">
            <span>{{ t("room.photoCount", { n: photos.length }) }}</span
            ><span>{{
              t("room.fileCount", {
                n: snapshot.contents.filter((c) => c.kind === "file").length,
              })
            }}</span
            ><span>{{
              t("room.peopleCount", { n: snapshot.participants_total })
            }}</span>
          </div>

          <nav
            v-if="!['screen', 'games'].includes(filter)"
            class="filter-chips"
            :aria-label="t('ui.contentTypes')"
          >
            <button
              :class="{ active: filter === 'all' }"
              :aria-pressed="filter === 'all'"
              @click="filter = 'all'"
            >
              {{ t("app.all") }}
            </button>
            <button
              v-for="m in room.settings.modules.filter((m) => m !== 'display')"
              :key="m"
              :class="{ active: filter === m }"
              :aria-pressed="filter === m"
              @click="filter = m"
            >
              {{ t("module." + m) }}
            </button>
          </nav>
          <ScreenShare
            v-motion="{ key: filter === 'screen', direction: motionDirection }"
            v-if="screenOpened"
            v-show="filter === 'screen'"
            :room-id="id"
            :my-id="snapshot.participant_id"
            :host="can('screen')"
            :closed="room.state !== 'active'"
          />
          <div
            v-if="gamesOpened"
            v-show="filter === 'games'"
            class="board-navigation"
            role="group"
            :aria-label="t('board.navigation')"
          >
            <button
              :aria-pressed="gameArea === 'party'"
              @click="gameArea = 'party'"
            >
              {{ t("board.party") }}</button
            ><button
              :aria-pressed="gameArea === 'board'"
              @click="gameArea = 'board'"
            >
              {{ t("board.title") }}
            </button>
          </div>
          <BoardPanel
            v-if="gamesOpened && gameArea === 'board'"
            v-show="filter === 'games'"
            :room-id="id"
            :my-id="snapshot.participant_id"
            :host="can('games')"
            :closed="room.state !== 'active'"
            :visible="filter === 'games'"
          />
          <GameCenter
            v-motion="{
              key: filter === 'games' && gameArea === 'party',
              direction: motionDirection,
            }"
            v-if="gamesOpened"
            v-show="filter === 'games' && gameArea === 'party'"
            :room-id="id"
            :my-id="snapshot.participant_id"
            :host="can('games')"
            :closed="room.state !== 'active'"
            :visible="filter === 'games' && gameArea === 'party'"
          />
          <template v-if="!['screen', 'games'].includes(filter)">
            <div v-if="!items.length" v-motion="filter" class="feed-empty">
              <Image :size="45" stroke-width="1" />
              <h2>
                {{
                  t(filter === "photo" ? "room.emptyPhotos" : "room.emptyTitle")
                }}
              </h2>
              <p>{{ t("room.emptyHelp") }}</p>
              <button
                v-if="room.state === 'active'"
                class="primary"
                :disabled="!connected"
                @click="panel = 'share'"
              >
                <Plus :size="18" />{{ t("room.share") }}
              </button>
            </div>
            <div
              v-else
              v-motion="{ key: filter, direction: motionDirection }"
              class="content-grid"
            >
              <ContentCard
                v-for="c in items"
                :key="c.id"
                :content="c"
                :room="room"
                :host="can('content')"
                :my-i-d="snapshot.participant_id"
                @changed="refresh"
              />
            </div>
          </template>
        </section>
        <aside
          v-if="host || can('display')"
          ref="controlColumn"
          class="control-column"
        >
          <section
            v-if="room.settings.modules.includes('display')"
            class="control-panel"
          >
            <div class="section-head">
              <h2>{{ t("room.display") }}</h2>
              <Monitor :size="19" />
            </div>
            <p class="muted small">
              {{
                snapshot.display_online
                  ? t("room.displayCount", { n: snapshot.display_online })
                  : t("room.displayNone")
              }}
            </p>
            <div class="display-preview">
              <img
                v-if="selectedPhoto && room.settings.display_mode === 'photos'"
                :src="assetURL(id, selectedPhoto.id, 'preview')"
                alt=""
              />
              <div>
                <h3>
                  {{
                    room.settings.display_mode === "blank"
                      ? t("room.blank")
                      : room.name
                  }}
                </h3>
                <small v-if="room.settings.display_mode === 'poll'">{{
                  displayPolls.find(
                    (p) => p.id === room?.settings.display_target,
                  )?.title || t("displayControl.empty")
                }}</small>
                <small v-else>{{
                  t(
                    "room." +
                      ({
                        welcome: "welcome",
                        photos: "photos",
                        pinned: "pinned",
                        poll: "postPoll",
                        blank: "blank",
                        board: "board",
                      }[room.settings.display_mode] || "welcome"),
                  )
                }}</small>
              </div>
            </div>
            <div class="display-modes">
              <button
                v-for="m in ['welcome', 'photos', 'pinned', 'poll', 'board']"
                :key="m"
                :class="{ chosen: room.settings.display_mode === m }"
                :aria-pressed="room.settings.display_mode === m"
                :disabled="busy"
                @click="
                  changeSettings({ display_mode: m, display_source: 'content' })
                "
              >
                {{
                  t(
                    m === "photos"
                      ? "room.photos"
                      : m === "poll"
                        ? "module.poll"
                        : "room." + m,
                  )
                }}
              </button>
            </div>
            <label
              v-if="room.settings.display_mode === 'poll'"
              class="display-poll-picker"
            >
              {{ t("displayControl.choose") }}
              <select
                v-pretty-select
                :value="room.settings.display_target"
                :disabled="busy || !room.settings.modules.includes('poll')"
                @change="
                  selectDisplayPoll(($event.target as HTMLSelectElement).value)
                "
              >
                <option value="">{{ t("displayControl.empty") }}</option>
                <option
                  v-for="poll in displayPolls"
                  :key="poll.id"
                  :value="poll.id"
                >
                  {{ poll.title }}
                </option>
              </select>
            </label>
            <button
              class="display-source-button"
              :disabled="busy"
              @click="
                changeSettings({
                  display_source:
                    room.settings.display_source === 'screen'
                      ? 'content'
                      : 'screen',
                })
              "
            >
              {{
                t(
                  room.settings.display_source === "screen"
                    ? "displayControl.content"
                    : "displayControl.screen",
                )
              }}
            </button>
            <div class="row">
              <button
                class="grow"
                :disabled="busy"
                @click="
                  changeSettings({
                    display_paused: !room.settings.display_paused,
                  })
                "
              >
                <Pause v-if="!room.settings.display_paused" :size="16" /><Play
                  v-else
                  :size="16"
                />{{
                  t(room.settings.display_paused ? "room.resume" : "room.pause")
                }}</button
              ><button
                class="icon-button"
                :title="t('room.blank')"
                :aria-label="t('room.blank')"
                :disabled="busy"
                @click="changeSettings({ display_mode: 'blank' })"
              >
                <Square :size="18" />
              </button>
            </div>
            <label class="inline-label"
              >{{ t("room.interval")
              }}<select
                v-pretty-select
                :value="room.settings.interval"
                @change="
                  changeSettings({
                    interval: Number(
                      ($event.target as HTMLSelectElement).value,
                    ),
                  })
                "
              >
                <option v-for="n in [5, 10, 20]" :key="n" :value="n">
                  {{ n }}
                </option>
              </select></label
            >
            <p class="muted small">
              <Pin :size="13" />
              {{
                t(
                  room.settings.auto_display
                    ? "room.autoOn"
                    : "room.selectedOnly",
                )
              }}
            </p>
          </section>
          <section v-if="host" class="control-panel room-actions">
            <h2>{{ t("room.manage") }}</h2>
            <button @click="panel = 'settings'">
              <Settings :size="17" />{{ t("app.settings") }}</button
            ><button @click="panel = 'members'">
              <Users :size="17" />{{ t("room.members") }}</button
            ><button @click="panel = 'export'">
              <Download :size="17" />{{ t("room.export") }}</button
            ><button v-if="room.state === 'active'" @click="panel = 'end'">
              <DoorClosed :size="17" />{{ t("room.end") }}</button
            ><button class="danger-text" @click="panel = 'delete'">
              <Trash2 :size="17" />{{ t("room.deleteRoom") }}
            </button>
          </section>
        </aside>
      </div>
    </section>
    <MotionNav
      v-if="!host"
      class="guest-bottom"
      :label="t('ui.roomNavigation')"
      :active-key="['screen', 'games'].includes(filter) ? filter : 'all'"
    >
      <button
        :class="{ active: !['screen', 'games'].includes(filter) }"
        :aria-pressed="!['screen', 'games'].includes(filter)"
        @click="filter = 'all'"
      >
        <LayoutGrid :size="21" />{{ t("ui.content") }}
      </button>
      <button
        :class="{ active: filter === 'screen' }"
        :aria-pressed="filter === 'screen'"
        @click="filter = 'screen'"
      >
        <Monitor :size="21" />{{ t("activity.screen") }}
      </button>
      <button
        v-if="room.state === 'active'"
        class="primary"
        :disabled="!connected"
        @click="panel = 'share'"
      >
        <Plus :size="22" />{{ t("room.share") }}
      </button>
      <button
        :class="{ active: filter === 'games' }"
        :aria-pressed="filter === 'games'"
        @click="filter = 'games'"
      >
        <Dices :size="21" />{{ t("activity.games") }}
      </button>
    </MotionNav>
    <DialogTransition
      ><ShareModal
        :identity="snapshot.participant_id"
        v-if="panel === 'share'"
        :room="room"
        :host="host"
        @close="panel = ''"
        @shared="refresh" /></DialogTransition
    ><DialogTransition
      ><SettingsModal
        v-if="panel === 'settings' && host"
        :room="room"
        @close="panel = ''"
        @changed="refresh" /></DialogTransition
    ><DialogTransition
      ><InviteModal
        v-if="panel === 'invite' && host"
        :snapshot="snapshot"
        @close="panel = ''"
        @changed="refresh"
    /></DialogTransition>
    <DialogTransition
      ><Modal
        v-if="panel === 'members' && host"
        :title="t('room.members')"
        @close="panel = ''"
        ><p class="muted small">{{ t("room.memberHint") }}</p>
        <div v-for="m in snapshot.members" :key="m.id" class="member-row">
          <span class="avatar">{{ m.name.slice(0, 1) }}</span>
          <div class="grow">
            <strong>{{ m.name }}</strong
            ><small>{{
              m.revoked ? t("room.removed") : m.online ? t("app.connected") : ""
            }}</small>
          </div>
          <template v-if="!m.revoked"
            ><button class="compact" @click="member(m.id, false, !m.muted)">
              {{ t(m.muted ? "room.unmute" : "room.mute") }}</button
            ><button
              class="icon-button"
              :aria-label="t('room.removeMember')"
              @click="member(m.id, true, m.muted)"
            >
              <Trash2 :size="17" /></button></template
          ><CohostPermissions
            v-if="!m.revoked"
            :room="id"
            :member="m.id"
            :permissions="m.permissions || []"
            :disabled="room.state !== 'active'"
            @changed="refresh"
          /></div></Modal
    ></DialogTransition>
    <DialogTransition
      ><Modal
        v-if="panel === 'end'"
        :busy="busy"
        :title="t('room.endTitle')"
        @close="panel = ''"
        ><p class="muted">{{ t("room.endHelp") }}</p>
        <div class="modal-actions">
          <button @click="panel = ''">{{ t("app.cancel") }}</button
          ><button class="primary" :disabled="busy" @click="closeRoom">
            {{ t("room.endConfirm") }}
          </button>
        </div></Modal
      ></DialogTransition
    >
    <DialogTransition
      ><Modal
        v-if="panel === 'delete'"
        :busy="busy"
        :title="t('room.deleteRoom')"
        @close="panel = ''"
        ><p class="muted">{{ t("room.deleteRoomHint") }}</p>
        <form class="form-stack" @submit.prevent="removeRoom">
          <label
            >{{ t("room.deleteName")
            }}<input
              v-model="confirmName"
              :placeholder="room.name"
              required /></label
          ><button class="danger" :disabled="confirmName !== room.name || busy">
            {{ t("room.deleteRoom") }}
          </button>
        </form></Modal
      ></DialogTransition
    >
    <DialogTransition
      ><Modal
        v-if="panel === 'export'"
        :title="t('room.export')"
        @close="panel = ''"
        ><p class="muted">{{ t("room.exportHelp") }}</p>
        <a class="button" :href="'/api/v1/rooms/' + id + '/storage'" download>{{
          t("activity.storage")
        }}</a>
        <div class="export-status">
          <template v-if="latestJob"
            ><a
              v-if="latestJob.state === 'ready'"
              class="button primary full"
              :href="'/api/v1/rooms/' + id + '/exports/' + latestJob.id"
              ><Download :size="18" />{{ t("room.ready") }} ·
              {{ bytes(latestJob.bytes) }}</a
            >
            <p v-else-if="['queued', 'running'].includes(latestJob.state)">
              {{ t("room." + latestJob.state) }}
            </p>
            <p v-else class="error">
              {{ t("error." + latestJob.error) }}
            </p></template
          ><button
            v-if="
              !latestJob ||
              latestJob.state === 'failed' ||
              latestJob.state === 'ready'
            "
            class="primary full"
            :disabled="busy"
            @click="exportRoom"
          >
            <Download :size="18" />{{
              t(
                latestJob?.state === "ready"
                  ? "room.refreshExport"
                  : "room.export",
              )
            }}
          </button>
        </div></Modal
      ></DialogTransition
    >
  </main>
</template>
