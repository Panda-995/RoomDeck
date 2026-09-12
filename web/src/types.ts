export interface Settings {
  modules: string[];
  join_locked: boolean;
  uploads_paused: boolean;
  download_original: boolean;
  auto_display: boolean;
  quota: number;
  max_members: number;
  display_mode: string;
  display_target: string;
  display_source?: string;
  screen_mode?: string;
  reactions_disabled?: boolean;
  danmaku_mode?: string;
  display_paused: boolean;
  interval: number;
}
export interface Room {
  id: string;
  code: string;
  name: string;
  state: string;
  ends_at: number;
  closed_at: number;
  delete_at: number;
  retention: number;
  created_at: number;
  settings: Settings;
  version: number;
  used: number;
  reserved: number;
  state_revision?: number;
}
export interface Poll {
  options: string[];
  multiple: boolean;
  max_choices: number;
  hide_results: boolean;
  closed: boolean;
  closes_at: number;
}
export interface Content {
  id: string;
  room_id: string;
  author_id: string;
  author_name: string;
  kind: string;
  title: string;
  body: string;
  filename: string;
  mime: string;
  bytes: number;
  visible: boolean;
  selected: boolean;
  created_at: number;
  poll?: Poll;
  counts?: number[];
  voters?: number;
  my_vote?: number[];
}
export interface Member {
  permissions?: string[];
  id: string;
  name: string;
  revoked: boolean;
  muted: boolean;
  online: boolean;
}
export interface Job {
  id: string;
  state: string;
  created_at: number;
  error: string;
  bytes: number;
}
export interface Snapshot {
  permissions?: string[];
  room: Room;
  contents: Content[];
  role: "host" | "guest" | "display";
  participant_id: string;
  name: string;
  online: number;
  participants_total: number;
  members: Member[];
  invite?: string;
  jobs?: Job[];
  display_online: number;
}
export interface Status {
  setup_required: boolean;
  authenticated: boolean;
  username: string;
  base_url: string;
}
