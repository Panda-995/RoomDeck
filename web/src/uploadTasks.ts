import { reactive } from "vue";
import { api, APIError, errorCode } from "./api";
export interface UploadRecord {
  id: string;
  filename: string;
  kind: string;
  bytes: number;
  hashes: string[];
  parts: number[];
  state: string;
  request_id: string;
  chunk_size: number;
}
export interface UploadTask {
  key: string;
  room: string;
  identity: string;
  filename: string;
  kind: string;
  bytes: number;
  file?: File;
  record?: UploadRecord;
  percent: number;
  state: string;
  error: string;
  running: boolean;
  retries: number;
  xhr?: XMLHttpRequest;
}
const tasks = reactive<UploadTask[]>([]);
let active = 0;
export function roomUploads(room: string, identity: string) {
  return tasks.filter((t) => t.room === room && t.identity === identity);
}
export async function restoreUploads(room: string, identity: string) {
  const records = await api<UploadRecord[]>(`/rooms/${room}/uploads`);
  for (const record of records) {
    if (
      record.state === "ready" ||
      tasks.some((t) => t.record?.id === record.id)
    )
      continue;
    tasks.push({
      key: record.id,
      room,
      identity,
      filename: record.filename,
      kind: record.kind,
      bytes: record.bytes,
      record,
      percent: Math.floor((record.parts.length / record.hashes.length) * 100),
      state: "reselect",
      error: "",
      running: false,
      retries: 0,
    });
  }
}
function hashFile(file: File): Promise<string[]> {
  return new Promise((resolve, reject) => {
    const worker = new Worker(
      new URL("./uploadHash.worker.ts", import.meta.url),
      { type: "module" },
    );
    worker.onmessage = (event) => {
      worker.terminate();
      event.data.error
        ? reject(new APIError(event.data.error, 0))
        : resolve(event.data.hashes);
    };
    worker.onerror = () => {
      worker.terminate();
      reject(new APIError("INTERNAL_ERROR", 0));
    };
    worker.postMessage(file);
  });
}
export function addUpload(
  room: string,
  identity: string,
  file: File,
  kind: string,
) {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  tasks.push({
    key: Array.from(bytes, (n) => n.toString(16).padStart(2, "0")).join(""),
    room,
    identity,
    file,
    filename: file.name,
    kind,
    bytes: file.size,
    percent: 0,
    state: "queued",
    error: "",
    running: false,
    retries: 0,
  });
  pump();
}
function chunk(task: UploadTask, index: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const r = task.record!;
    const xhr = new XMLHttpRequest();
    task.xhr = xhr;
    xhr.open(
      "PUT",
      `/api/v1/rooms/${task.room}/uploads/${r.id}/parts/${index}`,
    );
    xhr.setRequestHeader("X-RoomDeck-Request", "1");
    xhr.timeout = 60000;
    xhr.upload.onprogress = (e) => {
      task.percent = Math.min(
        99,
        Math.floor(
          ((r.parts.length * r.chunk_size + e.loaded) / task.bytes) * 100,
        ),
      );
    };
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
    xhr.onerror = () => reject(new APIError("NETWORK_ERROR", 0));
    xhr.ontimeout = () => reject(new APIError("NETWORK_TIMEOUT", 0));
    xhr.onabort = () => reject(new APIError("UPLOAD_PAUSED", 0));
    xhr.send(
      task.file!.slice(
        index * r.chunk_size,
        Math.min(task.bytes, (index + 1) * r.chunk_size),
      ),
    );
  });
}
async function run(task: UploadTask) {
  if (!task.file || task.running) return;
  task.running = true;
  active++;
  task.error = "";
  try {
    task.state = "preparing";
    const hashes = await hashFile(task.file);
    if (task.state !== "preparing") return;
    if (
      task.record &&
      (task.bytes !== task.file.size ||
        JSON.stringify(task.record.hashes) !== JSON.stringify(hashes))
    )
      throw new APIError("UPLOAD_CONFLICT", 409);
    if (!task.record)
      task.record = await api<UploadRecord>(
        `/rooms/${task.room}/uploads`,
        "POST",
        {
          request_id: task.key,
          filename: task.filename,
          kind: task.kind,
          bytes: task.bytes,
          hashes,
        },
      );
    else
      task.record = await api<UploadRecord>(
        `/rooms/${task.room}/uploads/${task.record.id}`,
      );
    if (task.state !== "preparing") return;
    if (task.record.state === "ready") {
      task.state = "ready";
      task.percent = 100;
      task.file = undefined;
      return;
    }
    task.state = "uploading";
    for (let i = 0; i < hashes.length; i++) {
      if (task.state !== "uploading") return;
      if (task.record.parts.includes(i)) continue;
      await chunk(task, i);
      task.record.parts.push(i);
    }
    if (task.state !== "uploading") return;
    task.percent = 100;
    task.state = "processing";
    await api(`/rooms/${task.room}/uploads/${task.record.id}/complete`, "POST");
    task.state = "ready";
    task.file = undefined;
  } catch (error) {
    if (task.state === "paused") return;
    task.error = errorCode(error);
    const retry = ["NETWORK_ERROR", "NETWORK_TIMEOUT"].includes(task.error);
    task.state = retry ? "waiting" : "failed";
    if (retry && task.retries++ < 5)
      setTimeout(
        () => {
          if (task.state === "waiting" && navigator.onLine) {
            task.state = "queued";
            pump();
          }
        },
        Math.min(30000, 1000 * 2 ** task.retries),
      );
  } finally {
    task.xhr = undefined;
    task.running = false;
    active--;
    pump();
  }
}
function pump() {
  for (const task of tasks) {
    if (active >= 2) break;
    if (task.state === "queued" && !task.running) void run(task);
  }
}
export function pauseUpload(task: UploadTask) {
  if (task.state === "processing") return;
  task.state = "paused";
  task.xhr?.abort();
}
export function resumeUpload(task: UploadTask, file?: File) {
  if (task.running) return;
  if (file) task.file = file;
  if (!task.file) {
    task.state = "reselect";
    return;
  }
  task.state = "queued";
  task.retries = 0;
  pump();
}
export async function cancelUpload(task: UploadTask) {
  if (task.running) {
    pauseUpload(task);
    return;
  }
  if (task.record)
    await api(`/rooms/${task.room}/uploads/${task.record.id}`, "DELETE");
  const i = tasks.indexOf(task);
  if (i >= 0) tasks.splice(i, 1);
}
window.addEventListener("online", () => {
  for (const t of tasks)
    if (t.state === "waiting") {
      t.state = "queued";
      t.retries = 0;
    }
  pump();
});
