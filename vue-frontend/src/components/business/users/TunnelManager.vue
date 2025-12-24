<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { NButton, NSelect, NTable, NTag, NSpace, NInputNumber, useMessage, useDialog } from 'naive-ui';
import type { User, UserTunnel, Tunnel } from '@/types';
import { getUserTunnelList, assignUserTunnel, removeUserTunnel, getTunnelList } from '@/api';

const props = defineProps<{
  user: User;
}>();

const loading = ref(false);
const userTunnels = ref<UserTunnel[]>([]);
const allTunnels = ref<Tunnel[]>([]);
const assignForm = ref({
  tunnelId: null as number | null,
  speedId: null
});

const message = useMessage();
const dialog = useDialog();

const loadData = async () => {
  loading.value = true;
  try {
    const [utRes, tRes] = await Promise.all([
      getUserTunnelList({ userId: props.user.id }),
      getTunnelList()
    ]);

    if (utRes.code === 0) userTunnels.value = utRes.data || [];
    if (tRes.code === 0) allTunnels.value = tRes.data || [];
  } catch (e) {
    message.error('加载隧道数据失败');
  } finally {
    loading.value = false;
  }
};

const handleAssign = async () => {
  if (!assignForm.value.tunnelId) {
    message.warning('请选择隧道');
    return;
  }
  
  try {
    const res = await assignUserTunnel({
      userId: props.user.id,
      tunnelId: assignForm.value.tunnelId,
      speedId: assignForm.value.speedId
    });
    
    if (res.code === 0) {
      message.success('分配成功');
      loadData();
      assignForm.value.tunnelId = null;
    } else {
      message.error(res.msg);
    }
  } catch (e) {
    message.error('分配失败');
  }
};

const handleRemove = (item: UserTunnel) => {
  dialog.warning({
    title: '移除权限',
    content: `确认移除隧道 ${item.tunnelName} 吗？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: async () => {
      const res = await removeUserTunnel({ id: item.id });
      if (res.code === 0) {
        message.success('移除成功');
        loadData();
      } else {
        message.error(res.msg);
      }
    }
  });
};

onMounted(() => {
  loadData();
});
</script>

<template>
  <div class="space-y-6">
    <!-- Assign New Tunnel -->
    <div class="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg flex items-end gap-4">
      <div class="flex-1">
        <div class="text-xs text-gray-500 mb-1">选择隧道</div>
        <NSelect 
          v-model:value="assignForm.tunnelId" 
          :options="allTunnels.map(t => ({ label: `${t.name} (ID:${t.id})`, value: t.id }))" 
          placeholder="选择隧道"
          filterable
        />
      </div>
      <!-- Speed Limit Placeholder - API supports it but simplified UI for now -->
      <!-- <div class="w-40">
        <div class="text-xs text-gray-500 mb-1">限速规则</div>
        <NSelect v-model:value="assignForm.speedId" placeholder="无限制" />
      </div> -->
      <NButton type="primary" @click="handleAssign" :disabled="!assignForm.tunnelId">
        分配权限
      </NButton>
    </div>

    <!-- User Tunnels List -->
    <NTable size="small" :bordered="false" :single-line="false">
      <thead>
        <tr>
          <th>ID</th>
          <th>隧道名称</th>
          <th>流量规则</th>
          <th>限速</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="userTunnels.length === 0">
           <td colspan="5" class="text-center py-4 text-gray-400">暂无分配的隧道</td>
        </tr>
        <tr v-for="item in userTunnels" :key="item.id">
          <td>{{ item.tunnelId }}</td>
          <td>{{ item.tunnelName }}</td>
          <td>
            <NTag size="small" :type="item.tunnelFlow === 1 ? 'info' : 'warning'">
              {{ item.tunnelFlow === 1 ? '单向' : '双向' }}
            </NTag>
          </td>
          <td>{{ item.speedLimitName || '无限制' }}</td>
          <td>
            <NButton size="tiny" type="error" ghost @click="handleRemove(item)">移除</NButton>
          </td>
        </tr>
      </tbody>
    </NTable>
  </div>
</template>
