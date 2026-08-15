<script setup lang="ts">
import { Spinner } from '@/components/shared'
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNotesStore } from '@/stores/notes'
import { useAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Textarea } from '@/components/ui/textarea'
import { toast } from 'vue-sonner'
import { useInfiniteScroll } from '@/composables/useInfiniteScroll'
import { getInitials, getAvatarGradient } from '@/lib/utils'
import {
  StickyNote,
  Pencil,
  Trash2,
  X,
  Check,
  Send
} from '@lucide/vue'

const props = defineProps<{
  contactId: string
}>()

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const notesStore = useNotesStore()
const authStore = useAuthStore()

const newNoteContent = ref('')
const editingNoteId = ref<string | null>(null)
const editingContent = ref('')
const isSaving = ref(false)
const notesEndRef = ref<HTMLElement | null>(null)

// Infinite scroll for older notes (scroll up to load more)
const notesScroll = useInfiniteScroll({
  direction: 'top',
  onLoadMore: async () => {
    await notesScroll.preserveScrollPosition(async () => {
      await notesStore.fetchOlderNotes(props.contactId)
      await nextTick()
    })
  },
  hasMore: computed(() => notesStore.hasMore),
  isLoading: computed(() => notesStore.isLoadingOlder)
})

function scrollToBottom(instant = false) {
  nextTick(() => {
    if (notesEndRef.value) {
      notesEndRef.value.scrollIntoView({
        behavior: instant ? 'instant' : 'smooth',
        block: 'end'
      })
    }
  })
}

onMounted(async () => {
  if (props.contactId) {
    // Only fetch if not already loaded for this contact (ChatView pre-fetches for badge count)
    if (notesStore.currentContactId !== props.contactId) {
      await notesStore.fetchNotes(props.contactId)
    }
    await nextTick()
    // Delay setup like messages do to ensure ScrollArea is fully rendered
    setTimeout(() => {
      scrollToBottom(true)
      notesScroll.setup()
    }, 50)
  }
})

watch(() => props.contactId, async (newId) => {
  if (newId) {
    notesScroll.cleanup()
    if (notesStore.currentContactId !== newId) {
      await notesStore.fetchNotes(newId)
    }
    await nextTick()
    setTimeout(() => {
      scrollToBottom(true)
      notesScroll.setup()
    }, 50)
  }
})

// Auto-scroll when new notes are added at the bottom
watch(() => notesStore.notes.length, (_newLen, oldLen) => {
  if (oldLen !== undefined && _newLen > oldLen) {
    scrollToBottom()
  }
})

async function addNote() {
  if (!newNoteContent.value.trim()) return
  isSaving.value = true
  try {
    await notesStore.createNote(props.contactId, newNoteContent.value.trim())
    newNoteContent.value = ''
    toast.success(t('chat.noteAdded'))
    scrollToBottom()
  } catch {
    toast.error(t('chat.noteAddFailed'))
  } finally {
    isSaving.value = false
  }
}

function startEditing(noteId: string, content: string) {
  editingNoteId.value = noteId
  editingContent.value = content
}

function cancelEditing() {
  editingNoteId.value = null
  editingContent.value = ''
}

async function saveEdit(noteId: string) {
  if (!editingContent.value.trim()) return
  isSaving.value = true
  try {
    await notesStore.updateNote(props.contactId, noteId, editingContent.value.trim())
    editingNoteId.value = null
    editingContent.value = ''
    toast.success(t('chat.noteUpdated'))
  } catch {
    toast.error(t('chat.noteUpdateFailed'))
  } finally {
    isSaving.value = false
  }
}

async function deleteNote(noteId: string) {
  if (!confirm(t('chat.confirmDeleteNote'))) return
  try {
    await notesStore.deleteNote(props.contactId, noteId)
    toast.success(t('chat.noteDeleted'))
  } catch {
    toast.error(t('chat.noteDeleteFailed'))
  }
}

function formatNoteTime(dateStr: string) {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMs / 3600000)
  const diffDays = Math.floor(diffMs / 86400000)

  if (diffMins < 1) return 'Just now'
  if (diffMins < 60) return `${diffMins}m ago`
  if (diffHours < 24) return `${diffHours}h ago`
  if (diffDays < 7) return `${diffDays}d ago`
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}
</script>

<template>
  <div id="notes-panel" class="w-80 border-l border-border bg-white dark:bg-[#111113] flex flex-col">
    <!-- Header -->
    <div class="px-4 py-3 border-b border-border flex items-center justify-between">
      <div class="flex items-center gap-2">
        <div class="size-7 rounded-lg bg-amber-500/15 flex items-center justify-center">
          <StickyNote class="size-4 text-warning" />
        </div>
        <span class="font-semibold text-foreground">{{ t('chat.internalNotes') }}</span>
        <Badge v-if="notesStore.notes.length > 0" class="bg-warning/20 text-warning border-0 px-1.5 py-0">
          {{ notesStore.notes.length }}
        </Badge>
      </div>
      <Button
        variant="ghost"
        size="icon"
        class="size-7 text-foreground/40 hover:text-foreground hover:bg-accent"
        @click="emit('close')"
      >
        <X class="size-4" />
      </Button>
    </div>

    <!-- Notes list -->
    <ScrollArea :ref="(el: any) => notesScroll.scrollAreaRef.value = el" class="flex-1 p-3">
      <div class="space-y-3">
        <!-- Loading older notes -->
        <div v-if="notesStore.isLoadingOlder" class="flex justify-center py-2">
          <Spinner class="size-4 text-foreground/30" />
        </div>

        <!-- Initial loading state -->
        <div v-if="notesStore.isLoading" class="flex justify-center py-8">
          <Spinner class="size-5 text-foreground/30" />
        </div>

        <!-- Notes (chronological: oldest first, latest last) -->
        <template v-else-if="notesStore.notes.length > 0">
          <div
            v-for="note in notesStore.notes"
            :key="note.id"
            data-testid="note-card"
            class="group relative rounded-xl p-3 backdrop-blur-sm border border-border bg-linear-to-br from-gray-50 to-white dark:from-white/4 dark:to-white/2 hover:from-gray-100 hover:to-gray-50 dark:hover:from-white/6 dark:hover:to-white/3 transition-colors duration-200"
          >
            <!-- Gradient accent line -->
            <div class="absolute top-0 left-3 right-3 h-[2px] rounded-full bg-linear-to-r from-amber-500/60 via-orange-500/40 to-transparent" />

            <!-- Editing mode -->
            <template v-if="editingNoteId === note.id">
              <Textarea
                v-model="editingContent"
                class="min-h-[60px] max-h-[100px] resize-none bg-muted border-warning/20 mt-1"
                :rows="2"
                @keydown.meta.enter.prevent="saveEdit(note.id)"
                @keydown.ctrl.enter.prevent="saveEdit(note.id)"
              />
              <div class="flex justify-end gap-1.5 mt-2">
                <Button variant="ghost" size="sm" class="h-7" @click="cancelEditing">
                  {{ t('common.cancel') }}
                </Button>
                <Button
                  size="sm"
                  class="h-7 bg-amber-600 hover:bg-amber-500 text-white"
                  :disabled="!editingContent.trim() || isSaving"
                  @click="saveEdit(note.id)"
                >
                  <Spinner v-if="isSaving" class="size-3 mr-1" />
                  <Check v-else class="size-3 mr-1" />
                  {{ t('common.save') }}
                </Button>
              </div>
            </template>

            <!-- Display mode -->
            <template v-else>
              <div class="flex items-start gap-2.5 mt-1">
                <Avatar class="size-6 ring-1 ring-border">
                  <AvatarFallback :class="'bg-linear-to-br text-white ' + getAvatarGradient(note.created_by_name)">
                    {{ getInitials(note.created_by_name) }}
                  </AvatarFallback>
                </Avatar>
                <div class="flex-1 min-w-0">
                  <div class="flex items-center justify-between mb-1">
                    <span class="font-medium text-foreground/70">{{ note.created_by_name }}</span>
                    <div class="flex items-center gap-1">
                      <!-- Hover actions (own notes only) -->
                      <div
                        v-if="note.created_by_id === authStore.user?.id"
                        class="opacity-0 group-hover:opacity-100 flex gap-0.5"
                      >
                        <button
                          data-testid="note-edit"
                          class="size-5 rounded-md flex items-center justify-center hover:bg-accent text-foreground/30 hover:text-foreground/60 transition-colors"
                          @click="startEditing(note.id, note.content)"
                        >
                          <Pencil class="size-3" />
                        </button>
                        <button
                          data-testid="note-delete"
                          class="size-5 rounded-md flex items-center justify-center hover:bg-red-500/10 text-foreground/30 hover:text-destructive transition-colors"
                          @click="deleteNote(note.id)"
                        >
                          <Trash2 class="size-3" />
                        </button>
                      </div>
                      <span class="text-foreground/30">{{ formatNoteTime(note.created_at) }}</span>
                    </div>
                  </div>
                  <p class="text-foreground/60 leading-relaxed whitespace-pre-wrap wrap-break-word">{{ note.content }}</p>
                </div>
              </div>
            </template>
          </div>
        </template>

        <!-- Empty state -->
        <div v-else class="flex flex-col items-center justify-center py-12 text-center">
          <div class="size-12 rounded-xl bg-warning/10 flex items-center justify-center mb-3">
            <StickyNote class="size-6 text-warning/50" />
          </div>
          <p class="font-medium text-foreground/40 mb-1">{{ t('chat.noNotes') }}</p>
          <p class="text-foreground/25">{{ t('chat.writeNote') }}</p>
        </div>

        <!-- Scroll anchor -->
        <div ref="notesEndRef" />
      </div>
    </ScrollArea>

    <!-- Add note input -->
    <div class="p-4 border-t border-border">
      <div class="flex items-center gap-2 p-2 rounded-xl bg-muted border border-border">
        <textarea
          v-model="newNoteContent"
          :placeholder="t('chat.writeNote') + '...'"
          class="flex-1 bg-transparent text-foreground placeholder:text-foreground/30 focus:outline-hidden resize-none min-h-[36px] max-h-[120px] py-2 overflow-y-auto"
          rows="1"
          @keydown.enter.exact.prevent="addNote"
        />
        <button
          class="size-9 rounded-lg bg-warning hover:bg-warning flex items-center justify-center transition-colors disabled:opacity-50"
          :disabled="!newNoteContent.trim() || isSaving"
          @click="addNote"
        >
          <Spinner v-if="isSaving" class="size-4 text-white" />
          <Send v-else class="size-4 text-white" />
        </button>
      </div>
    </div>
  </div>
</template>
