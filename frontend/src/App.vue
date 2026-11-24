<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AddressBar from './components/AddressBar.vue'
import DetailPanel from './components/DetailPanel.vue'
import RegistryTree from './components/RegistryTree.vue'
import TagTable from './components/TagTable.vue'
import {
  GetTagDetail,
  ListNamespaces,
  ListRegistries,
  ListTags,
  ParseReference,
} from '../bindings/github.com/ch3cho/registry-browser/internal/app/service'
import type { RegistryInfo } from '../bindings/github.com/ch3cho/registry-browser/internal/app/models'
import type { NamespaceEntry } from '../bindings/github.com/ch3cho/registry-browser/internal/app/models'
import type { TagInfo } from '../bindings/github.com/ch3cho/registry-browser/internal/app/models'
import type { TagDetail } from '../bindings/github.com/ch3cho/registry-browser/internal/app/models'

const registries = ref<RegistryInfo[]>([])
const activeRegistry = ref('')
const namespaces = ref<NamespaceEntry[]>([])
const activeRepository = ref('')
const tags = ref<TagInfo[]>([])
const activeTag = ref('')
const detail = ref<TagDetail | null>(null)

const address = ref('')
// 三个面板各自独立的加载状态：点 tag 时不应该让目录树与 tag 列表跟着闪。
const querying = ref(false)
const treeLoading = ref(false)
const tagsLoading = ref(false)
const detailLoading = ref(false)
const treeError = ref('')
const tagError = ref('')
const detailError = ref('')

/** 把后端返回的错误字符串翻译成用户能看懂的一句话。 */
function explain(raw: unknown): string {
  const message = String(raw).replace(/^RuntimeError:\s*/, '')
  const kind = /failed:\s*([a-z_]+)/.exec(message)?.[1]

  switch (kind) {
    case 'unauthorized':
      return '未登录或凭据已失效，请先执行 docker login'
    case 'forbidden':
      return '没有访问权限，请确认账号对该仓库的权限'
    case 'not_found':
      return '仓库或 tag 不存在'
    case 'unsupported':
      return '该 registry 不支持目录列举，请改用上方地址查询'
    case 'rate_limited':
      return '被 registry 限流，请稍后重试'
    case 'server_error':
      return 'registry 返回服务端错误'
    case 'network':
      return '网络不可达，请检查地址与连通性'
    case 'invalid_response':
      return 'registry 返回了无法解析的内容'
    default:
      return message
  }
}

async function loadRegistries() {
  try {
    registries.value = (await ListRegistries()) ?? []
    if (registries.value.length) {
      await selectRegistry(registries.value[0].host)
    }
  } catch (error) {
    treeError.value = explain(error)
  }
}

async function selectRegistry(host: string) {
  activeRegistry.value = host
  activeRepository.value = ''
  tags.value = []
  activeTag.value = ''
  detail.value = null
  treeError.value = ''

  treeLoading.value = true
  try {
    namespaces.value = (await ListNamespaces(host)) ?? []
  } catch (error) {
    namespaces.value = []
    treeError.value = explain(error)
  } finally {
    treeLoading.value = false
  }
}

async function selectImage(repository: string) {
  activeRepository.value = repository
  activeTag.value = ''
  detail.value = null
  tagError.value = ''

  tagsLoading.value = true
  try {
    tags.value = (await ListTags(activeRegistry.value, repository)) ?? []
  } catch (error) {
    tags.value = []
    tagError.value = explain(error)
  } finally {
    tagsLoading.value = false
  }
}

async function selectTag(tag: string) {
  activeTag.value = tag
  detailError.value = ''

  detailLoading.value = true
  try {
    detail.value = (await GetTagDetail(activeRegistry.value, activeRepository.value, tag)) ?? null
  } catch (error) {
    detail.value = null
    detailError.value = explain(error)
  } finally {
    detailLoading.value = false
  }
}

async function queryAddress() {
  const input = address.value.trim()
  if (!input) {
    return
  }

  querying.value = true
  try {
    const reference = await ParseReference(input)
    if (reference.registry !== activeRegistry.value) {
      await selectRegistry(reference.registry)
    }
    await selectImage(reference.repository)
    if (reference.tag) {
      await selectTag(reference.tag)
    }
  } catch (error) {
    tagError.value = explain(error)
  } finally {
    querying.value = false
  }
}

onMounted(loadRegistries)
</script>

<template>
  <div class="shell">
    <header class="shell__bar">
      <div class="shell__brand">
        <span class="shell__mark">▣</span>
        <span class="shell__name">Registry Browser</span>
      </div>
      <AddressBar v-model="address" :busy="querying" @submit="queryAddress" />
    </header>

    <main class="shell__panes">
      <RegistryTree
        class="shell__pane shell__pane--tree"
        :registries="registries"
        :active-registry="activeRegistry"
        :namespaces="namespaces"
        :active-repository="activeRepository"
        :loading="treeLoading"
        :error="treeError"
        @select-registry="selectRegistry"
        @select-image="selectImage"
      />
      <TagTable
        class="shell__pane shell__pane--tags"
        :repository="activeRepository"
        :tags="tags"
        :active-tag="activeTag"
        :loading="tagsLoading"
        :error="tagError"
        @select-tag="selectTag"
      />
      <DetailPanel
        class="shell__pane shell__pane--detail"
        :host="activeRegistry"
        :repository="activeRepository"
        :tag="activeTag"
        :detail="detail"
        :loading="detailLoading"
        :error="detailError"
      />
    </main>
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.shell__bar {
  display: flex;
  align-items: flex-end;
  gap: var(--space-6);
  padding: var(--space-4) var(--space-5);
  background: var(--surface-raised);
  border-bottom: 1px solid var(--border-hairline);
  box-shadow: var(--shadow-panel);
}

.shell__brand {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  flex-shrink: 0;
}

.shell__mark {
  color: var(--accent);
  font-size: var(--text-md);
}

.shell__name {
  font-size: var(--text-md);
  font-weight: var(--weight-semibold);
  letter-spacing: -0.01em;
}

.shell__panes {
  display: grid;
  grid-template-columns: minmax(200px, 260px) minmax(220px, 320px) 1fr;
  flex: 1;
  min-height: 0;
}

.shell__pane {
  min-width: 0;
  min-height: 0;
}
</style>
