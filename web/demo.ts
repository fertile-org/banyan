// Web Dashboard Demo — records a video navigating through all pages.
// Run with: make demo-web
// Requires: npx playwright install chromium

import { chromium } from "playwright";
import path from "path";

const DASHBOARD_URL = process.env.DASHBOARD_URL || "http://localhost:3000";
const OUTPUT_DIR = path.resolve(process.cwd(), "..");

async function sleep(ms: number) {
  return new Promise((r) => setTimeout(r, ms));
}

async function main() {
  console.log(`Recording web dashboard demo from ${DASHBOARD_URL}...`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1920, height: 1080 },
    recordVideo: {
      dir: OUTPUT_DIR,
      size: { width: 1920, height: 1080 },
    },
    colorScheme: "dark",
  });

  const page = await context.newPage();

  // --- Overview ---
  await page.goto(DASHBOARD_URL);
  await page.waitForSelector(".stat-grid", { timeout: 10000 });
  await sleep(2000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, "website/public/dashboard/web-overview.png") });

  // --- Agents ---
  await page.click('a[href="/agents"]');
  await page.waitForSelector("table");
  await sleep(1500);

  // Click first agent row
  const agentRow = page.locator("tbody tr").first();
  if (await agentRow.count()) {
    await agentRow.click();
    await sleep(1500);
    await page.goBack();
    await sleep(500);
  }

  // --- Deployments ---
  await page.click('a[href="/deployments"]');
  await page.waitForSelector("table");
  await sleep(1500);

  // Click first deployment
  const deployRow = page.locator("tbody tr").first();
  if (await deployRow.count()) {
    await deployRow.click();
    await sleep(2000);
    await page.goBack();
    await sleep(500);
  }

  // --- Containers ---
  await page.click('a[href="/containers"]');
  await page.waitForSelector("table");
  await sleep(1500);

  // Use filter
  const filterInput = page.locator('input[placeholder*="Filter"]');
  if (await filterInput.count()) {
    await filterInput.fill("api");
    await sleep(1000);
    await filterInput.fill("");
    await sleep(500);
  }

  // Click first container for detail
  const containerRow = page.locator("tbody tr").first();
  if (await containerRow.count()) {
    await containerRow.click();
    await sleep(2000);
    await page.goBack();
    await sleep(500);
  }

  // --- Engine ---
  await page.click('a[href="/engine"]');
  await sleep(1500);

  // --- Events ---
  await page.click('a[href="/events"]');
  await sleep(1500);

  // --- Logs ---
  await page.click('a[href="/logs"]');
  await sleep(1000);

  // Select first container in dropdown
  const logSelect = page.locator("select").first();
  if (await logSelect.count()) {
    const options = await logSelect.locator("option").allTextContents();
    const containerOption = options.find((o) => o !== "Select a container...");
    if (containerOption) {
      await logSelect.selectOption({ label: containerOption });
      await sleep(3000);
    }
  }

  // --- Command Palette ---
  await page.keyboard.press("Control+k");
  await sleep(1000);
  await page.keyboard.type("over", { delay: 100 });
  await sleep(800);
  await page.keyboard.press("Enter");
  await sleep(1500);

  // --- Toggle theme ---
  const themeBtn = page.locator('button[title="Toggle theme"]');
  if (await themeBtn.count()) {
    await themeBtn.click();
    await sleep(1500);
    await themeBtn.click();
    await sleep(1000);
  }

  // Done
  await page.close();
  await context.close();
  await browser.close();

  console.log("Demo recorded. Outputs:");
  console.log(`  Screenshot: website/public/dashboard/web-overview.png`);
  console.log(`  Video: check output directory for .webm file`);
}

main().catch((err) => {
  console.error("Demo failed:", err);
  process.exit(1);
});
