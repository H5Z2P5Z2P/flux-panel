import { formatFlow } from './format';

interface StatisticsFlow {
    time: string;
    flow?: number;
}

export const processFlowChartData = (statisticsFlows: StatisticsFlow[]) => {
    const now = new Date();
    const hours: string[] = [];

    // Create last 24h labels
    for (let i = 23; i >= 0; i--) {
        const time = new Date(now.getTime() - i * 60 * 60 * 1000);
        const hourString = time.getHours().toString().padStart(2, '0') + ':00';
        hours.push(hourString);
    }

    const flowMap = new Map<string, number>();
    statisticsFlows.forEach(item => {
        flowMap.set(item.time, item.flow || 0);
    });

    return hours.map(hour => {
        const flowVal = flowMap.get(hour) || 0;
        return {
            time: hour,
            value: flowVal,
            formattedFlow: formatFlow(flowVal)
        };
    });
};
