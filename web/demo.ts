// Web Dashboard Demo — records a short video: Overview → Agents → Container (db) → Logs.
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
  await sleep(2500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, "website/public/dashboard/web-overview.png") });

  // --- Agents ---
  await page.click('a[href="/agents"]');
  await page.waitForSelector("table");
  await sleep(2000);

  // --- Containers → click the db container ---
  await page.click('a[href="/containers"]');
  await page.waitForSelector("table");
  await sleep(1500);

  // Find and click the row containing "db" in the container name
  const dbRow = page.locator("tbody tr", { hasText: "db" }).first();
  if (await dbRow.count()) {
    await dbRow.click();
    await sleep(2000);

    // Expand the inline log panel
    const logPanelHeader = page.locator(".panel-header", { hasText: "Recent Logs" });
    if (await logPanelHeader.count()) {
      await logPanelHeader.click();
      await sleep(2000);
    }

    // Navigate to full logs page via the Logs button
    const logsBtn = page.locator("button", { hasText: "Logs" });
    if (await logsBtn.count()) {
      await logsBtn.click();
      await sleep(3000);
    }
  } else {
    // Fallback: go to logs page directly
    await page.click('a[href="/logs"]');
    await sleep(3000);
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
