<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  Pin,
  Trash2,
  MoreHorizontal,
  EyeOff,
  Eye,
  Download,
  Copy,
  ExternalLink,
  FileText,
  StickyNote,
  Link,
  ChartNoAxesColumn,
} from "lucide-vue-next";
import type { Content, Room } from "../types";
import { api, assetURL, errorCode, notify } from "../api";
import { useFormat } from "../format";
import Modal from "./Modal.vue";
import ReactionBar from "./ReactionBar.vue";
const actionsOpen = ref(false);
const props = defineProps<{
  content: Content;
  room: Room;
  host: boolean;
  myID: string;
}>();
const emit = defineEmits<{ changed: [] }>();
const { t } = useI18n();
const { date, bytes } = useFormat();
const detail = ref(false),
  deleteConfirm = ref(false),
  busy = ref(false),
  manual = ref(false),
  choices = ref<number[]>([]);
watch(
  () => props.content.my_vote,
  (v) => (choices.value = [...(v || [])]),
  { immediate: true },
);
const canDelete = computed(
  () =>
    props.host ||
    (props.myID === props.content.author_id && props.room.state === "active"),
);
const active = computed(() => props.room.state === "active");
async function update(data: unknown) {
  busy.value = true;
  try {
    await api(
      `/rooms/${props.room.id}/contents/${props.content.id}`,
      "PATCH",
      data,
    );
    emit("changed");
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
async function showPoll() {
  busy.value = true;
  try {
    await api(`/rooms/${props.room.id}/display/control`, "POST", {
      mode: "poll",
      target: props.content.id,
      source: "content",
      expected_version: props.room.version,
    });
    actionsOpen.value = false;
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    emit("changed");
    busy.value = false;
  }
}
async function remove() {
  busy.value = true;
  try {
    await api(`/rooms/${props.room.id}/contents/${props.content.id}`, "DELETE");
    deleteConfirm.value = false;
    detail.value = false;
    emit("changed");
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
async function copy() {
  try {
    await navigator.clipboard.writeText(props.content.body);
    notify(t("app.copied"));
  } catch {
    manual.value = true;
  }
}
function toggle(i: number) {
  if (props.content.poll?.multiple) {
    choices.value = choices.value.includes(i)
      ? choices.value.filter((v) => v !== i)
      : [...choices.value, i];
  } else choices.value = [i];
}
async function vote() {
  busy.value = true;
  try {
    await api(`/rooms/${props.room.id}/polls/${props.content.id}/vote`, "PUT", {
      choices: choices.value,
    });
    emit("changed");
    notify(t("room.voted"));
  } catch (e) {
    notify(t("error." + errorCode(e)));
  } finally {
    busy.value = false;
  }
}
const urlHost = computed(() => {
  try {
    return new URL(props.content.body).hostname;
  } catch {
    return "";
  }
});
</script>
<template>
  <article
    class="content-card"
    :class="[content.kind, { 'is-hidden': !content.visible }]"
  >
    <button
      v-if="content.kind === 'photo'"
      class="photo-open"
      :aria-label="t('room.selectPhoto') + ': ' + content.filename"
      @click="detail = true"
    >
      <img
        :src="assetURL(room.id, content.id)"
        :alt="content.filename"
        loading="lazy"
      /><span v-if="content.selected" class="photo-pin"
        ><Pin :size="14" />{{ t("room.selected") }}</span
      >
    </button>
    <div v-else class="content-body">
      <div class="content-type">
        <component
          :is="
            content.kind === 'file'
              ? FileText
              : content.kind === 'note'
                ? StickyNote
                : content.kind === 'link'
                  ? Link
                  : ChartNoAxesColumn
          "
          :size="19"
        /><span>{{ t("module." + content.kind) }}</span
        ><span v-if="!content.visible" class="tag quiet">{{
          t("room.hidden")
        }}</span>
      </div>
      <h3>{{ content.title || content.filename }}</h3>
      <p v-if="content.kind === 'note'" class="note-body">{{ content.body }}</p>
      <template v-if="content.kind === 'link'"
        ><p class="muted small">{{ urlHost }}</p>
        <a
          :href="content.body"
          target="_blank"
          rel="noopener noreferrer"
          class="external-link"
          >{{ content.body }}<ExternalLink :size="15" /></a
      ></template>
      <p v-if="content.kind === 'file'" class="muted">
        {{ bytes(content.bytes) }}
      </p>
      <template v-if="content.poll"
        ><p class="muted small">
          {{
            content.poll.multiple
              ? t("room.multipleHint", { n: content.poll.max_choices })
              : t("room.singleHint")
          }}
        </p>
        <div class="poll-options">
          <button
            v-for="(option, i) in content.poll.options"
            :key="i"
            :class="{ chosen: choices.includes(i) }"
            :disabled="content.poll.closed || !active || busy"
            @click="toggle(i)"
          >
            <div class="row between">
              <span>{{ choices.includes(i) ? "◉" : "○" }} {{ option }}</span
              ><strong v-if="content.counts">{{ content.counts[i] }}</strong>
            </div>
            <span v-if="content.counts" class="poll-bar"
              ><span
                :style="{
                  width:
                    (content.voters
                      ? (content.counts[i]! / content.voters) * 100
                      : 0) + '%',
                }"
              ></span
            ></span>
          </button>
        </div>
        <div class="row between wrap">
          <small v-if="content.voters !== undefined">{{
            t("room.votes", { n: content.voters })
          }}</small
          ><small v-else>{{ t("room.resultsHidden") }}</small
          ><button
            v-if="!content.poll.closed && active"
            class="primary compact"
            :disabled="!choices.length || busy"
            @click="vote"
          >
            {{
              t(content.my_vote?.length ? "room.updateVote" : "room.vote")
            }}</button
          ><span v-else class="tag quiet">{{ t("room.pollClosed") }}</span>
        </div>
        <button
          v-if="host && !content.poll.closed"
          class="text-button"
          @click="update({ close_poll: true })"
        >
          {{ t("room.closePoll") }}
        </button></template
      >
    </div>
    <footer class="content-footer">
      <span class="avatar">{{ content.author_name.slice(0, 1) }}</span>
      <div class="content-by">
        <span>{{ content.author_name }}</span
        ><small>{{ date(content.created_at) }}</small>
      </div>
      <div class="content-actions">
        <a
          v-if="content.kind === 'file'"
          class="icon-button"
          :href="assetURL(room.id, content.id, 'original')"
          :aria-label="t('app.download')"
          :title="t('app.download')"
          ><Download :size="17" /></a
        ><button
          v-if="content.kind === 'note' || content.kind === 'link'"
          class="icon-button"
          :aria-label="t('app.copy')"
          :title="t('app.copy')"
          @click="copy"
        >
          <Copy :size="16" /></button
        ><button
          v-if="host && content.kind !== 'file'"
          class="icon-button"
          :class="{ accent: content.selected }"
          :aria-label="t(content.selected ? 'room.unselect' : 'room.select')"
          :title="t(content.selected ? 'room.unselect' : 'room.select')"
          :disabled="busy"
          @click="update({ selected: !content.selected })"
        >
          <Pin :size="16" /></button
        ><button
          v-if="host || canDelete"
          class="icon-button"
          :aria-label="t('ui.contentActions')"
          :title="t('ui.contentActions')"
          @click="actionsOpen = true"
        >
          <MoreHorizontal :size="19" />
        </button>
      </div>
    </footer>
    <ReactionBar
      v-if="content.visible"
      :room="room.id"
      :content="content.id"
      :disabled="!!room.settings.reactions_disabled || !active"
    />
  </article>
  <DialogTransition
    ><Modal
      v-if="actionsOpen"
      :title="t('ui.contentActions')"
      :busy="busy"
      @close="actionsOpen = false"
    >
      <div class="content-action-list">
        <button
          v-if="
            host &&
            active &&
            content.kind === 'poll' &&
            content.visible &&
            room.settings.modules.includes('display')
          "
          :disabled="busy"
          @click="showPoll"
        >
          {{ t("displayControl.show") }}
        </button>
        <button
          v-if="host"
          :disabled="busy"
          @click="update({ visible: !content.visible })"
        >
          <EyeOff v-if="content.visible" :size="18" /><Eye
            v-else
            :size="18"
          />{{ t(content.visible ? "room.hide" : "room.show") }}
        </button>
        <button
          v-if="canDelete"
          class="danger-text"
          @click="
            actionsOpen = false;
            deleteConfirm = true;
          "
        >
          <Trash2 :size="18" />{{ t("app.delete") }}
        </button>
      </div>
    </Modal></DialogTransition
  >
  <DialogTransition
    ><Modal v-if="detail" :title="content.filename" wide @close="detail = false"
      ><img
        class="detail-photo"
        :src="assetURL(room.id, content.id, 'preview')"
        :alt="content.filename"
      />
      <div class="row wrap">
        <a
          class="button primary"
          :href="assetURL(room.id, content.id, 'preview') + '?download=1'"
          ><Download :size="17" />{{ t("room.downloadPhoto") }}</a
        ><a
          v-if="host || room.settings.download_original"
          class="button"
          :href="assetURL(room.id, content.id, 'original')"
          >{{ t("room.downloadOriginal") }}</a
        >
      </div></Modal
    ></DialogTransition
  >
  <DialogTransition
    ><Modal
      v-if="deleteConfirm"
      :title="t('room.deleteTitle')"
      :busy="busy"
      @close="deleteConfirm = false"
      ><p class="muted">{{ t("room.deleteHelp") }}</p>
      <div class="modal-actions">
        <button @click="deleteConfirm = false">{{ t("app.cancel") }}</button
        ><button class="danger" :disabled="busy" @click="remove">
          {{ t("app.delete") }}
        </button>
      </div></Modal
    ></DialogTransition
  >
  <DialogTransition
    ><Modal v-if="manual" :title="t('app.copy')" @close="manual = false"
      ><p>{{ t("app.manualCopy") }}</p>
      <textarea
        readonly
        :aria-label="t('room.body')"
        :value="content.body"
        rows="5"
        @focus="(e) => (e.target as HTMLTextAreaElement).select()"
      ></textarea></Modal
  ></DialogTransition>
</template>
