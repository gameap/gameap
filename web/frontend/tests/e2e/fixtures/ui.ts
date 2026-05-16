import { expect, type APIResponse, type Page } from '@playwright/test';

// naive-ui $dialog / notification "Close" action button — i18n-tolerant,
// including the raw i18n key rendered when window.i18n is unpopulated.
const CLOSE = /close|закрыть|main\.close/i;

// Server 500s are masked to a generic body by the API responder; surface the
// response text in the failure message so a backend error is self-describing
// without a re-run. The body can be evicted once the page navigates, so it is
// read best-effort and only on a status mismatch.
export async function expectStatus(
  response: APIResponse,
  expected: number,
  label: string,
): Promise<void> {
  const actual = response.status();
  if (actual === expected) {
    return;
  }

  let body = '<unavailable>';
  try {
    body = await response.text();
  } catch {
    // Response body evicted by a page navigation (e.g. a successful login
    // triggers location.reload()).
  }

  expect(actual, `${label} (body: ${body})`).toBe(expected);
}

// naive-ui notification() opens a blocking $dialog (role="dialog") with a
// single "Close" action button. Scope to the top-most dialog so a stray
// card-header close "X" (aria-label="close") elsewhere on the page does not
// cause a strict-mode match.
export async function dismissTopDialog(page: Page): Promise<void> {
  const close = page
    .getByRole('dialog')
    .last()
    .getByRole('button', { name: CLOSE });
  await expect(close).toBeVisible({ timeout: 10_000 });
  await close.click();
  await expect(close).toBeHidden({ timeout: 10_000 });
}
