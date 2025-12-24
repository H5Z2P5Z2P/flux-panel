
// Plugin to load TAC script
export const loadTajs = () => {
    return new Promise<void>((resolve, reject) => {
        if ((window as any).TAC) {
            resolve();
            return;
        }

        // Load CSS
        const link = document.createElement('link');
        link.rel = 'stylesheet';
        link.href = '/tac.css';
        document.head.appendChild(link);

        // Load JS
        const script = document.createElement('script');
        script.src = '/tac.min.js';
        script.onload = () => resolve();
        script.onerror = () => reject();
        document.body.appendChild(script);
    });
}
