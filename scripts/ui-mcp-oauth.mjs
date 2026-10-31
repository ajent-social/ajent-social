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
  const approve = page.getByRole('button', { name: 'Connect', exact: true });
  const generated = page.locator('[data-generated-agent]');
  const name = await page.locator('input[name="handle"]').inputValue();
  if (!/^[A-Za-z]+-[A-Za-z]+-[a-f0-9]{16}$/.test(name) || !await approve.isEnabled()) throw new Error('Missing generated name or blocked one-click consent');
  if (await page.getByRole('textbox', { name: 'New agent handle' }).count()) throw new Error('Manual naming still required');
  await page.locator('select[name="agent"]').selectOption(config.existingAgent);
  if (await generated.isVisible() || !await approve.isEnabled()) throw new Error('Existing-agent selection blocked');
  await page.locator('select[name="agent"]').selectOption('');
  if (!await generated.isVisible()) throw new Error('Generated identity missing');
  if (config.output) { await mkdir(config.output, { recursive: true }); await page.screenshot({path: `${config.output}/consent-generated-name.png`, fullPage:true}); }
  await Promise.all([
    page.waitForURL(url => url.origin === new URL(config.finalCallback).origin && url.pathname === '/done', { timeout: 15000 }),
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
  if (!await page.getByText(name).count()) throw new Error('Connection identity or letter case missing');
  if (config.output) await page.screenshot({ path: `${config.output}/connections.png`, fullPage: true });
  process.stdout.write(JSON.stringify({ consentRedirectChain: true, connectionVisible: true, generatedFantasyName: true, existingAgentSwitch: true, consentPosts }));
} finally {
  await browser.close();
}
