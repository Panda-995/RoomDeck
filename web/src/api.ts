import { ref } from "vue";
export class APIError extends Error {
  constructor(
    public code: string,
    public status: number,
  ) {
    super(code);
  }
}
export const notification = ref("");
let timer: ReturnType<typeof setTimeout>;
export function notify(message: string) {
  notification.value = message;
  clearTimeout(timer);
  timer = setTimeout(() => (notification.value = ""), 5000);
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15_000);
  let status = 0;
  try {
    const response = await fetch("/api/v1" + path, {
      signal: controller.signal,
      method,
      headers: {
        "Content-Type": "application/json",
        "X-RoomDeck-Request": "1",
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    status = response.status;
    const result = await response.json();
    if (!response.ok)
      throw new APIError(
        result.error?.code || "INTERNAL_ERROR",
        response.status,
      );
    return result;
  } catch (error) {
    if (error instanceof APIError) throw error;
    throw new APIError(
      controller.signal.aborted ? "NETWORK_TIMEOUT" : "NETWORK_ERROR",
      status,
    );
  } finally {
    clearTimeout(timeout);
  }
}
export function upload(
  room: string,
  file: File,
  kind: string,
  progress: (percent: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open(
      "POST",
      `/api/v1/rooms/${room}/assets?${new URLSearchParams({ kind, name: file.name, size: String(file.size) })}`,
    );
    xhr.setRequestHeader("X-RoomDeck-Request", "1");
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    xhr.upload.onprogress = (e) =>
      progress(e.lengthComputable ? Math.round((e.loaded / e.total) * 100) : 0);
    xhr.onerror = () => reject(new APIError("NETWORK_ERROR", 0));
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) resolve();
      else {
        let code = "INTERNAL_ERROR";
        try {
          code = JSON.parse(xhr.responseText).error.code;
        } catch {}
        reject(new APIError(code, xhr.status));
      }
    };
    xhr.send(file);
  });
}
export function errorCode(error: unknown) {
  return error instanceof APIError ? error.code : "INTERNAL_ERROR";
}
export function assetURL(
  room: string,
  content: string,
  variant = "thumb",
  display = false,
) {
  return `/api/v1/rooms/${room}/assets/${content}/${variant}${display ? "?display=1" : ""}`;
}
