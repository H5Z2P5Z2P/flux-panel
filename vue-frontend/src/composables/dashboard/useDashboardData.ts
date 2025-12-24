import { ref } from 'vue';
import { getUserPackageInfo, getGuestDashboard } from '@/api';
import type { User, UserTunnel } from '@/types';
import { useMessage } from 'naive-ui';

interface Forward {
    id: number;
    name: string;
    tunnelId: number;
    tunnelName: string;
    inIp: string;
    inPort: number;
    remoteAddr: string;
    inFlow: number;
    outFlow: number;
    // Guest mode might have createdTime etc
}

interface StatisticsFlow {
    id: number;
    userId: number;
    flow: number;
    totalFlow: number;
    time: string;
}

export function useDashboardData(isGuest = false, token?: string) {
    const loading = ref(true);
    const userInfo = ref<User>({} as User);
    const userTunnels = ref<UserTunnel[]>([]);
    const forwardList = ref<Forward[]>([]);
    const statisticsFlows = ref<StatisticsFlow[]>([]);

    const message = useMessage();

    const loadData = async () => {
        loading.value = true;
        try {
            // Clear data first
            userInfo.value = {} as User;
            userTunnels.value = [];
            forwardList.value = [];
            statisticsFlows.value = [];

            let res;
            if (isGuest && token) {
                res = await getGuestDashboard(token);
            } else {
                res = await getUserPackageInfo();
            }

            if (res.code === 0) {
                const data = res.data;
                userInfo.value = data.userInfo || {};
                userTunnels.value = data.tunnelPermissions || [];
                forwardList.value = data.forwards || [];
                statisticsFlows.value = data.statisticsFlows || [];
            } else {
                message.error(res.msg || '获取数据失败');
            }
        } catch (error) {
            console.error('Failed to load dashboard data:', error);
            message.error('获取数据失败');
        } finally {
            loading.value = false;
        }
    };

    return {
        loading,
        userInfo,
        userTunnels,
        forwardList,
        statisticsFlows,
        loadData
    };
}
