<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { ArrowRight, ScanLine, ShieldCheck } from "lucide-vue-next";
import { api, errorCode } from "../api";
import type { Status } from "../types";
const { t } = useI18n();
const router = useRouter();
const setup = ref(false),
  ready = ref(false),
  busy = ref(false),
  error = ref("");
const username = ref(""),
  password = ref(""),
  token = ref("");
onMounted(async () => {
  try {
    const s = await api<Status>("/status");
    if (s.authenticated) {
      await router.replace("/");
      return;
    }
    setup.value = s.setup_required;
    ready.value = true;
  } catch (e) {
    error.value = errorCode(e);
  }
});
async function submit() {
  busy.value = true;
  error.value = "";
  try {
    await api(setup.value ? "/setup" : "/login", "POST", {
      username: username.value,
      password: password.value,
      ...(setup.value ? { token: token.value } : {}),
    });
    await router.push("/");
  } catch (e) {
    error.value = errorCode(e);
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <main class="auth-layout">
    <aside class="auth-story">
      <span class="brand-mark" aria-hidden="true"><ScanLine :size="30" /></span>
      <h1>{{ t("app.tagline") }}</h1>
      <p>{{ t("auth.private") }}</p>
      <div class="story-mosaic" aria-hidden="true">
        <div><ScanLine :size="38" /></div>
        <div><ShieldCheck :size="38" /></div>
        <div>Room<br />Deck.</div>
      </div>
    </aside>
    <section v-motion class="auth-form">
      <div v-if="!ready && !error" class="skeleton block"></div>
      <template v-else
        ><span class="eyebrow">RoomDeck</span>
        <h1>{{ t(setup ? "auth.setupTitle" : "auth.loginTitle") }}</h1>
        <p class="muted">
          {{ t(setup ? "auth.setupDesc" : "auth.loginDesc") }}
        </p>
        <form class="form-stack" @submit.prevent="submit">
          <label v-if="setup"
            >{{ t("auth.token")
            }}<input v-model="token" required autocomplete="off" /><small>{{
              t("auth.tokenHelp")
            }}</small></label
          ><label
            >{{ t("auth.username")
            }}<input
              v-model="username"
              required
              maxlength="40"
              autocomplete="username" /></label
          ><label
            >{{ t("auth.password")
            }}<input
              v-model="password"
              type="password"
              required
              :minlength="setup ? 12 : 1"
              maxlength="72"
              :autocomplete="setup ? 'new-password' : 'current-password'"
            /><small v-if="setup">{{ t("auth.passwordHint") }}</small></label
          >
          <p v-if="error" class="error" role="alert">
            {{ t("error." + error) }}
          </p>
          <button class="primary" :disabled="busy || !ready">
            {{ t(busy ? "app.working" : setup ? "auth.setup" : "auth.login")
            }}<ArrowRight :size="18" />
          </button>
        </form>
        <RouterLink to="/join" class="text-link auth-link"
          >{{ t("auth.joinInstead") }} →</RouterLink
        ></template
      >
    </section>
  </main>
</template>
