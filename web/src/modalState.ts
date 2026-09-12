import { ref } from "vue";

// Native dialogs live in the top layer; status messages must live there too.
export const modalStack = ref<string[]>([]);
