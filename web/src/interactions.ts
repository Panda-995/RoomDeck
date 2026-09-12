import { reactive, onMounted, onBeforeUnmount } from "vue";
import { api } from "./api";
export interface Reaction {
  content: string;
  emoji: string;
  count: number;
  mine: number;
}
export interface Message {
  id: string;
  name: string;
  body: string;
  state: string;
  expires_at: number;
}
interface Interactions {
  reactions: Reaction[];
  messages: Message[];
}
const stores = new Map<string, Interactions>();
const displayHistory = new Map<string, Map<string, number>>();
export function seenMessages(room: string) {
  let seen=displayHistory.get(room);
  if(!seen){seen=new Map();displayHistory.set(room,seen);}
  const now=Date.now();
  for(const [id,expires] of seen) if(expires<now) seen.delete(id);
  return seen;
}
export function interactionStore(id: string, display = false) {
  const key = id + ":" + display;
  let store = stores.get(key);
  if (!store) {
    store = reactive({ reactions: [], messages: [] });
    stores.set(key, store);
  }
  return store;
}
export function useInteractions(id: string, display = false) {
  const store = interactionStore(id, display);
  let disposed = false,
    inflight = false,
    pending = false,
    timer: ReturnType<typeof setInterval> | undefined;
  async function refresh() {
    if (disposed) return;
    if (inflight) {
      pending = true;
      return;
    }
    inflight = true;
    try {
      const data = await api<Interactions>(
        `/rooms/${id}/interactions${display ? "?display=1" : ""}`,
      );
      if (!disposed) {
        store.reactions = data.reactions;
        store.messages = data.messages;
      }
    } catch {
      if (!disposed)
        store.messages = store.messages.filter(
          (m) => m.expires_at > Date.now() / 1000,
        );
    } finally {
      inflight = false;
      if (pending && !disposed) {
        pending = false;
        void refresh();
      }
    }
  }
  function invalidated(event: Event) {
    const detail = (event as CustomEvent).detail;
    if (detail.id === id && detail.display === display) void refresh();
  }
  function wake() {
    if (document.visibilityState === "visible") void refresh();
  }
  onMounted(() => {
    void refresh();
    timer = setInterval(refresh, 20000);
    window.addEventListener("roomdeck:interactions", invalidated);
    window.addEventListener("online", wake);
    document.addEventListener("visibilitychange", wake);
  });
  onBeforeUnmount(() => {
    disposed = true;
    clearInterval(timer);
    window.removeEventListener("roomdeck:interactions", invalidated);
    window.removeEventListener("online", wake);
    document.removeEventListener("visibilitychange", wake);
    store.messages = [];
    store.reactions = [];
  });
  return { store, refresh };
}
