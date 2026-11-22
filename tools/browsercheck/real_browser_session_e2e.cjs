"use strict";

// Test-only Chromium bridge: preserve the browser's app.example.test origin
// while forwarding to one disposable HTTPS/Go/PostgreSQL listener. No active
// application router or production proxy is involved.
const assert = require("node:assert/strict");
const https = require("node:https");
const {chromium} = require("playwright");

const origin = "https://app.example.test";
const attackerOrigin = "https://attacker.other.test";
const siblingOrigin = "https://attacker.example.test";
const local = new URL(process.env.MAMONY_SESSION_E2E_LOCAL_TLS || "");
assert.equal(local.protocol, "https:");
assert.equal(local.hostname, "127.0.0.1");
assert.match(local.port, /^[0-9]{2,5}$/);
assert.equal(local.pathname, "/");
const email = process.env.MAMONY_SESSION_E2E_EMAIL;
const password = process.env.MAMONY_SESSION_E2E_PASSWORD;
assert.match(email || "", /^[a-z0-9_-]+@example[.]test$/);
assert.ok(password && password.length >= 15);

function forward(request) {
  const url = new URL(request.url());
  const headers = request.headers();
  delete headers["content-length"];
  delete headers["accept-encoding"];
  return new Promise((resolve, reject) => {
    const upstream = https.request({
      hostname: "127.0.0.1",
      port: Number(local.port),
      path: url.pathname,
      method: request.method(),
      headers: {...headers, host: "app.example.test"},
      rejectUnauthorized: false, // disposable loopback self-signed TLS ONLY
      servername: "app.example.test",
      timeout: 12000,
    }, (reply) => {
      const chunks = [];
      reply.on("data", (chunk) => chunks.push(chunk));
      reply.on("end", () => {
        const allowedHeaders = {};
        for (const name of [
          "content-type", "content-security-policy", "cache-control",
          "referrer-policy", "x-content-type-options", "pragma",
        ]) {
          if (reply.headers[name]) allowedHeaders[name] = String(reply.headers[name]);
        }
        // Unlike email-verification E2E, cookie acceptance is under test.
        // Let Chromium, NOT Node or Playwright's APIRequestContext, decide.
        const cookies = reply.headers["set-cookie"] || [];
        assert.ok(cookies.length <= 1, "unexpected multiple Set-Cookie headers");
        if (cookies.length === 1) allowedHeaders["set-cookie"] = cookies[0];
        resolve({status: reply.statusCode, headers: allowedHeaders, body: Buffer.concat(chunks)});
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

async function perform(page, path, init) {
  return page.evaluate(async ({path, init}) => {
    const response = await fetch(path, init);
    return {
      status: response.status,
      contentType: response.headers.get("content-type"),
      setCookieVisible: response.headers.get("set-cookie"),
      body: await response.text(),
    };
  }, {path, init});
}

function expectSafeSessionCookie(cookies) {
  assert.equal(cookies.length, 1, "real Chromium must store exactly one host-only cookie");
  const cookie = cookies[0];
  assert.equal(cookie.name, "__Host-mamony_sid");
  assert.equal(cookie.domain, "app.example.test");
  assert.equal(cookie.path, "/");
  assert.equal(cookie.secure, true);
  assert.equal(cookie.httpOnly, true);
  assert.equal(cookie.sameSite, "Strict");
  assert.match(cookie.value, /^[A-Za-z0-9_-]{43}$/);
}

async function run() {
  const browser = await chromium.launch({headless: true});
  const context = await browser.newContext({
    serviceWorkers: "block", javaScriptEnabled: true, ignoreHTTPSErrors: false,
  });
  const counters = {login: 0, logout: 0, deniedCrossSite: 0, deniedSibling: 0, dropped: 0};
  const violations = [];
  let dropNextLogout = false;
  try {
    await context.route("**/*", async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      if ((url.origin === attackerOrigin || url.origin === siblingOrigin) && url.pathname === "/") {
        await route.fulfill({status: 200, contentType: "text/html",
          body: "<!doctype html><title>Cross-site test</title>"});
        return;
      }
      const allowed = url.origin === origin && url.search === "" && url.hash === "" &&
        ["/__test__/session-browser", "/v1/identity/login", "/v1/identity/session",
          "/v1/identity/csrf", "/v1/identity/logout"].includes(url.pathname);
      if (!allowed) {
        violations.push("unexpected outbound browser URL");
        await route.abort("blockedbyclient");
        return;
      }
      if (url.pathname === "/__test__/session-browser") {
        assert.equal(request.method(), "GET");
      } else if (url.pathname === "/v1/identity/login") {
        assert.equal(request.method(), "POST");
        assert.equal(request.headers().origin, origin);
        if (request.headers()["sec-fetch-site"] !== undefined) {
          assert.equal(request.headers()["sec-fetch-site"], "same-origin");
        }
        counters.login++;
      }
      if (url.pathname === "/v1/identity/logout") {
        assert.equal(request.method(), "POST");
        counters.logout++;
        if (request.headers().origin === attackerOrigin) {
          assert.ok(!request.headers().cookie,
            "SameSite=Strict must not send session to a cross-site POST");
          counters.deniedCrossSite++;
        } else if (request.headers().origin === siblingOrigin) {
          assert.ok(request.headers().cookie?.startsWith("__Host-mamony_sid="),
            "a same-site sibling may carry Strict cookies to the victim host");
          counters.deniedSibling++;
        } else {
          assert.equal(request.headers().origin, origin);
          if (request.headers()["sec-fetch-site"] !== undefined) {
          assert.equal(request.headers()["sec-fetch-site"], "same-origin");
        }
        }
      }
      const response = await forward(request);
      if (url.pathname === "/v1/identity/logout" &&
          (request.headers().origin === attackerOrigin ||
           request.headers().origin === siblingOrigin)) {
        assert.equal(response.status, 403, "cross-origin POST must be rejected");
      }
      if (dropNextLogout && url.pathname === "/v1/identity/logout" &&
          request.headers().origin === origin) {
        dropNextLogout = false;
        assert.equal(response.status, 204, "revoke must commit before lost response");
        counters.dropped++;
        await route.abort("failed");
        return;
      }
      await route.fulfill(response);
    });

    const page = await context.newPage();
    await page.goto(origin + "/__test__/session-browser", {waitUntil: "load"});
    assert.equal(await page.evaluate(() => document.cookie), "");

    let reply = await perform(page, "/v1/identity/session");
    assert.equal(reply.status, 401, "unauthenticated session check must fail");
    reply = await perform(page, "/v1/identity/login", {
      method: "POST", headers: {"Content-Type": "application/json"},
      body: JSON.stringify({email, password: password + "-incorrect"}),
    });
    assert.equal(reply.status, 401, "wrong password must not mint cookie");
    assert.equal((await context.cookies(origin)).length, 0);

    reply = await perform(page, "/v1/identity/login", {
      method: "POST", headers: {"Content-Type": "application/json"},
      body: JSON.stringify({email, password}),
    });
    assert.equal(reply.status, 204, "real password proof + session commit must succeed");
    assert.equal(reply.setCookieVisible, null, "fetch cannot read Set-Cookie");
    assert.equal(reply.body, "");
    expectSafeSessionCookie(await context.cookies(origin));
    assert.equal(await page.evaluate(() => document.cookie), "", "HttpOnly token reached JS");

    reply = await perform(page, "/v1/identity/session");
    assert.equal(reply.status, 200);
    assert.deepEqual(JSON.parse(reply.body), {authenticated: true});
    assert.equal(reply.setCookieVisible, null);
    reply = await perform(page, "/v1/identity/csrf");
    assert.equal(reply.status, 200);
    const firstCSRF = JSON.parse(reply.body).csrfToken;
    assert.match(firstCSRF, /^[A-Za-z0-9_-]{54}$/);
    assert.equal(reply.setCookieVisible, null);

    // Cross-origin javascript must not revoke a session or read a response.
    const evil = await context.newPage();
    await evil.goto(attackerOrigin + "/");
    const crossSiteOutcome = await evil.evaluate(async (destination) => {
      try {
        const response = await fetch(destination, {method: "POST", credentials: "include",
          headers: {"Content-Type": "text/plain"}, body: ""});
        return response.status;
      } catch {
        return "cors-blocked";
      }
    }, origin + "/v1/identity/logout");
    // Playwright route.fulfill may bypass the browser's native CORS response
    // filtering. This bridge asserts the authoritative server returned 403;
    // it deliberately does NOT claim production CORS behavior is proven.
    assert.ok(crossSiteOutcome === 403 || crossSiteOutcome === "cors-blocked",
      "cross-site POST must be rejected by the server");
    assert.equal(counters.deniedCrossSite, 1);
    await evil.close();

    // SameSite=Strict is NOT a same-origin policy: a sibling subdomain is
    // same-site and can send the cookie. Only the exact server Origin check
    // prevents a cross-origin sibling from revoking the victim's session.
    const sibling = await context.newPage();
    await sibling.goto(siblingOrigin + "/");
    const siblingOutcome = await sibling.evaluate(async (destination) => {
      try {
        const response = await fetch(destination, {method: "POST", credentials: "include",
          headers: {"Content-Type": "text/plain"}, body: ""});
        return response.status;
      } catch {
        return "cors-blocked";
      }
    }, origin + "/v1/identity/logout");
    assert.ok(siblingOutcome === 403 || siblingOutcome === "cors-blocked",
      "same-site cross-origin POST must be denied by Origin policy");
    assert.equal(counters.deniedSibling, 1);
    await sibling.close();
    assert.equal((await perform(page, "/v1/identity/session")).status, 200);

    reply = await perform(page, "/v1/identity/logout", {method: "POST"});
    assert.equal(reply.status, 403, "no CSRF cannot logout");
    reply = await perform(page, "/v1/identity/logout", {
      method: "POST", headers: {"X-Mamony-CSRF": firstCSRF + "x"},
    });
    assert.equal(reply.status, 403, "tampered CSRF cannot logout");
    assert.equal((await perform(page, "/v1/identity/session")).status, 200);

    reply = await perform(page, "/v1/identity/logout", {
      method: "POST", headers: {"X-Mamony-CSRF": firstCSRF},
    });
    assert.equal(reply.status, 204, "confirmed logout must revoke and delete cookie");
    assert.equal(reply.setCookieVisible, null);
    assert.equal((await context.cookies(origin)).length, 0, "logout cookie deletion failed");
    assert.equal((await perform(page, "/v1/identity/session")).status, 401);

    // Re-login and reject a CSRF token bound to the old session.
    reply = await perform(page, "/v1/identity/login", {
      method: "POST", headers: {"Content-Type": "application/json"},
      body: JSON.stringify({email, password}),
    });
    assert.equal(reply.status, 204);
    expectSafeSessionCookie(await context.cookies(origin));
    assert.equal((await perform(page, "/v1/identity/logout", {
      method: "POST", headers: {"X-Mamony-CSRF": firstCSRF},
    })).status, 403, "CSRF from former session cannot revoke a new one");

    const fresh = await perform(page, "/v1/identity/csrf");
    assert.equal(fresh.status, 200);
    const secondCSRF = JSON.parse(fresh.body).csrfToken;
    dropNextLogout = true;
    const ambiguous = await page.evaluate(async (token) => {
      try {
        await fetch("/v1/identity/logout", {
          method: "POST", headers: {"X-Mamony-CSRF": token},
        });
        return false;
      } catch {
        return true;
      }
    }, secondCSRF);
    assert.equal(ambiguous, true, "the user must see uncertain transport outcome");
    assert.equal(counters.dropped, 1);
    // The response never reached Chromium: the cookie remains, but the DB
    // authority must reject it; a UI must not claim definite logout success.
    expectSafeSessionCookie(await context.cookies(origin));
    assert.equal((await perform(page, "/v1/identity/session")).status, 401);
    assert.equal(counters.login, 3, "unexpected password retry");
    assert.equal(counters.logout, 7, "unexpected automatic logout replay");
    assert.deepEqual(violations, []);
    process.stdout.write("real Chromium cookie/CSRF/logout/uncertain-revoke: PASS\n");
  } finally {
    await context.close();
    await browser.close();
  }
}

run().catch((err) => {
  let failure = String(err && err.stack ? err.stack : err);
  // Avoid printing synthetic password in an assertion or test failure.
  failure = failure.replaceAll(password || "", "[REDACTED_PASSWORD]");
  process.stderr.write(failure);
  process.exitCode = 1;
});
