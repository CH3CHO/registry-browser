<script setup lang="ts">
defineProps<{
  modelValue: string
  busy: boolean
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'submit'): void
}>()

function onInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}
</script>

<template>
  <form class="address" role="search" @submit.prevent="emit('submit')">
    <label class="address__label" for="address-input">镜像地址</label>
    <div class="address__row">
      <input
        id="address-input"
        class="address__input"
        type="text"
        spellcheck="false"
        autocomplete="off"
        placeholder="registry.example.com/namespace/image:tag"
        :value="modelValue"
        :disabled="busy"
        @input="onInput"
      />
      <button class="address__submit" type="submit" :disabled="busy">
        {{ busy ? '查询中…' : '查询' }}
      </button>
    </div>
  </form>
</template>

<style scoped>
.address {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  flex: 1;
  min-width: 0;
}

.address__label {
  font-size: var(--text-xs);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-tertiary);
}

.address__row {
  display: flex;
  gap: var(--space-2);
}

.address__input {
  flex: 1;
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  background: var(--surface-raised);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: var(--text-sm);
}

.address__input:disabled {
  background: var(--surface-sunken);
  color: var(--text-tertiary);
}

.address__submit {
  padding: var(--space-2) var(--space-5);
  background: var(--accent);
  border: 1px solid var(--accent);
  border-radius: var(--radius-sm);
  color: #fff;
  font-size: var(--text-sm);
  font-weight: var(--weight-medium);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.address__submit:hover:not(:disabled) {
  background: var(--accent-hover);
  border-color: var(--accent-hover);
}

.address__submit:disabled {
  opacity: 0.55;
  cursor: default;
}
</style>
