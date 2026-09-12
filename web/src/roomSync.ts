import { computed, onBeforeUnmount, ref } from "vue";
import { api, errorCode } from "./api";
import type { Snapshot } from "./types";

export function useRoomSync(id: string, display = false) {
  const snapshot = ref<Snapshot>(),
    connected = ref(false),
    error = ref(""),
    lastSync = ref(0);
  const online = ref(navigator.onLine),
    syncing = ref(false),
    terminal = ref(false);
  const state = computed(() =>
    terminal.value
      ? "denied"
      : !online.value
        ? "offline"
        : error.value
          ? "reconnecting"
          : syncing.value && !connected.value
            ? "syncing"
            : connected.value
              ? "connected"
              : "reconnecting",
  );
  let socket: WebSocket | undefined,
    stopped = false,
    started = false,
    attempt = 0;
  let reconnect: ReturnType<typeof setTimeout> | undefined,
    polling: ReturnType<typeof setInterval> | undefined;
  let inflight: Promise<void> | undefined,
    pending = false;
  const suffix = display ? "?display=1" : "";
  function refresh(): Promise<void> {
    if (stopped) return Promise.resolve();
    if (inflight) {
      pending = true;
      return inflight;
    }
    syncing.value = true;
    inflight = (async () => {
      try {
        const next = await api<Snapshot>("/rooms/" + id + "/snapshot" + suffix);
        if (stopped) return;
        if (
          !snapshot.value ||
          (next.room.state_revision || 0) >=
            (snapshot.value.room.state_revision || 0)
        )
          snapshot.value = next;
        lastSync.value = Date.now();
        error.value = "";
      } catch (e) {
        if (stopped) return;
        error.value = errorCode(e);
        if (
          [
            "AUTH_REQUIRED",
            "ROOM_DELETED",
            "ROOM_NOT_FOUND",
            "FORBIDDEN",
            "MODULE_DISABLED",
          ].includes(error.value)
        ) {
          snapshot.value = undefined;
          terminal.value = true;
          try {
            const prefix = `roomdeck:draft:${id}:`;
            for (const key of Object.keys(sessionStorage))
              if (key.startsWith(prefix)) sessionStorage.removeItem(key);
          } catch {}
          stop();
        }
      } finally {
        inflight = undefined;
        syncing.value = false;
        if (pending && !stopped) {
          pending = false;
          void refresh();
        }
      }
    })();
    return inflight;
  }
  function schedule() {
    clearTimeout(reconnect);
    if (!stopped && online.value)
      reconnect = setTimeout(
        connect,
        Math.min(30000, 1000 * 2 ** Math.min(attempt++, 5)) *
          (0.8 + Math.random() * 0.4),
      );
  }
  function connect() {
    clearTimeout(reconnect);
    if (
      stopped ||
      !online.value ||
      (socket && socket.readyState < WebSocket.CLOSING)
    )
      return;
    const current = new WebSocket(
      `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/api/v1/rooms/${id}/ws${suffix}`,
    );
    socket = current;
    current.onopen = () => {
      if (socket !== current || stopped) return;
      connected.value = true;
      void refresh();
    };
    current.onmessage = (event) => {
      if (socket !== current || stopped) return;
      attempt = 0;
      try {
        if (JSON.parse(event.data).type === "interactions") {
          window.dispatchEvent(
            new CustomEvent("roomdeck:interactions", {
              detail: { id, display },
            }),
          );
          return;
        }
      } catch {}
      void refresh();
    };
    current.onclose = () => {
      if (socket !== current) return;
      socket = undefined;
      connected.value = false;
      schedule();
    };
    current.onerror = () => current.close();
  }
  function wake() {
    online.value = navigator.onLine;
    if (stopped || !online.value || document.visibilityState === "hidden")
      return;
    void refresh();
    connect();
  }
  function offline() {
    online.value = false;
    connected.value = false;
    clearTimeout(reconnect);
    socket?.close();
  }
  async function start() {
    if (started || stopped) return;
    started = true;
    window.addEventListener("online", wake);
    window.addEventListener("offline", offline);
    document.addEventListener("visibilitychange", wake);
    await refresh();
    if (stopped) return;
    connect();
    polling = setInterval(() => {
      if (online.value) void refresh();
    }, 20000);
  }
  function stop() {
    stopped = true;
    connected.value = false;
    clearTimeout(reconnect);
    clearInterval(polling);
    window.removeEventListener("online", wake);
    window.removeEventListener("offline", offline);
    document.removeEventListener("visibilitychange", wake);
    socket?.close();
  }
  onBeforeUnmount(stop);
  return {
    snapshot,
    connected: computed(() => connected.value && !error.value && online.value),
    state,
    error,
    lastSync,
    refresh,
    start,
  };
}
