import { useI18n } from "vue-i18n";
export function useFormat() {
  const { locale, t } = useI18n();
  return {
    date: (seconds: number) =>
      new Intl.DateTimeFormat(locale.value === "zh" ? "zh-CN" : "en", {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      }).format(new Date(seconds * 1000)),
    bytes: (n: number) =>
      n >= 1024 ** 3
        ? (n / 1024 ** 3).toFixed(1) + " GB"
        : n >= 1024 ** 2
          ? (n / 1024 ** 2).toFixed(1) + " MB"
          : Math.ceil(n / 1024) + " KB",
    err: (code: string) => t("error." + code),
  };
}
