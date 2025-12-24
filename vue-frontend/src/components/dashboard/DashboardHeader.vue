<script setup lang="ts">
import { computed } from 'vue';
import { NButton, NTag, NIcon } from 'naive-ui';
import { useClipboard } from '@vueuse/core';
import { useMessage } from 'naive-ui';
import { getGuestLink } from '@/api';

const props = defineProps<{
  expTime?: number;
  loading?: boolean;
}>();

const emit = defineEmits(['refresh']);

const message = useMessage();
const { copy, isSupported } = useClipboard();

const expireStatus = computed(() => {
  if (!props.expTime) return { type: 'success', text: '永久', color: undefined };
  
  const now = Date.now();
  if (props.expTime < now) return { type: 'error', text: '已过期' };
  
  const diff = props.expTime - now;
  const days = Math.ceil(diff / (86400 * 1000));
  
  if (days <= 7) return { type: 'warning', text: `${days}天后过期` };
  return { type: 'success', text: `${days}天后过期` };
});

const formatDate = (ts?: number) => {
  if (!ts) return '';
  return new Date(ts).toLocaleString();
};

const handleShare = async () => {
  try {
    const res = await getGuestLink();
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
</script>

<template>
  <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
    <div>
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">仪表盘</h1>
      <p class="text-sm text-gray-500 dark:text-gray-400">概览与状态监控</p>
    </div>
    
    <div class="flex items-center gap-3">
      <div v-if="expTime" class="hidden md:flex items-center gap-2 px-4 py-2 rounded-lg border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
        <span class="text-sm font-medium">到期时间:</span>
        <span class="text-sm font-bold">{{ formatDate(expTime) }}</span>
        <NTag :type="expireStatus.type as any" size="small" round :bordered="false">
          {{ expireStatus.text }}
        </NTag>
      </div>

      <NButton secondary type="primary" size="small" @click="handleShare">
        分享访客链接
      </NButton>

      <NButton quaternary circle @click="$emit('refresh')" :loading="loading">
        <template #icon>
          <div class="i-mdi-refresh text-xl" />
        </template>
      </NButton>
    </div>
  </div>
</template>
