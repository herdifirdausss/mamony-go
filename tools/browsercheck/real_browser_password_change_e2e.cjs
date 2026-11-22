"use strict";

// Isolated Chromium bridge for the stacked, UNMOUNTED password-change HTTP
// candidate. Only one disposable 127.0.0.1 TLS server is reachable.
const assert = require("node:assert/strict");
const https = require("node:https");
const {chromium} = require("playwright");

const origin = "https://app.example.test";
const siblingOrigin = "https://sibling.example.test";
const attackerOrigin = "https://attacker.other.test";
const local = new URL(process.env.MAMONY_ROTATION_E2E_LOCAL_TLS || "");
assert.equal(local.protocol, "https:");
assert.equal(local.hostname, "127.0.0.1");
assert.match(local.port, /^[0-9]{2,5}$/);
assert.equal(local.pathname, "/");
const a = {
  email: process.env.MAMONY_ROTATION_E2E_EMAIL_A,
  old: process.env.MAMONY_ROTATION_E2E_OLD_A,
  next: process.env.MAMONY_ROTATION_E2E_NEXT_A,
};
const b = {
  email: process.env.MAMONY_ROTATION_E2E_EMAIL_B,
  old: process.env.MAMONY_ROTATION_E2E_OLD_B,
  next: process.env.MAMONY_ROTATION_E2E_NEXT_B,
};
for (const account of [a, b]) {
  assert.match(account.email || "", /^[a-z0-9_-]+@example[.]test$/);
  assert.ok(account.old && account.old.length >= 20);
  assert.ok(account.next && account.next.length >= 20);
}

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
      rejectUnauthorized: false, // only this synthetic loopback TLS server
      servername: "app.example.test",
      timeout: 15000,
    }, (response) => {
      const chunks = [];
      response.on("data", (chunk) => chunks.push(chunk));
      response.on("end", () => {
        const allowedHeaders = {};
        for (const name of ["content-type", "content-security-policy",
          "cache-control", "referrer-policy", "x-content-type-options", "pragma"]) {
          if (response.headers[name]) allowedHeaders[name] = String(response.headers[name]);
        }
        const cookies = response.headers["set-cookie"] || [];
        assert.ok(cookies.length <= 1, "multiple Set-Cookie values are forbidden");
        if (cookies.length === 1) allowedHeaders["set-cookie"] = cookies[0];
        resolve({status: response.statusCode, headers: allowedHeaders,
          body: Buffer.concat(chunks)});
      });
      response.on("error", reject);
    });
    upstream.on("timeout", () => upstream.destroy(new Error("isolated HTTPS timeout")));
    upstream.on("error", reject);
    const body = request.postDataBuffer();
    if (body) upstream.write(body);
    upstream.end();
  });
}

async function perform(page, path, init) {
  return page.evaluate(async ({path, init}) => {
    const response = await fetch(path, init);
    return {status: response.status, body: await response.text(),
      cookieVisible: response.headers.get("set-cookie")};
  }, {path, init});
}

function form(account) {
  return JSON.stringify({email: account.email, currentPassword: account.old,
    nextPassword: account.next});
}

function secureCookie(cookies) {
  assert.equal(cookies.length, 1, "a browser must hold exactly one opaque cookie");
  const cookie = cookies[0];
  assert.equal(cookie.name, "__Host-mamony_sid");
  assert.equal(cookie.domain, "app.example.test");
  assert.equal(cookie.path, "/");
  assert.equal(cookie.httpOnly, true);
  assert.equal(cookie.secure, true);
  assert.equal(cookie.sameSite, "Strict");
  assert.match(cookie.value, /^[A-Za-z0-9_-]{43}$/);
}

async function login(page, account, secret, want) {
  const result = await perform(page, "/v1/identity/login", {
    method: "POST", headers: {"Content-Type": "application/json"},
    body: JSON.stringify({email: account.email, password: secret}),
  });
  assert.equal(result.status, want);
  assert.equal(result.cookieVisible, null);
  return result;
}

async function csrf(page) {
  const result = await perform(page, "/v1/identity/csrf");
  assert.equal(result.status, 200);
  assert.equal(result.cookieVisible, null);
  const value = JSON.parse(result.body).csrfToken;
  assert.match(value, /^[A-Za-z0-9_-]{54}$/);
  return value;
}

async function change(page, account, token, body) {
  return perform(page, "/v1/identity/change-password", {
    method: "POST",
    headers: {"Content-Type": "application/json", "X-Mamony-CSRF": token},
    body: body === undefined ? form(account) : body,
  });
}

async function run() {
  const browser = await chromium.launch({headless: true});
  const contexts = [];
  const counters = {dropped: 0, sibling: 0, crossSite: 0, changes: 0};
  const unexpected = [];
  let dropNextChange = false;

  async function createContext() {
    const context = await browser.newContext({
      serviceWorkers: "block", javaScriptEnabled: true, ignoreHTTPSErrors: false,
    });
    contexts.push(context);
    await context.route("**/*", async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      if ((url.origin === siblingOrigin || url.origin === attackerOrigin) &&
          url.pathname === "/") {
        await route.fulfill({status: 200, contentType: "text/html",
          body: "<!doctype html><title>Isolated attacker origin</title>"});
        return;
      }
      const allowed = url.origin === origin && url.search === "" && url.hash === "" &&
        ["/__test__/password-browser", "/v1/identity/login",
          "/v1/identity/session", "/v1/identity/csrf",
          "/v1/identity/change-password", "/v1/identity/logout"].includes(url.pathname);
      if (!allowed) {
        unexpected.push("unexpected outbound URL");
        await route.abort("blockedbyclient");
        return;
      }
      const headers = request.headers();
      if (url.pathname === "/v1/identity/change-password") {
        assert.equal(request.method(), "POST");
        counters.changes++;
        if (headers.origin === siblingOrigin) {
          assert.ok(headers.cookie?.startsWith("__Host-mamony_sid="),
            "same-site sibling should send Strict cookie; server must reject Origin");
          counters.sibling++;
        } else if (headers.origin === attackerOrigin) {
          assert.ok(!headers.cookie,
            "cross-site attacker must not send a SameSite=Strict session cookie");
          counters.crossSite++;
        } else {
          assert.equal(headers.origin, origin);
        }
      }
      const response = await forward(request);
      if (headers.origin === siblingOrigin || headers.origin === attackerOrigin) {
        assert.equal(response.status, 403, "server must deny cross-origin mutation");
      }
      if (dropNextChange && url.pathname === "/v1/identity/change-password" &&
          headers.origin === origin) {
        dropNextChange = false;
        assert.equal(response.status, 204, "commit must complete BEFORE dropped response");
        counters.dropped++;
        await route.abort("failed");
        return;
      }
      await route.fulfill(response);
    });
    const page = await context.newPage();
    await page.goto(origin + "/__test__/password-browser", {waitUntil: "load"});
    assert.equal(await page.evaluate(() => document.cookie), "");
    return {context, page};
  }

  try {
    const first = await createContext();
    const second = await createContext();
    const other = await createContext();

    // Same user, two genuinely separate browser cookie jars.
    await login(first.page, a, a.old, 204);
    await login(second.page, a, a.old, 204);
    secureCookie(await first.context.cookies(origin));
    secureCookie(await second.context.cookies(origin));
    assert.equal((await perform(first.page, "/v1/identity/session")).status, 200);
    assert.equal((await perform(second.page, "/v1/identity/session")).status, 200);
    const token = await csrf(first.page);
    const staleToken = await csrf(second.page);

    // No browser bypass may spend the Argon2id gate or change a credential.
    let r = await perform(first.page, "/v1/identity/change-password", {
      method: "POST", headers: {"Content-Type": "application/json"}, body: form(a),
    });
    assert.equal(r.status, 403, "missing CSRF must be denied");
    r = await change(first.page, a, token + "x");
    assert.equal(r.status, 403, "tampered CSRF must be denied");
    r = await change(first.page, a, token,
      '{"email":"' + a.email + '","email":"' + a.email +
      '","currentPassword":"' + a.old + '","nextPassword":"' + a.next + '"}');
    assert.equal(r.status, 400, "duplicate JSON keys must be denied");
    r = await change(first.page, a, token,
      JSON.stringify({email: a.email, currentPassword: a.old,
        nextPassword: "a".repeat(5000)}));
    assert.equal(r.status, 413, "oversized JSON must be refused");
    assert.equal((await perform(second.page, "/v1/identity/session")).status, 200);

    // A correct password for account B MUST NOT rotate the session of A.
    r = await change(first.page, b, token);
    assert.equal(r.status, 401, "a second account's password is not session reauth");
    assert.equal((await perform(first.page, "/v1/identity/session")).status, 200);

    // Same-site sibling sends a Strict cookie: only exact Origin forbids it.
    const sibling = await first.context.newPage();
    await sibling.goto(siblingOrigin + "/");
    const siblingResult = await sibling.evaluate(async (target) => {
      try {
        const response = await fetch(target, {method: "POST", credentials: "include",
          headers: {"Content-Type": "text/plain"}, body: "{}"});
        return response.status;
      } catch { return "cors-blocked"; }
    }, origin + "/v1/identity/change-password");
    assert.ok(siblingResult === 403 || siblingResult === "cors-blocked");
    await sibling.close();
    assert.equal(counters.sibling, 1);

    const evil = await first.context.newPage();
    await evil.goto(attackerOrigin + "/");
    const crossSiteResult = await evil.evaluate(async (target) => {
      try {
        const response = await fetch(target, {method: "POST", credentials: "include",
          headers: {"Content-Type": "text/plain"}, body: "{}"});
        return response.status;
      } catch { return "cors-blocked"; }
    }, origin + "/v1/identity/change-password");
    assert.ok(crossSiteResult === 403 || crossSiteResult === "cors-blocked");
    await evil.close();
    assert.equal(counters.crossSite, 1);

    // The certain success deletes only the current browser cookie, while
    // PostgreSQL epoch invalidation also revokes the OTHER browser's bearer.
    r = await change(first.page, a, token);
    assert.equal(r.status, 204);
    assert.equal(r.body, "");
    assert.equal(r.cookieVisible, null);
    assert.equal((await first.context.cookies(origin)).length, 0);
    assert.equal((await first.page.evaluate(() => document.cookie)), "");
    assert.equal((await perform(second.page, "/v1/identity/session")).status, 401);
    secureCookie(await second.context.cookies(origin));
    r = await change(second.page, a, staleToken);
    assert.equal(r.status, 401, "old browser session cannot rotate again");

    await login(first.page, a, a.old, 401);
    await login(first.page, a, a.next, 204);
    secureCookie(await first.context.cookies(origin));
    assert.equal((await perform(first.page, "/v1/identity/session")).status, 200);

    // Separate account B. After the response is dropped, Chromium retains the
    // cookie, but PostgreSQL has already atomically invalidated that session.
    await login(other.page, b, b.old, 204);
    secureCookie(await other.context.cookies(origin));
    const otherCSRF = await csrf(other.page);
    dropNextChange = true;
    const lost = await other.page.evaluate(async ({body, token}) => {
      try {
        await fetch("/v1/identity/change-password", {
          method: "POST", headers: {"Content-Type": "application/json",
            "X-Mamony-CSRF": token}, body,
        });
        return false;
      } catch { return true; }
    }, {body: form(b), token: otherCSRF});
    assert.equal(lost, true, "lost response must be uncertain in the browser");
    assert.equal(counters.dropped, 1, "exactly one response was lost");
    secureCookie(await other.context.cookies(origin));
    assert.equal((await perform(other.page, "/v1/identity/session")).status, 401);
    await other.context.clearCookies(); // test-only acknowledgement of stale cookie
    await login(other.page, b, b.old, 401);
    await login(other.page, b, b.next, 204);
    secureCookie(await other.context.cookies(origin));
    assert.equal((await perform(other.page, "/v1/identity/session")).status, 200);
    assert.equal(dropNextChange, false);
    assert.deepEqual(unexpected, []);
    process.stdout.write("real Chromium password-change/epoch/notices/lost-response: PASS\n");
  } finally {
    for (const context of contexts) await context.close();
    await browser.close();
  }
}

run().catch((err) => {
  let failure = String(err && err.stack ? err.stack : err);
  for (const account of [a, b]) {
    for (const value of [account.email, account.old, account.next]) {
      failure = failure.replaceAll(value || "\0", "[REDACTED_FIXTURE]");
    }
  }
  failure = failure.replace(/__Host-mamony_sid=[A-Za-z0-9_-]{43}/g,
    "[REDACTED_SESSION_COOKIE]");
  failure = failure.replace(/\b[A-Za-z0-9_-]{54}\b/g, "[REDACTED_CSRF]");
  process.stderr.write(failure);
  process.exitCode = 1;
});
