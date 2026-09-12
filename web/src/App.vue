<script setup lang="ts">
import { watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute } from "vue-router";
import { Languages, PanelsTopLeft } from "lucide-vue-next";
import { notification } from "./api";
import { modalStack } from "./modalState";
const { t, locale } = useI18n();
const route = useRoute();
watch(
  locale,
  (v) => {
    notification.value = "";
    localStorage.setItem("roomdeck-language", v);
    document.documentElement.lang = v === "zh" ? "zh-CN" : "en";
  },
  { immediate: true },
);
</script>
<template>
  <div
    v-if="!route.path.startsWith('/display')"
    class="room-backdrop"
    aria-hidden="true"
  ></div>
  <header
    class="app-header"
    :class="{ 'dark-header': route.path.startsWith('/display') }"
  >
    <RouterLink class="brand" to="/" :aria-label="t('app.brandLabel')"
      ><span class="logo"><PanelsTopLeft :size="20" /></span
      >RoomDeck</RouterLink
    >
    <div class="language-control">
      <Languages :size="17" /><label class="sr-only" for="language">{{
        t("app.language")
      }}</label
      ><select v-pretty-select id="language" v-model="locale">
        <option value="zh">简体中文</option>
        <option value="en">English</option>
      </select>
    </div>
  </header>
  <RouterView :key="route.path" />
  <Transition name="toast"
    ><div
      v-if="notification && !modalStack.length"
      class="toast-message"
      role="status"
    >
      {{ notification }}
    </div></Transition
  >
</template>
