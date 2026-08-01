import { expect, test, type Page } from '@playwright/test';

async function send(page: Page, scenario: string) {
  await page.goto(`/?scenario=${scenario}`);
  await page.getByRole('button', { name: 'Send' }).click();
}

test('streams text through real useChat', async ({ page }) => {
  await send(page, 'text');
  await expect(page.getByText('Hello from ai-go')).toBeVisible();
  await expect(page.getByTestId('status')).toHaveText('ready');
});

test('round-trips a client tool result', async ({ page }) => {
  await send(page, 'tool');
  await page.getByRole('button', { name: 'Run tool' }).click();
  await expect(page.getByText('Tool round-trip complete')).toBeVisible();
  await expect(page.getByTestId('status')).toHaveText('ready');
});

test('round-trips an approval', async ({ page }) => {
  await send(page, 'approval');
  await page.getByRole('button', { name: 'Approve' }).click();
  await expect(page.getByText('Approval accepted')).toBeVisible();
  await expect(page.getByTestId('status')).toHaveText('ready');
});

test('round-trips a denial', async ({ page }) => {
  await send(page, 'approval');
  await page.getByRole('button', { name: 'Deny' }).click();
  await expect(page.getByText('Approval denied')).toBeVisible();
  await expect(page.getByTestId('status')).toHaveText('ready');
});
