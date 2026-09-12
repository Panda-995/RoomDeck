<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { ArrowRight, DoorOpen } from "lucide-vue-next";
import { api, errorCode } from "../api";
import { useFormat } from "../format";
const { t } = useI18n();
const { date } = useFormat();
const route = useRoute(),
  router = useRouter();
const code = ref(String(route.query.code || "")),
  token = ref(""),
  name = ref(localStorage.getItem("roomdeck-nickname") || ""),
  busy = ref(false),
  error = ref("");
const info = ref<{
  name: string;
  ends_at: number;
  retention: number;
  auto_display: boolean;
}>();
onMounted(() => {
  const params = new URLSearchParams(location.hash.slice(1));
  token.value = params.get("invite") || "";
  if (token.value) {
    history.replaceState(null, "", location.pathname);
    void lookup();
  } else if (code.value) {
    void lookup();
  }
});
async function lookup() {
  busy.value = true;
  error.value = "";
  try {
    info.value = await api("/invite-info", "POST", {
      code: code.value,
      token: token.value,
      name: "",
    });
  } catch (e) {
    error.value = errorCode(e);
  } finally {
    busy.value = false;
  }
}
async function join() {
  busy.value = true;
  error.value = "";
  try {
    const res = await api<{ room_id: string }>("/join", "POST", {
      code: code.value,
      token: token.value,
      name: name.value,
    });
    localStorage.setItem("roomdeck-nickname", name.value);
    await router.push("/room/" + res.room_id);
  } catch (e) {
    error.value = errorCode(e);
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <main v-motion class="join-container">
    <div class="join-symbol"><DoorOpen :size="38" stroke-width="1.3" /></div>
    <span class="tag">{{ t("join.title") }}</span>
    <h1>{{ info?.name || t("app.tagline") }}</h1>
    <p class="muted">{{ t("join.subtitle") }}</p>
    <p v-if="info" class="join-time">
      {{ t("home.ends", { time: date(info.ends_at) }) }}
    </p>
    <form class="form-stack" @submit.prevent="info ? join() : lookup()">
      <label v-if="!info"
        >{{ t("join.code")
        }}<input
          v-model="code"
          required
          maxlength="10"
          class="code-input"
          autocomplete="off"
          autofocus
          placeholder="ABCD23"
        /><small>{{ t("join.codeHint") }}</small></label
      ><template v-else
        ><label
          >{{ t("join.nickname")
          }}<input
            v-model="name"
            required
            maxlength="20"
            autocomplete="nickname"
            :placeholder="t('join.namePlaceholder')"
        /></label>
        <div class="notice">
          <p>
            {{ t(info.auto_display ? "join.autoPrivacy" : "join.privacy") }}
          </p>
          <p>{{ t("join.retention", { hours: info.retention / 3600 }) }}</p>
        </div></template
      >
      <p v-if="error" class="error" role="alert">{{ t("error." + error) }}</p>
      <button class="primary" :disabled="busy">
        {{ t(busy ? "app.working" : info ? "join.enter" : "join.lookup")
        }}<ArrowRight :size="18" />
      </button>
    </form>
    <button
      v-if="info || error"
      class="text-button"
      @click="
        info = undefined;
        token = '';
        error = '';
      "
    >
      {{ t("join.different") }}
    </button>
  </main>
</template>
