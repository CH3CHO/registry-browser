<script setup lang="ts">
import { computed, ref } from 'vue'
import type { TagInfo } from '../../bindings/github.com/ch3cho/registry-browser/internal/app/models'

const props = defineProps<{
  repository: string
  tags: TagInfo[]
  activeTag: string
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  (event: 'select-tag', tag: string): void
}>()

const filter = ref('')

const visibleTags = computed(() => {
  const keyword = filter.value.trim().toLowerCase()
  if (!keyword) {
    return props.tags
  }
  return props.tags.filter((tag) => tag.name.toLowerCase().includes(keyword))
})
</script>

<template>
  <section class="tags" aria-label="标签列表">
    <header class="tags__head">
      <div class="tags__title">
        <h2 class="tags__heading">{{ props.repository || '未选择镜像' }}</h2>
        <span v-if="props.tags.length" class="tags__count">{{ props.tags.length }} 个 tag</span>
      </div>
      <input
        v-model="filter"
        class="tags__filter"
        type="search"
        placeholder="过滤 tag"
        :disabled="!props.tags.length"
        aria-label="过滤 tag"
      />
    </header>

    <div class="tags__body">
      <p v-if="props.loading" class="tags__note">正在读取 tag…</p>
      <p v-else-if="props.error" class="tags__note tags__note--error">{{ props.error }}</p>
      <p v-else-if="!props.repository" class="tags__note">从左侧选择一个镜像，或在上方输入镜像地址。</p>
      <p v-else-if="!visibleTags.length" class="tags__note">没有匹配的 tag。</p>

      <table v-else class="tags__table">
        <thead>
          <tr>
            <th scope="col">Tag</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="tag in visibleTags"
            :key="tag.name"
            class="tags__row"
            :class="{ 'tags__row--active': tag.name === props.activeTag }"
            @click="emit('select-tag', tag.name)"
          >
            <td class="tags__cell">{{ tag.name }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.tags {
  display: flex;
  flex-direction: column;
  min-width: 0;
  height: 100%;
  background: var(--surface-page);
  border-right: 1px solid var(--border-hairline);
}

.tags__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--border-hairline);
}

.tags__title {
  display: flex;
  align-items: baseline;
  gap: var(--space-3);
  min-width: 0;
}

.tags__heading {
  margin: 0;
  font-family: var(--font-mono);
  font-size: var(--text-md);
  font-weight: var(--weight-medium);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tags__count {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  white-space: nowrap;
}

.tags__filter {
  width: 140px;
  padding: var(--space-2) var(--space-3);
  background: var(--surface-raised);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  font-size: var(--text-sm);
}

.tags__filter:disabled {
  color: var(--text-tertiary);
  background: var(--surface-sunken);
}

.tags__body {
  flex: 1;
  overflow-y: auto;
}

.tags__note {
  margin: 0;
  padding: var(--space-5);
  color: var(--text-tertiary);
  font-size: var(--text-sm);
}

.tags__note--error {
  color: var(--status-error);
}

.tags__table {
  width: 100%;
  border-collapse: collapse;
}

.tags__table th {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: var(--space-2) var(--space-5);
  background: var(--surface-sunken);
  border-bottom: 1px solid var(--border-hairline);
  text-align: left;
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-tertiary);
}

.tags__row {
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.tags__row:hover {
  background: var(--surface-hover);
}

.tags__row--active {
  background: var(--surface-selected);
}

.tags__cell {
  padding: var(--space-2) var(--space-5);
  font-family: var(--font-mono);
  font-size: var(--text-sm);
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-hairline);
}

.tags__row--active .tags__cell {
  color: var(--text-primary);
  font-weight: var(--weight-medium);
}
</style>
