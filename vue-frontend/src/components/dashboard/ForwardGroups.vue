<script setup lang="ts">
import { computed } from 'vue';
import { NCard, NGrid, NGridItem, NTag } from 'naive-ui';
import { formatFlow, formatAddress } from '@/utils/format';
import { useForwardViewModel } from '@/composables/dashboard/useForwardViewModel';
import { useAddressModal } from '@/composables/dashboard/useAddressModal';
import AddressModal from './AddressModal.vue';

const props = defineProps<{
  forwardList: any[];
}>();

const { groupedForwards } = useForwardViewModel(props.forwardList);
const addressModal = useAddressModal();

const hasMultiple = (str: string) => str.split(',').filter(Boolean).length > 1;

const handleAddressClick = (ip: string, port: number, title: string) => {
  if (hasMultiple(ip)) {
    addressModal.openModal(ip, port, title);
  } else {
    addressModal.openModal(ip, port, title); // Also support single copy via modal logic or direct
  }
};
</script>

<template>
  <div class="space-y-4">
    <div v-if="!groupedForwards.length" class="text-center py-12 text-gray-400">
      暂无转发配置
    </div>

    <div v-else v-for="group in groupedForwards" :key="group.tunnelName" 
      class="border border-gray-200 dark:border-gray-800 rounded-lg p-4 bg-white dark:bg-gray-900/50"
    >
      <div class="flex items-center justify-between mb-4">
        <h3 class="font-semibold text-gray-900 dark:text-gray-100 flex items-center gap-2">
          <div class="i-mdi-server-network text-primary text-xl" />
          {{ group.tunnelName }}
        </h3>
        <NTag size="small" type="primary" secondary round>
          {{ group.forwards.length }} 个转发
        </NTag>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
        <div v-for="item in group.forwards" :key="item.id" 
          class="bg-gray-50 dark:bg-gray-800/50 rounded-lg p-3 hover:shadow-md transition-shadow border border-transparent hover:border-primary/20"
        >
          <div class="font-medium text-sm mb-2 truncate" :title="item.name">{{ item.name }}</div>
          
          <div 
            class="bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300 px-2 py-1 rounded text-xs font-mono mb-2 truncate cursor-pointer hover:opacity-80 transition-opacity"
            @click="handleAddressClick(item.inIp, item.inPort, '入口地址')"
            :title="formatAddress(item.inIp, item.inPort)"
          >
             {{ formatAddress(item.inIp, item.inPort) }}
          </div>

          <div class="grid grid-cols-3 gap-1 text-xs text-center border-t border-gray-200 dark:border-gray-700 pt-2">
            <div>
              <div class="text-gray-400 mb-0.5">上传</div>
              <div class="font-medium text-green-600">{{ formatFlow(item.inFlow || 0) }}</div>
            </div>
            <div>
              <div class="text-gray-400 mb-0.5">下载</div>
              <div class="font-medium text-orange-600">{{ formatFlow(item.outFlow || 0) }}</div>
            </div>
            <div>
              <div class="text-gray-400 mb-0.5">计费</div>
              <div class="font-medium text-primary">{{ formatFlow((item.inFlow||0) + (item.outFlow||0)) }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <AddressModal 
      :show="addressModal.isOpen.value" 
      :title="addressModal.title.value"
      :list="addressModal.addressList.value"
      @update:show="val => addressModal.isOpen.value = val"
      @copy="addressModal.copyItem"
      @copy-all="addressModal.copyAll"
    />
  </div>
</template>
