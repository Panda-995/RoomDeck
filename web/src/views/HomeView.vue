<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import {
  Plus,
  ArrowUpRight,
  DoorOpen,
  LogOut,
  Clock,
  Layers,
} from "lucide-vue-next";
import { api, errorCode, notify } from "../api";
import type { Room, Status } from "../types";
import Modal from "../components/Modal.vue";
import { useFormat } from "../format";
const { t } = useI18n();
const { date, bytes } = useFormat();
const router = useRouter();
const rooms = ref<Room[]>([]),
  loading = ref(true),
  error = ref(""),
  showCreate = ref(false),
  busy = ref(false);
const name = ref(""),
  duration = ref(12 * 3600),
  retention = ref(86400),
  preset = ref("gathering"),
  modules = ref(["photo", "file", "note", "link", "poll", "display"]);
onMounted(load);
async function load() {
  try {
    const s = await api<Status>("/status");
    if (s.setup_required || !s.authenticated) {
      await router.replace(s.setup_required ? "/setup" : "/login");
      return;
    }
    rooms.value = await api<Room[]>("/rooms");
  } catch (e) {
    error.value = errorCode(e);
  } finally {
    loading.value = false;
  }
}
function applyPreset() {
  modules.value =
    preset.value === "quick"
      ? ["file", "note", "link"]
      : preset.value === "meeting"
        ? ["file", "note", "link", "poll", "display"]
        : ["photo", "file", "note", "link", "poll", "display"];
}
async function create() {
  busy.value = true;
  try {
    const room = await api<Room>("/rooms", "POST", {
      name: name.value,
      duration: duration.value,
      retention: retention.value,
      modules: modules.value,
    });
    await router.push("/room/" + room.id + "?invite=1");
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
async function logout() {
  try {
    await api("/logout", "POST");
    await router.push("/login");
  } catch (e) {
    notify(t("error." + errorCode(e)));
  }
}
</script>
<template>
  <main class="home-container">
    <div v-motion class="home-heading">
      <div>
        <p class="eyebrow">{{ t("app.host") }}</p>
        <h1>{{ t("home.title") }}</h1>
        <p class="muted">{{ t("home.subtitle") }}</p>
      </div>
      <div class="row">
        <button
          class="icon-button"
          :aria-label="t('app.logout')"
          @click="logout"
        >
          <LogOut :size="20" /></button
        ><button class="primary" @click="showCreate = true">
          <Plus :size="18" />{{ t("home.create") }}
        </button>
      </div>
    </div>
    <div v-if="loading" class="room-grid">
      <div v-for="i in 3" :key="i" class="skeleton room-skeleton"></div>
    </div>
    <p v-else-if="error" class="error">
      {{ t("error." + error) }}
      <button @click="load">{{ t("app.retry") }}</button>
    </p>
    <section v-else-if="!rooms.length" class="welcome-empty">
      <DoorOpen :size="54" stroke-width="1" />
      <h2>{{ t("home.noRooms") }}</h2>
      <p>{{ t("home.emptyHelp") }}</p>
      <button class="primary" @click="showCreate = true">
        <Plus :size="18" />{{ t("home.create") }}
      </button>
    </section>
    <template v-else
      ><section
        v-for="state in ['active', 'closed'].filter((state) =>
          rooms.some((r) =>
            state === 'active' ? r.state === 'active' : r.state !== 'active',
          ),
        )"
        :key="state"
        class="room-section"
      >
        <div class="section-head">
          <h2>{{ t(state === "active" ? "home.active" : "home.recent") }}</h2>
          <span class="count">{{
            rooms.filter((r) =>
              state === "active" ? r.state === "active" : r.state !== "active",
            ).length
          }}</span>
        </div>
        <div v-motion class="room-grid">
          <RouterLink
            v-for="room in rooms.filter((r) =>
              state === 'active' ? r.state === 'active' : r.state !== 'active',
            )"
            :key="room.id"
            :to="'/room/' + room.id"
            class="room-card"
            ><div class="row between">
              <span class="tag" :class="{ quiet: room.state !== 'active' }">{{
                t(room.state === "active" ? "room.live" : "room.closed")
              }}</span
              ><ArrowUpRight :size="21" />
            </div>
            <h2>{{ room.name }}</h2>
            <p class="muted mono">{{ room.code }}</p>
            <div class="card-modules">
              <span v-for="m in room.settings.modules" :key="m">{{
                t("module." + m)
              }}</span>
            </div>
            <footer>
              <span
                ><Clock :size="14" />{{
                  t(room.state === "active" ? "home.ends" : "home.deletes", {
                    time: date(
                      room.state === "active" ? room.ends_at : room.delete_at,
                    ),
                  })
                }}</span
              ><small>{{ bytes(room.used) }}</small>
            </footer></RouterLink
          >
        </div>
      </section></template
    >
    <footer class="home-footer">
      <span><Layers :size="15" />RoomDeck · {{ t("app.tagline") }}</span
      ><RouterLink to="/join" class="text-link"
        >{{ t("home.join") }} →</RouterLink
      >
    </footer>
    <DialogTransition
      ><Modal
        v-if="showCreate"
        :title="t('home.create')"
        :busy="busy"
        @close="showCreate = false"
        ><form class="form-stack" @submit.prevent="create">
          <label
            >{{ t("home.roomName")
            }}<input
              v-model="name"
              :placeholder="t('home.namePlaceholder')"
              maxlength="40"
              required
              autofocus /></label
          ><label
            >{{ t("home.preset")
            }}<select v-pretty-select v-model="preset" @change="applyPreset">
              <option value="gathering">{{ t("home.gathering") }}</option>
              <option value="meeting">{{ t("home.meeting") }}</option>
              <option value="quick">{{ t("home.quick") }}</option>
            </select></label
          >
          <div class="form-two">
            <label
              >{{ t("home.duration")
              }}<select v-pretty-select v-model="duration">
                <option v-for="n in [2, 6, 12, 24]" :key="n" :value="n * 3600">
                  {{ t("home.hours", { n }) }}
                </option>
              </select></label
            ><label
              >{{ t("home.retention")
              }}<select v-pretty-select v-model="retention">
                <option v-for="n in [1, 7, 30]" :key="n" :value="n * 86400">
                  {{ t("home.days", { n }) }}
                </option>
              </select></label
            >
          </div>
          <fieldset>
            <legend>{{ t("home.modules") }}</legend>
            <div class="module-checks">
              <label
                v-for="m in [
                  'photo',
                  'file',
                  'note',
                  'link',
                  'poll',
                  'display',
                ]"
                :key="m"
                ><input v-model="modules" type="checkbox" :value="m" />{{
                  t("module." + m)
                }}</label
              >
            </div>
          </fieldset>
          <button class="primary" :disabled="busy || !modules.length">
            {{ t(busy ? "app.working" : "home.createGo")
            }}<ArrowUpRight :size="18" />
          </button></form></Modal
    ></DialogTransition>
  </main>
</template>
