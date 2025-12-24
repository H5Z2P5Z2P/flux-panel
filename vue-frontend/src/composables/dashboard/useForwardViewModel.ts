import { computed } from 'vue';

export function useForwardViewModel(forwardList: any[]) { // Using 'any' for now to match flexible inputs, stricter type preferred

    const groupedForwards = computed(() => {
        const groups: Record<string, { tunnelName: string; forwards: any[] }> = {};

        if (!forwardList) return [];

        forwardList.forEach(forward => {
            const tunnelName = forward.tunnelName || '未知隧道';
            if (!groups[tunnelName]) {
                groups[tunnelName] = {
                    tunnelName,
                    forwards: []
                };
            }
            groups[tunnelName].forwards.push(forward);
        });

        return Object.values(groups);
    });

    return {
        groupedForwards
    };
}
