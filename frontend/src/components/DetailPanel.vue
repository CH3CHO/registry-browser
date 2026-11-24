<script setup lang="ts">
import { computed, ref } from 'vue'
import type { TagDetail } from '../../bindings/github.com/ch3cho/registry-browser/internal/app/models'

const props = defineProps<{
  host: string
  repository: string
  tag: string
  detail: TagDetail | null
  loading: boolean
  error: string
}>()

const copied = ref(false)

const manifest = computed(() => props.detail?.manifest ?? null)
const imageConfig = computed(() => props.detail?.config ?? null)

const pullCommand = computed(() => {
  if (!props.host || !props.repository || !props.tag) {
    return ''
  }
  return `docker pull ${props.host}/${props.repository}:${props.tag}`
})

const rawJson = computed(() => {
  const raw = manifest.value?.raw
  if (!raw) {
    return ''
  }
  return typeof raw === 'string' ? raw : JSON.stringify(raw, null, 2)
})

const totalLayerSize = computed(() => {
  return (manifest.value?.layers ?? []).reduce((sum, layer) => sum + (layer.size ?? 0), 0)
})

function formatSize(bytes: number | undefined): string {
  if (!bytes) {
    return '—'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

async function copyPullCommand() {
  if (!pullCommand.value) {
    return
  }
  try {
    await navigator.clipboard.writeText(pullCommand.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    copied.value = false
  }
}
</script>

<template>
  <section class="detail" aria-label="Tag 详情">
    <header class="detail__head">
      <div class="detail__title">
        <h2 class="detail__heading">
          {{ props.tag || '未选择 tag' }}
          <span v-if="props.loading" class="detail__spinner">加载中…</span>
        </h2>
        <p v-if="manifest" class="detail__digest" :title="manifest?.digest">
          {{ manifest?.digest }}
        </p>
      </div>
      <button
        v-if="pullCommand"
        type="button"
        class="detail__copy"
        @click="copyPullCommand"
      >
        {{ copied ? '已复制' : '复制 pull 命令' }}
      </button>
    </header>

    <div class="detail__body">
      <p v-if="props.error" class="detail__note detail__note--error">{{ props.error }}</p>
      <p v-else-if="!props.detail && props.loading" class="detail__note">正在读取清单…</p>
      <p v-else-if="!props.detail" class="detail__note">选择一个 tag 查看详情。</p>

      <template v-else>
        <section class="block">
          <h3 class="block__title">概要</h3>
          <dl class="facts">
            <div class="facts__item">
              <dt>媒体类型</dt>
              <dd class="mono">{{ manifest?.mediaType }}</dd>
            </div>
            <div class="facts__item">
              <dt>清单大小</dt>
              <dd>{{ formatSize(manifest?.size) }}</dd>
            </div>
            <div class="facts__item">
              <dt>层合计</dt>
              <dd>{{ formatSize(totalLayerSize) }}</dd>
            </div>
            <div v-if="imageConfig" class="facts__item">
              <dt>平台</dt>
              <dd class="mono">
                {{ imageConfig?.os }}/{{ imageConfig?.architecture
                }}{{ imageConfig?.variant ? '/' + imageConfig?.variant : '' }}
              </dd>
            </div>
            <div v-if="imageConfig" class="facts__item">
              <dt>构建时间</dt>
              <dd>
                {{ imageConfig?.created }}
                <span class="hint">来自镜像配置，非推送时间</span>
              </dd>
            </div>
          </dl>
        </section>

        <section v-if="manifest?.platforms ?? []?.length" class="block">
          <h3 class="block__title">多架构</h3>
          <table class="grid">
            <thead>
              <tr>
                <th scope="col">平台</th>
                <th scope="col">摘要</th>
                <th scope="col">大小</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="platform in manifest?.platforms ?? []" :key="platform.digest">
                <td class="mono">{{ platform.platform || '—' }}</td>
                <td class="mono dim">{{ platform.digest }}</td>
                <td class="num">{{ formatSize(platform.size) }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section v-if="manifest?.layers ?? []?.length" class="block">
          <h3 class="block__title">镜像层</h3>
          <table class="grid">
            <thead>
              <tr>
                <th scope="col">#</th>
                <th scope="col">媒体类型</th>
                <th scope="col">摘要</th>
                <th scope="col">大小</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(layer, index) in manifest?.layers ?? []" :key="layer.digest">
                <td class="num">{{ index + 1 }}</td>
                <td class="mono dim">{{ layer.mediaType }}</td>
                <td class="mono dim">{{ layer.digest }}</td>
                <td class="num">{{ formatSize(layer.size) }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section v-if="imageConfig?.labels && Object.keys(imageConfig?.labels).length" class="block">
          <h3 class="block__title">Labels</h3>
          <dl class="facts facts--pairs">
            <div v-for="(value, key) in imageConfig?.labels" :key="key" class="facts__item">
              <dt class="mono">{{ key }}</dt>
              <dd class="mono">{{ value }}</dd>
            </div>
          </dl>
        </section>

        <section v-if="imageConfig?.env?.length" class="block">
          <h3 class="block__title">环境变量</h3>
          <ul class="plain-list">
            <li v-for="entry in imageConfig?.env" :key="entry" class="mono">{{ entry }}</li>
          </ul>
        </section>

        <section v-if="imageConfig?.history?.length" class="block">
          <h3 class="block__title">构建历史</h3>
          <ol class="history">
            <li v-for="(entry, index) in imageConfig?.history" :key="index" class="history__item">
              <span class="history__index">{{ index + 1 }}</span>
              <span class="history__command mono">{{ entry.createdBy || entry.comment || '—' }}</span>
              <span v-if="entry.emptyLayer" class="badge">无层</span>
            </li>
          </ol>
        </section>

        <section v-if="rawJson" class="block">
          <details class="raw">
            <summary class="raw__summary">原始清单 JSON</summary>
            <pre class="raw__code">{{ rawJson }}</pre>
          </details>
        </section>
      </template>
    </div>
  </section>
</template>

<style scoped>
.detail {
  display: flex;
  flex-direction: column;
  min-width: 0;
  height: 100%;
  background: var(--surface-raised);
}

.detail__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--border-hairline);
}

.detail__heading {
  margin: 0;
  font-family: var(--font-mono);
  font-size: var(--text-lg);
  font-weight: var(--weight-medium);
}

.detail__digest {
  margin: var(--space-1) 0 0;
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  word-break: break-all;
}

.detail__spinner {
  margin-left: var(--space-2);
  font-size: var(--text-xs);
  font-weight: var(--weight-regular);
  letter-spacing: 0;
  text-transform: none;
  color: var(--text-tertiary);
}

.detail__copy {
  flex-shrink: 0;
  padding: var(--space-2) var(--space-4);
  background: var(--surface-page);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.detail__copy:hover {
  background: var(--surface-hover);
}

.detail__body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-5);
}

.detail__note {
  margin: 0;
  color: var(--text-tertiary);
  font-size: var(--text-sm);
}

.detail__note--error {
  color: var(--status-error);
}

.block + .block {
  margin-top: var(--space-6);
}

.block__title {
  margin: 0 0 var(--space-3);
  padding-bottom: var(--space-2);
  border-bottom: 1px solid var(--border-hairline);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-tertiary);
}

.facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3) var(--space-5);
  margin: 0;
}

.facts--pairs {
  grid-template-columns: 1fr;
  gap: var(--space-2);
}

.facts__item {
  min-width: 0;
}

.facts__item dt {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
}

.facts__item dd {
  margin: 0;
  font-size: var(--text-sm);
  color: var(--text-primary);
  word-break: break-all;
}

.hint {
  display: block;
  font-size: var(--text-xs);
  color: var(--text-tertiary);
}

.grid {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

.grid th {
  padding: var(--space-2) var(--space-3) var(--space-2) 0;
  border-bottom: 1px solid var(--border-hairline);
  text-align: left;
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-tertiary);
}

.grid td {
  padding: var(--space-2) var(--space-3) var(--space-2) 0;
  border-bottom: 1px solid var(--border-hairline);
  color: var(--text-secondary);
}

.grid .num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.mono {
  font-family: var(--font-mono);
}

.dim {
  color: var(--text-tertiary);
  word-break: break-all;
}

.plain-list {
  margin: 0;
  padding-left: var(--space-5);
  font-size: var(--text-sm);
  color: var(--text-secondary);
}

.history {
  margin: 0;
  padding: 0;
  list-style: none;
}

.history__item {
  display: flex;
  gap: var(--space-3);
  align-items: baseline;
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--border-hairline);
  font-size: var(--text-sm);
}

.history__index {
  flex-shrink: 0;
  width: 1.5em;
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.history__command {
  flex: 1;
  min-width: 0;
  color: var(--text-secondary);
  word-break: break-all;
}

.badge {
  flex-shrink: 0;
  padding: 0 var(--space-2);
  background: var(--status-warn-soft);
  color: var(--status-warn);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
}

.raw__summary {
  cursor: pointer;
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-tertiary);
}

.raw__code {
  margin: var(--space-3) 0 0;
  padding: var(--space-4);
  max-height: 320px;
  overflow: auto;
  background: var(--surface-sunken);
  border: 1px solid var(--border-hairline);
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  line-height: var(--leading-normal);
  color: var(--text-secondary);
}
</style>
