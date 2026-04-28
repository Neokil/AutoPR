import { test, expect, type Locator, type Page } from '@playwright/test';
import * as fs from 'fs';
import { FIXTURES_FILE, type E2EFixtures } from '../global-setup';

function loadFixtures(): E2EFixtures {
  return JSON.parse(fs.readFileSync(FIXTURES_FILE, 'utf-8')) as E2EFixtures;
}

function ticketStatus(ticketItem: Locator): Locator {
  return ticketItem.locator('.ticket-status-row .meta');
}

async function waitForStatus(ticketItem: Locator, status: string): Promise<void> {
  await expect(ticketStatus(ticketItem)).toContainText(status, { timeout: 30_000 });
}

test('full ticket lifecycle: add → investigate → implement → done → cleanup', async ({ page }) => {
  const { repoDir, ticketId } = loadFixtures();

  await page.goto('/');

  // ── 1. Open "Add Ticket" dialog ──────────────────────────────────────────
  await page.getByRole('button', { name: 'Add Ticket' }).click();

  const modal = page.locator('.modal');
  await expect(modal).toBeVisible();

  await page.locator('#repo-path-input').fill(repoDir);
  await page.locator('#ticket-number-input').fill(ticketId);
  await modal.getByRole('button', { name: 'Schedule Run' }).click();

  await expect(modal).not.toBeVisible();

  // ── 2. Ticket appears in the list; select it ─────────────────────────────
  const ticketItem = page.locator('.ticket-item').filter({ hasText: ticketId });
  await expect(ticketItem).toBeVisible();
  await ticketItem.click();

  // ── 3. Wait for investigate state (status = "waiting") ───────────────────
  await waitForStatus(ticketItem, 'waiting');

  // Action buttons are in the right detail panel
  await expect(page.getByRole('button', { name: 'Approve' })).toBeVisible();

  // ── 4. Approve → transition to implementation ────────────────────────────
  await page.getByRole('button', { name: 'Approve' }).click();

  // ── 5. Wait for implementation state (status = "waiting") ────────────────
  await waitForStatus(ticketItem, 'waiting');

  await expect(page.getByRole('button', { name: 'Accept' })).toBeVisible();

  // ── 6. Accept → transition to done ──────────────────────────────────────
  await page.getByRole('button', { name: 'Accept' }).click();

  // ── 7. Verify done ───────────────────────────────────────────────────────
  await waitForStatus(ticketItem, 'done');

  // ── 8. Cleanup via hamburger menu ────────────────────────────────────────
  await page.locator('.menu-trigger').click();

  const cleanupItem = page.locator('.menu-item').filter({ hasText: 'Cleanup' });
  await expect(cleanupItem).toBeVisible();
  await cleanupItem.click();

  // ── 9. Ticket removed from list ──────────────────────────────────────────
  await expect(ticketItem).not.toBeVisible({ timeout: 10_000 });
});

async function screenshotOnError(page: Page, name: string): Promise<void> {
  await page.screenshot({ path: `/tmp/autopr-e2e-${name}.png` });
}
