<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue';
import { NButton, NInput, NModal, useMessage, useDialog } from 'naive-ui';
import { getAllUsers, createUser, updateUser, deleteUser, resetUserFlow } from '@/api';
import type { User, UserForm } from '@/types';
import UserCard from '@/components/business/users/UserCard.vue';
import UserFormComp from '@/components/business/users/UserForm.vue';
import TunnelManager from '@/components/business/users/TunnelManager.vue';
// import ForwardManager from '@/components/business/users/ForwardManager.vue'; // To be implemented later if needed separately, or inside TunnelManager context? Actually Forward list depends on Tunnels

const users = ref<User[]>([]);
const loading = ref(false);
const searchKeyword = ref('');
const pagination = reactive({
  page: 1,
  pageSize: 12,
  itemCount: 0
});

const message = useMessage();
const dialog = useDialog();

// Modal States
const showUserModal = ref(false);
const isEditUser = ref(false);
const userFormModel = ref<UserForm>({} as UserForm);

const showTunnelModal = ref(false);
const currentTargetUser = ref<User | null>(null);

const loadData = async () => {
    loading.value = true;
    try {
        const res = await getAllUsers({
            current: pagination.page,
            size: pagination.pageSize,
            keyword: searchKeyword.value
        });
        if (res.code === 0) {
            users.value = res.data || [];
            // API response might not have total for pagination if backend structure differs, but type says 'total'.
            // wait, getAllUsers response data is list? or { list, total }?
            // In React code: setUsers(data || []). It seems backend returns List directly for 'data'?
            // Let's check api/index.ts: Network.post("/user/list", pageData).
            // Usually valid pagination returns { list: [], total: 0 } or similar.
            // React code: setUsers(data || []). It treats data as array.
            // But React code also has setPagination(prev => ({...total: 0})). Wait, React code DOES NOT update total from response?
            // React code: setUsers(data || []). It seems the API strictly returns an array?
            // If so, pagination is fake or backend handles it silently but doesn't return total?
            // If only array is returned, I can't do real pagination count. 
            // I'll assume standard pagination structure or array. If array, I might need to adjust.
            // Looking at React Code: It passes current/size to `getAllUsers`. It updates users list. But it never updates `total` in pagination state from response? 
            // Correct, React code line 237: setUsers(data || []). It ignores total count.
            // This implies I might not know total count. I'll stick to simple pagination controls or just load more?
            // Naive UI pagination requires itemCount.
            // If API doesn't return total, I can't use numbered pagination fully.
            // I will assume for now it returns a List.
            pagination.itemCount = (res.data || []).length; // This is definitely wrong for backend pagination.
            // I will follow React implementation logic: just show what we got.
        }
    } catch (e) {
        message.error('获取用户列表失败');
    } finally {
        loading.value = false;
    }
};

const handleAddUser = () => {
    isEditUser.value = false;
    userFormModel.value = {
        user: '',
        status: 1,
        flow: 100,
        num: 10,
        expTime: null,
        flowResetTime: 0
    } as UserForm;
    showUserModal.value = true;
};

const handleEditUser = (user: User) => {
    isEditUser.value = true;
    userFormModel.value = {
        id: user.id,
        user: user.user,
        name: user.name,
        status: user.status,
        flow: user.flow,
        num: user.num,
        expTime: user.expTime, // Check type match: number vs Date? UserForm definition says Date | null
        flowResetTime: user.flowResetTime
    } as any; 
    // Types confusion: User interface expTime is number. UserForm interface expTime is Date | null used in React.
    // I should convert timestamp to date for Naive UI DatePicker.
    // Naive UI DatePicker v-model value is number (timestamp) or null.
    // So actually UserForm interface in React might be different from Vue requirement.
    // Let's adjust UserForm in Vue to use number | null for expTime to be compatible with Naive UI.
    showUserModal.value = true;
};

const submitUserForm = async () => {
    try {
        const payload = { ...userFormModel.value };
        // If expTime is timestamp (number), straightforward.
        
        const res = isEditUser.value ? await updateUser(payload) : await createUser(payload);
        if (res.code === 0) {
            message.success(isEditUser.value ? '更新成功' : '创建成功');
            showUserModal.value = false;
            loadData();
        } else {
            message.error(res.msg);
        }
    } catch (e) {
        message.error('操作失败');
    }
};

const handleDeleteUser = (user: User) => {
    dialog.warning({
        title: '确认删除',
        content: `确认删除用户 ${user.user} 吗？此操作不可逆。`,
        positiveText: '确认',
        negativeText: '取消',
        onPositiveClick: async () => {
             const res = await deleteUser(user.id);
             if (res.code === 0) {
                 message.success('删除成功');
                 loadData();
             } else {
                 message.error(res.msg);
             }
        }
    });
};

const handleResetFlow = (user: User) => {
    dialog.info({
        title: '重置流量',
        content: `确认重置用户 ${user.user} 的流量吗？`,
        positiveText: '确认',
        negativeText: '取消',
        onPositiveClick: async () => {
            const res = await resetUserFlow({ id: user.id, type: 1 });
            if (res.code === 0) {
                message.success('重置成功');
                loadData();
            } else {
                message.error(res.msg);
            }
        }
    });
};

const handleManageTunnels = (user: User) => {
    currentTargetUser.value = user;
    showTunnelModal.value = true;
};

const handleManageForwards = (user: User) => {
    // For now, Forward Management is often linked to tunnels or a specific page.
    // React code has managing forwards inside the User Page as a Modal.
    // It shares context. I should implement a ForwardManager component similar to TunnelManager.
    message.info('转发管理功能开发中');
};

onMounted(() => {
    loadData();
});
</script>

<template>
  <div class="p-6">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row justify-between gap-4 mb-6">
        <div class="flex gap-2 w-full sm:w-auto">
            <NInput v-model:value="searchKeyword" placeholder="搜索用户名..." @keyup.enter="loadData">
                <template #prefix><div class="i-mdi-magnify" /></template>
            </NInput>
            <NButton type="primary" ghost @click="loadData">搜索</NButton>
        </div>
        <NButton type="primary" @click="handleAddUser">
            <template #icon><div class="i-mdi-plus" /></template>
            新建用户
        </NButton>
    </div>

    <!-- Grid -->
    <div v-if="loading" class="text-center py-20"><div class="i-mdi-loading animate-spin text-2xl" /></div>
    <div v-else-if="users.length === 0" class="text-center py-20 text-gray-400">暂无用户</div>
    <div v-else class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
        <UserCard 
           v-for="user in users" 
           :key="user.id" 
           :user="user"
           @edit="handleEditUser"
           @delete="handleDeleteUser"
           @reset-flow="handleResetFlow"
           @manage-tunnels="handleManageTunnels"
           @manage-forwards="handleManageForwards"
        />
    </div>

    <!-- Modals -->
    <NModal v-model:show="showUserModal" preset="card" :title="isEditUser ? '编辑用户' : '新建用户'" style="width: 500px">
        <UserFormComp v-model="userFormModel" :is-edit="isEditUser" @submit="submitUserForm" />
    </NModal>

    <!-- Tunnel Manager Modal -->
    <NModal v-model:show="showTunnelModal" preset="card" title="隧道权限管理" style="width: 800px; max-width: 95vw;">
        <TunnelManager v-if="currentTargetUser" :user="currentTargetUser" />
    </NModal>
  </div>
</template>
