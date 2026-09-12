import { sha256 } from "@noble/hashes/sha2.js";
self.onmessage = async (event: MessageEvent<File>) => {
  try {
    const hashes: string[] = [];
    for (let offset = 0; offset < event.data.size; offset += 4 * 1024 * 1024) {
      const bytes = new Uint8Array(
        await event.data.slice(offset, offset + 4 * 1024 * 1024).arrayBuffer(),
      );
      hashes.push(
        Array.from(sha256(bytes), (n) => n.toString(16).padStart(2, "0")).join(
          "",
        ),
      );
    }
    self.postMessage({ hashes });
  } catch {
    self.postMessage({ error: "NETWORK_ERROR" });
  }
};
