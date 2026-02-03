const { chromium } = require('playwright');

(async () => {
    const browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
        viewport: { width: 390, height: 844 }, // iPhone 14 size
        userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15'
    });
    const page = await context.newPage();

    console.log('1. 訪問登入頁...');
    await page.goto('https://bookbark.io/weekend-chart/');
    await page.screenshot({ path: '/tmp/test-1-login.png' });

    console.log('2. 輸入帳號密碼...');
    await page.fill('#username', 'wake');
    await page.fill('#password', '721225');
    await page.screenshot({ path: '/tmp/test-2-filled.png' });

    console.log('3. 點擊登入...');
    await page.click('button[type="submit"]');
    await page.waitForURL('**/dashboard.html', { timeout: 10000 });
    await page.screenshot({ path: '/tmp/test-3-dashboard.png' });

    console.log('4. 檢查 Agent 列表...');
    const agents = await page.locator('.agent-card').count();
    console.log(`   找到 ${agents} 個 Agent`);

    if (agents > 0) {
        console.log('5. 點擊第一個 Agent...');
        await page.locator('.agent-card').first().click();
        await page.waitForURL('**/remote.html**', { timeout: 5000 });
        await page.waitForTimeout(2000);
        await page.screenshot({ path: '/tmp/test-4-remote.png' });

        console.log('6. 測試輸入框...');
        const inputBar = page.locator('#textInput');
        const isVisible = await inputBar.isVisible();
        console.log(`   輸入框可見: ${isVisible}`);

        if (isVisible) {
            await inputBar.focus();
            await inputBar.fill('Hello World');
            const value = await inputBar.inputValue();
            console.log(`   輸入值: "${value}"`);
            await page.screenshot({ path: '/tmp/test-5-input.png' });
        }
    }

    console.log('\n測試完成！截圖已保存到 /tmp/test-*.png');
    await browser.close();
})();
