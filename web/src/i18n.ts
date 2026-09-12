import { createI18n } from "vue-i18n";
import zh from "./locales/zh.json";
import en from "./locales/en.json";
const saved = localStorage.getItem("roomdeck-language");
export const i18n = createI18n({
  legacy: false,
  locale:
    saved === "en" || saved === "zh"
      ? saved
      : navigator.language.startsWith("zh")
        ? "zh"
        : "en",
  fallbackLocale: "en",
  messages: { zh, en },
});
