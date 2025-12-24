<script setup lang="ts">
import { NModal, NCard, NDataTable, NButton } from 'naive-ui';
import type { AddressItem } from '@/composables/dashboard/useAddressModal';

const props = defineProps<{
  show: boolean;
  title: string;
  list: AddressItem[];
}>();

const emit = defineEmits(['update:show', 'copy', 'copy-all']);

const columns = [
  { title: '地址', key: 'address' },
  { title: '操作', key: 'action', render: (row: AddressItem) => {
      // Create VNode for Button? Or simpler usage in NDataTable
      // Since render function in script setup is tricky without h, let's use list view instead of DataTable for simplicity, or simple template
      return null; 
    }
  }
];
// Switching to simple list to avoid h() render complexity in file generation
</script>

<template>
  <NModal :show="show" @update:show="val => $emit('update:show', val)">
    <NCard
      style="width: 600px; max-width: 90vw;"
      :title="title"
      :bordered="false"
      size="huge"
      role="dialog"
      aria-modal="true"
    >
      <template #header-extra>
        <NButton size="small" @click="$emit('copy-all')">
          复制全部
        </NButton>
      </template>
      
      <div class="max-h-[60vh] overflow-y-auto space-y-2">
        <div v-for="item in list" :key="item.id" 
          class="flex justify-between items-center p-3 border border-gray-100 dark:border-gray-800 rounded-lg bg-gray-50 dark:bg-gray-900/50"
        >
          <code class="text-sm font-mono break-all mr-4">{{ item.address }}</code>
          <NButton size="tiny" secondary @click="$emit('copy', item)">
            复制
          </NButton>
        </div>
      </div>
    </NCard>
  </NModal>
</template>
