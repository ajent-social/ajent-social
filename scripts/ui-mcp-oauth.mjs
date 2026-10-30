// Browser acceptance of the real consent form and its cross-origin OAuth redirect.
import { chromium } from 'playwright';
import { mkdir } from 'node:fs/promises';
let input = '';
for await (const chunk of process.stdin) input += chunk;
const config = JSON.parse(input);
const browser = await chromium.launch({ headless: true });
try {
  const context = await browser.newContext({ viewport: { width: 1100, height: 850 } });
  await context.addCookies([{ name: 'ajent_human', value: config.session, url: config.origin, httpOnly: true, sameSite: 'Lax' }]);
  const page = await context.newPage();
  let consentPosts = 0;
  page.on('request', request => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === '/oauth/consent') consentPosts++;
  });
  await page.goto(config.authorize, { waitUntil: 'networkidle' });
  const handle = page.getByLabel('New agent handle');
  const approve = page.getByRole('button', { name: 'Connect', exact: true });
  const error = page.locator('#agent-handle-error');
  if (await handle.getAttribute('maxlength') !== '64' || !await approve.isDisabled()) throw new Error('Initial handle rules were not applied');
  for (const value of ['GROKBOT', 'Grok.Bot_7-X', '42', '_A', '.-', 'Z'.repeat(64)]) {
    await handle.fill(value);
    if (!await handle.evaluate(input => input.checkValidity()) || !await approve.isEnabled() || await error.textContent()) throw new Error('Supported handle was rejected');
  }
  for (const value of ['A', 'Grok Bot', 'Grok/Bot', 'Grok@Bot', 'Ågent']) {
    await handle.fill(value);
    if (!await approve.isDisabled() || !await error.textContent() || await handle.getAttribute('aria-invalid') !== 'true') throw new Error('Invalid format lacked live feedback');
  }
  await handle.evaluate(input => { input.value = 'Z'.repeat(65); input.dispatchEvent(new Event('input', { bubbles: true })); });
  if (!await approve.isDisabled() || !await error.textContent()) throw new Error('Overlong handle lacked live feedback');
  await handle.press('Enter');
  if (consentPosts !== 0) throw new Error('Invalid handle was submitted');
  if (!await page.getByRole('button', { name: 'Cancel', exact: true }).evaluate(button => button.formNoValidate)) throw new Error('Invalid input prevents cancellation');
  if (config.output) {
    await mkdir(config.output, { recursive: true });
    await page.screenshot({ path: `${config.output}/consent-invalid-handle.png`, fullPage: true });
  }
  await page.locator('select[name="agent"]').selectOption(config.existingAgent);
  if (!await handle.isDisabled() || !await approve.isEnabled() || await error.textContent()) throw new Error('Stale invalid handle blocks existing-agent selection');
  await page.locator('select[name="agent"]').selectOption('');
  if (!await handle.isEnabled() || !await approve.isDisabled()) throw new Error('New-agent validation was not restored');
  await handle.fill('GROK.Bot_7-X');
  if (config.output) {
    await mkdir(config.output, { recursive: true });
    await page.screenshot({ path: `${config.output}/consent.png`, fullPage: true });
  }
  await Promise.all([
    page.waitForURL(url => url.origin === new URL(config.callback).origin && url.pathname === '/callback', { timeout: 15000 }),
    page.getByRole('button', { name: 'Connect', exact: true }).click(),
  ]);
  if (new URL(page.url()).searchParams.get('state') !== 'browser-state') throw new Error('OAuth state was not preserved');
  const exchange = await context.request.post(`${config.origin}/oauth/token`, { form: {
    grant_type: 'authorization_code', client_id: config.clientId, redirect_uri: config.callback,
    code: new URL(page.url()).searchParams.get('code'), code_verifier: config.verifier,
    resource: `${config.origin}/mcp`,
  } });
  if (exchange.status() !== 200) throw new Error(`Code exchange failed with status ${exchange.status()}`);
  await page.goto(`${config.origin}/human/connections`, { waitUntil: 'networkidle' });
  if (!await page.getByText('GROK.Bot_7-X').count()) throw new Error('Connection identity or letter case missing');
  if (config.output) await page.screenshot({ path: `${config.output}/connections.png`, fullPage: true });
  process.stdout.write(JSON.stringify({ consentRedirect: true, connectionVisible: true, liveHandleValidation: true, handle64Accepted: true, invalidPosts: 0, existingAgentSwitch: true }));
} finally {
  await browser.close();
}
