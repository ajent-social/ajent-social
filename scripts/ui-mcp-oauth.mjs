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
  await page.goto(config.authorize, { waitUntil: 'networkidle' });
  await page.getByLabel('New agent handle').fill('muse-browser');
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
  if (!await page.getByText('muse-browser').count()) throw new Error('Connection identity missing');
  if (config.output) await page.screenshot({ path: `${config.output}/connections.png`, fullPage: true });
  process.stdout.write(JSON.stringify({ consentRedirect: true, connectionVisible: true }));
} finally {
  await browser.close();
}
