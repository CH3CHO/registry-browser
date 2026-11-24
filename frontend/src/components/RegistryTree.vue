<script setup lang="ts">
import type { NamespaceEntry, RegistryInfo } from '../../bindings/github.com/ch3cho/registry-browser/internal/app/models'

const props = defineProps<{
  registries: RegistryInfo[]
  activeRegistry: string
  namespaces: NamespaceEntry[]
  activeRepository: string
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  (event: 'select-registry', host: string): void
  (event: 'select-image', repository: string): void
}>()

function onRegistryChange(event: Event) {
  emit('select-registry', (event.target as HTMLSelectElement).value)
}
</script>

<template>
  <nav class="tree" aria-label="仓库目录">
    <header class="tree__head">
      <label class="tree__label" for="registry-select">Registry</label>
      <select
        id="registry-select"
        class="tree__select"
        :value="activeRegistry"
        @change="onRegistryChange"
      >
        <option v-for="registry in props.registries" :key="registry.host" :value="registry.host">
          {{ registry.name }}
        </option>
      </select>
    </header>

    <div class="tree__body">
      <p v-if="props.loading" class="tree__note">正在读取目录…</p>

      <p v-else-if="props.error" class="tree__note tree__note--error">{{ props.error }}</p>

      <p v-else-if="props.namespaces.length === 0" class="tree__note">
        该 registry 不支持目录列举<br />
        请在上方输入完整镜像地址查询
      </p>

      <ul v-else class="tree__list">
        <li v-for="group in props.namespaces" :key="group.name" class="tree__group">
          <span class="tree__group-name">{{ group.name || '(根)' }}</span>
          <ul class="tree__images">
            <li v-for="image in group.images" :key="image.repository">
              <button
                type="button"
                class="tree__image"
                :class="{ 'tree__image--active': image.repository === props.activeRepository }"
                @click="emit('select-image', image.repository)"
              >
                {{ image.name }}
              </button>
            </li>
          </ul>
        </li>
      </ul>
    </div>
  </nav>
</template>

<style scoped>
.tree {
  display: flex;
  flex-direction: column;
  min-width: 0;
  height: 100%;
  background: var(--surface-sunken);
  border-right: 1px solid var(--border-hairline);
}

.tree__head {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
  border-bottom: 1px solid var(--border-hairline);
}

.tree__label {
  font-size: var(--text-xs);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-tertiary);
}

.tree__select {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--surface-raised);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  font-size: var(--text-sm);
}

.tree__body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-3) 0;
}

.tree__note {
  padding: var(--space-4);
  color: var(--text-tertiary);
  font-size: var(--text-xs);
  line-height: var(--leading-normal);
}

.tree__note--error {
  color: var(--status-error);
}

.tree__list,
.tree__images {
  list-style: none;
  margin: 0;
  padding: 0;
}

.tree__group + .tree__group {
  margin-top: var(--space-3);
}

.tree__group-name {
  display: block;
  padding: var(--space-2) var(--space-4);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-tertiary);
}

.tree__image {
  display: block;
  width: 100%;
  padding: var(--space-2) var(--space-4) var(--space-2) var(--space-5);
  border: 0;
  border-left: 2px solid transparent;
  background: transparent;
  text-align: left;
  font-size: var(--text-sm);
  color: var(--text-secondary);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.tree__image:hover {
  background: var(--surface-hover);
  color: var(--text-primary);
}

.tree__image--active {
  background: var(--surface-selected);
  border-left-color: var(--accent);
  color: var(--text-primary);
  font-weight: var(--weight-medium);
}
</style>
