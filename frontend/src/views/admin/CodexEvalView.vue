<template>
  <AppLayout>
    <div class="card overflow-hidden">
      <iframe
        :src="embeddedUrl"
        :title="locale.toLowerCase().startsWith('zh') ? '模型哨兵' : 'Model Guard'"
        class="block w-full border-0"
        style="height: calc(100dvh - 160px); min-height: 600px"
      ></iframe>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { buildEmbeddedUrl, detectTheme } from '@/utils/embedded-url'

const { locale } = useI18n()
const pageTheme = ref<'light' | 'dark'>(detectTheme())
// Keep the frame on this origin: it reads the auth store's localStorage auth_token
// for admin API requests. Never disclose the bearer token in an iframe URL.
const embeddedUrl = computed(() => buildEmbeddedUrl(
  new URL('/codex-eval', window.location.origin).toString(),
  undefined,
  pageTheme.value,
  locale.value,
))
let themeObserver: MutationObserver | null = null
onMounted(() => {
  themeObserver = new MutationObserver(() => { pageTheme.value = detectTheme() })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})
onUnmounted(() => { themeObserver?.disconnect() })
</script>
