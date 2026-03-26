/**
 * Captures full-page screenshots of every main tab and the Settings modal.
 * Run scripts/generate-demo-gif.sh to assemble docs/demo.gif with ffmpeg.
 */
import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

const FRAMES_DIR = path.join(__dirname, '../screenshots/frames');

test.describe('demo gif frames', () => {
  test.beforeAll(() => {
    fs.mkdirSync(FRAMES_DIR, { recursive: true });
  });

  test('capture all screens for animated gif', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 });

    await page.goto('/');
    await expect(page.getByTestId('app-title')).toBeVisible({ timeout: 30_000 });

    // Let status bar settle (TV may be unreachable in CI — UI still renders).
    await page.waitForTimeout(800);

    let n = 1;
    const shot = async (_label: string) => {
      const file = path.join(FRAMES_DIR, `frame-${String(n).padStart(2, '0')}.png`);
      await page.screenshot({ path: file, fullPage: true });
      console.log('[demo-gif] wrote', file);
      n++;
    };

    await shot('remote');
    await page.getByTestId('tab-apps').click();
    await page.waitForTimeout(400);
    await shot('apps');

    await page.getByTestId('tab-inputs').click();
    await page.waitForTimeout(400);
    await shot('inputs');

    await page.getByTestId('tab-picture').click();
    await page.waitForTimeout(400);
    await shot('picture');

    await page.getByTestId('tab-power').click();
    await page.waitForTimeout(400);
    await shot('power');

    await page.getByTestId('settings-btn').click();
    await expect(page.getByTestId('settings-modal')).toBeVisible();
    await page.waitForTimeout(400);
    await shot('settings');

    await page.locator('#closeSettings').click();
    await expect(page.getByTestId('settings-modal')).not.toBeVisible();

    await page.getByTestId('tab-remote').click();
    await page.waitForTimeout(300);
    await shot('remote-final');
  });
});
