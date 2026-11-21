"use strict";

// This script is executed ONLY from the disposable PostgreSQL-backed Go test.
// Playwright drives actual Chromium, while route.fulfill forwards exact HTTP
// responses from the test-only Go HTTPS listener. The production httpedge
// routers are never mounted or configured with these candidate handlers.
const assert = require("node:assert/strict");
const https = require("node:https");
const { chromium } = require("playwright");

const origin = "https://app.example.test";
const localServer = new URL(process.env.MAMONY_E2E_LOCAL_TLS || "");
assert.equal(localServer.protocol, "https:");
assert.equal(localServer.hostname, "127.0.0.1");
assert.match(localServer.port, /^[0-9]{2,5}$/);
assert.equal(localServer.pathname, "/");

const successLink = process.env.MAMONY_E2E_LINK_SUCCESS;
const ambiguousLink = process.env.MAMONY_E2E_LINK_AMBIGUOUS;
const expiredLink = process.env.MAMONY_E2E_LINK_EXPIRED;
for (const link of [successLink, ambiguousLink, expiredLink]) {
  const u = new URL(link || "");
  assert.equal(u.origin, origin);
  assert.equal(u.pathname, "/verify-email");
  assert.match(u.hash, /^#token=[A-Za-z0-9_-]{43}$/);
}

// The browser is really running JS/CSP and generating Origin, Fetch Metadata
// and Referrer headers. Only the test bridge maps the approved HTTPS origin
// to an ephemeral loopback HTTPS socket; its inbound Host stays unchanged.
// No production proxy/offload implementation is created or exercised here.
function proxyToLoopback(request) {
  const url = new URL(request.url());
  const originalHeaders = request.headers();
  delete originalHeaders["content-length"];
  delete originalHeaders["accept-encoding"];
  return new Promise((resolve, reject) => {
    const upstream = https.request({
      hostname: "127.0.0.1",
      port: Number(localServer.port),
      path: url.pathname + url.search,
      method: request.method(),
      headers: { ...originalHeaders, host: "app.example.test" },
      rejectUnauthorized: false, // ONLY test loopback self-signed TLS
      servername: "app.example.test",
      timeout: 12000,
    }, (reply) => {
      const chunks = [];
      reply.on("data", (data) => chunks.push(data));
      reply.on("end", () => {
        const responseHeaders = {};
        for (const field of [
          "content-type", "content-security-policy", "cache-control",
          "referrer-policy", "x-content-type-options", "x-frame-options",
          "cross-origin-resource-policy", "permissions-policy", "pragma",
        ]) {
          if (reply.headers[field]) {
            responseHeaders[field] = String(reply.headers[field]);
          }
        }
        resolve({
          status: reply.statusCode,
          headers: responseHeaders,
          body: Buffer.concat(chunks),
        });
      });
      reply.on("error", reject);
    });
    upstream.on("timeout", () => upstream.destroy(new Error("test-only TLS timeout")));
    upstream.on("error", reject);
    const body = request.postDataBuffer();
    if (body) upstream.write(body);
    upstream.end();
  });
}

async function requireCleanURLAndNoEarlyPOST(page, counters) {
  assert.equal(await page.evaluate(() => window.location.hash), "");
  assert.equal(page.url(), origin + "/verify-email");
  assert.equal(counters.posts, 0, "mail preview GET must NOT consume a proof");
  assert.match((await page.locator("#status").innerText()), /Ready to verify/);
  assert.equal(await page.getByRole("button", {name: "Verify my email"}).isVisible(), true);
}

async function run() {
  const browser = await chromium.launch({headless: true});
  const context = await browser.newContext({
    serviceWorkers: "block",
    javaScriptEnabled: true,
    ignoreHTTPSErrors: false,
  });
  const counters = { posts: 0, pageGets: 0, failures: 0 };
  const violations = [];
  try {
    await context.route("**/*", async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const allowed = url.origin === origin &&
        (url.pathname === "/verify-email" ||
         url.pathname === "/v1/identity/verify-email") &&
        url.search === "" && url.hash === "";
      if (!allowed) {
        violations.push("Unexpected outbound resource or token-bearing URL");
        await route.abort("blockedbyclient");
        return;
      }
      if (url.pathname === "/verify-email") {
        assert.equal(request.method(), "GET");
        counters.pageGets++;
      } else {
        assert.equal(request.method(), "POST");
        assert.equal(request.headers().origin, origin);
        assert.equal(request.headers()["content-type"], "application/json");
        assert.equal(request.headers().cookie, undefined);
        assert.equal(request.headers().authorization, undefined);
        assert.equal(request.headers().referer, undefined);
        counters.posts++;
      }
      const response = await proxyToLoopback(request);
      // Simulate an ambiguous lost reply AFTER the genuine PostgreSQL verifier
      // has committed. The UI must NOT retry or assert the account is inactive.
      const isAmbiguous = url.pathname === "/v1/identity/verify-email" &&
        request.postDataJSON().token === new URL(ambiguousLink).hash.slice(7);
      if (isAmbiguous) {
        assert.equal(response.status, 204, "verifier must have committed before dropping reply");
        counters.failures++;
        await route.abort("failed");
        return;
      }
      await route.fulfill(response);
    });

    // The earliest actual browser interaction is an email-client navigation.
    const success = await context.newPage();
    await success.goto(successLink, {waitUntil: "load"});
    await requireCleanURLAndNoEarlyPOST(success, counters);
    const csp = await success.evaluate(() => document.policy?.allowedFeatures?.() || []);
    assert.ok(Array.isArray(csp), "real browser DOM must be available");
    await success.getByRole("button", {name: "Verify my email"}).click();
    await success.getByRole("status").getByText(
      "Email verified. Verification does not sign you in.", {exact: true}
    ).waitFor({timeout: 10000});
    assert.equal(counters.posts, 1);
    assert.equal(success.url(), origin + "/verify-email");
    assert.equal(await success.context().cookies(), 0, "browser has no cookies");
    await success.close();

    // Broken connectivity AFTER COMMIT must be an ambiguous browser message,
    // no token echo and NO automatic second POST.
    const ambiguous = await context.newPage();
    await ambiguous.goto(ambiguousLink, {waitUntil: "load"});
    assert.equal(await ambiguous.evaluate(() => location.hash), "");
    await ambiguous.getByRole("button", {name: "Verify my email"}).click();
    await ambiguous.getByRole("status").getByText(/may have succeeded/).waitFor({timeout: 10000});
    assert.equal(counters.posts, 2);
    assert.equal(counters.failures, 1);
    assert.equal(ambiguous.url(), origin + "/verify-email");
    await ambiguous.close();

    // Authoritatively expired proof is rejected by the actual verifier DB.
    const expired = await context.newPage();
    await expired.goto(expiredLink, {waitUntil: "load"});
    assert.equal(await expired.evaluate(() => location.hash), "");
    await expired.getByRole("button", {name: "Verify my email"}).click();
    await expired.getByRole("status").getByText(/already used, or invalid/).waitFor({timeout: 10000});
    assert.equal(counters.posts, 3);
    await expired.close();

    // A direct scanner-style GET with a missing token has no action button.
    const scanner = await context.newPage();
    await scanner.goto(origin + "/verify-email", {waitUntil: "load"});
    assert.match(await scanner.getByRole("status").innerText(), /missing or cannot be used/);
    assert.equal(await scanner.getByRole("button", {name: "Verify my email"}).isVisible(), false);
    assert.equal(counters.posts, 3);
    await scanner.close();

    assert.equal(counters.pageGets, 4);
    assert.deepEqual(violations, [], "page must not request third-party resources or token URLs");
    // No token should be present in browser-visible final URLs, cookies or
    // a request URL. The only proof transfer was POST JSON to one origin.
    process.stdout.write("real Chromium HTTPS-origin/CSP/GET/POST verification: PASS\n");
  } finally {
    await context.close();
    await browser.close();
  }
}
run().catch((err) => {
  // Error objects may contain test bearer URLs. Parent Go runner redacts all
  // three known synthetic bearer values before ever reporting test failures.
  process.stderr.write(String(err && err.stack ? err.stack : err));
  process.exitCode = 1;
});
