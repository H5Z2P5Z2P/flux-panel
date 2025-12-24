<script setup lang="ts">
import { computed } from 'vue';
import { NCard, NTag, NButton, NProgress, NDropdown, useMessage, useDialog } from 'naive-ui';
import type { User } from '@/types';
import { formatFlow, getExpStatus } from '@/utils/format';
import { getGuestLink } from '@/api';
import { useClipboard } from '@vueuse/core';

const props = defineProps<{
  user: User;
}>();

const emit = defineEmits(['edit', 'delete', 'manage-tunnels', 'manage-forwards', 'reset-flow']);

const message = useMessage();
const dialog = useDialog();
const { copy, isSupported } = useClipboard();

const status = computed(() => {
  return props.user.status === 1 
    ? { type: 'success' as const, text: '正常' } 
    : { type: 'error' as const, text: '禁用' };
});

const usedFlow = computed(() => (props.user.inFlow || 0) + (props.user.outFlow || 0));
const totalFlow = computed(() => (props.user.flow || 0) * 1024 * 1024 * 1024);
const flowPercentage = computed(() => {
  if (!totalFlow.value) return 0;
  return Math.min((usedFlow.value / totalFlow.value) * 100, 100);
});

const expStatus = computed(() => getExpStatus(props.user.expTime));

const handleShare = async () => {
    try {
      const res = await getGuestLink(props.user.id);
      if (res.code === 0 && res.data?.token) {
        const url = `${window.location.origin}/guest?token=${res.data.token}`;
        if (isSupported) {
          await copy(url);
          message.success('访客链接已复制');
        } else {
          message.info('请手动复制: ' + url);
        }
      } else {
        message.error(res.msg || '获取链接失败');
      }
    } catch (e) {
      message.error('获取链接失败');
    }
};

const options = [
  { label: '编辑用户', key: 'edit' },
  { label: '隧道权限', key: 'tunnels' },
  { label: '转发管理', key: 'forwards' },
  { label: '重置流量', key: 'reset' },
  { type: 'divider', key: 'd1' },
  { label: '删除用户', key: 'delete', props: { style: { color: 'red' } } }
];

const handleSelect = (key: string) => {
  switch (key) {
    case 'edit': emit('edit', props.user); break;
    case 'tunnels': emit('manage-tunnels', props.user); break;
    case 'forwards': emit('manage-forwards', props.user); break;
    case 'reset': emit('reset-flow', props.user); break;
    case 'delete': emit('delete', props.user); break;
  }
};
</script>

<template>
  <NCard size="small" hoverable class="h-full">
    <template #header>
      <div class="flex justify-between items-start">
        <div class="min-w-0 pr-2">
          <div class="font-bold text-base truncate">{{ user.name || user.user }}</div>
          <div class="text-xs text-gray-500 truncate">@{{ user.user }}</div>
        </div>
        <NTag size="small" :type="status.type" class="flex-shrink-0">{{ status.text }}</NTag>
      </div>
    </template>
    
    <template #header-extra>
        <NDropdown trigger="click" :options="options" @select="handleSelect">
            <NButton text class="text-gray-400 hover:text-primary">
                <div class="i-mdi-dots-vertical text-xl" />
            </NButton>
        </NDropdown>
    </template>

    <div class="space-y-3 mt-1">
      <!-- Expire Time -->
      <div class="flex justify-between text-xs">
         <span class="text-gray-500">状态</span>
         <span :class="expStatus.color">{{ expStatus.text }}</span>
      </div>

      <!-- Flow Progress -->
      <div>
        <div class="flex justify-between text-xs mb-1">
          <span class="text-gray-500">流量 ({{ formatFlow(user.flow, 'gb') }})</span>
          <span class="text-gray-700 dark:text-gray-300">{{ formatFlow(usedFlow) }}</span>
        </div>
        <NProgress 
          type="line" 
          :percentage="flowPercentage" 
          :color="flowPercentage > 90 ? '#ef4444' : flowPercentage > 70 ? '#f97316' : '#10b981'"
          :height="6"
          :show-indicator="false"
        />
      </div>

      <!-- Actions Row -->
      <div class="flex gap-2 pt-2">
        <NButton size="tiny" secondary type="primary" class="flex-1" @click="handleShare">
           <template #icon><div class="i-mdi-share-variant" /></template>
           分享
        </NButton>
        <NButton size="tiny" secondary @click="emit('manage-tunnels', user)" class="flex-1">
           <template #icon><div class="i-mdi-tunnel" /></template>
           隧道
        </NButton>
      </div>
    </div>
  </NCard>
</template>
