"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

// No browser, npm dependency or live network is necessary. Execute the EXACT
// JavaScript embedded by the Go handler in a minimal deterministic DOM/Fetch
// sandbox. This tests behavior, not just static source-string assertions.
const pageJS = fs.readFileSync(
  path.join(__dirname, "../../internal/modules/identity/internal/handler/email_verification_page.js"),
  "utf8"
);
const token = Buffer.alloc(32, 0x3c).toString("base64url");
assert.equal(token.length, 43);

function startPage(fragment, options = {}) {
  const status = { textContent: "Checking verification link..." };
  const button = {
    hidden: true,
    disabled: false,
    clickHandler: undefined,
    addEventListener(event, handler) {
      if (event !== "click" || this.clickHandler) {
        throw new Error("unexpected button listener");
      }
      this.clickHandler = handler;
    },
  };
  const callbacks = new Map();
  const calls = [];
  let scrubCalls = 0;
  const browser = {
    location: { hash: fragment },
    history: {
      replaceState(_state, _title, nextURL) {
        scrubCalls++;
        if (options.scrubFails) {
          throw new Error("synthetic history failure");
        }
        assert.equal(nextURL, "/verify-email");
        browser.location.hash = "";
      },
    },
    addEventListener(event, handler) {
      if (callbacks.has(event)) {
        throw new Error("duplicate page event");
      }
      callbacks.set(event, handler);
    },
  };
  const document = {
    getElementById(id) {
      if (id === "status") return status;
      if (id === "confirm") return button;
      throw new Error("unexpected DOM access");
    },
  };
  const mockFetch = (url, init) => {
    calls.push({ url, init });
    if (options.networkFailure) {
      return Promise.reject(new Error("synthetic network failure with secret"));
    }
    return Promise.resolve({ status: options.httpStatus ?? 204 });
  };
  vm.runInNewContext(pageJS, { window: browser, document, fetch: mockFetch }, {
    timeout: 1000,
    filename: "email_verification_page.js",
  });
  return {
    browser, button, status, calls,
    get scrubCalls() { return scrubCalls; },
    ready() {
      const callback = callbacks.get("DOMContentLoaded");
      assert.equal(typeof callback, "function");
      callback();
    },
    click() {
      assert.equal(typeof button.clickHandler, "function");
      button.clickHandler();
    },
  };
}

async function drainPromises() {
  // Let the VM's same-origin fetch promise and its callbacks complete.
  await new Promise((resolve) => setImmediate(resolve));
}

test("GET preview scrubs the fragment early and NEVER consumes before explicit click", async () => {
  const page = startPage("#token=" + token);
  assert.equal(page.scrubCalls, 1);
  assert.equal(page.browser.location.hash, "");
  assert.equal(page.calls.length, 0);
  page.ready();
  assert.equal(page.calls.length, 0);
  assert.equal(page.button.hidden, false);
  assert.match(page.status.textContent, /Ready to verify/);

  page.click();
  assert.equal(page.calls.length, 1);
  assert.equal(page.button.hidden, true);
  assert.equal(page.button.disabled, true);
  page.click();
  assert.equal(page.calls.length, 1, "never submit a token twice on the same page");

  const [request] = page.calls;
  assert.equal(request.url, "/v1/identity/verify-email");
  assert.equal(request.init.method, "POST");
  assert.equal(request.init.headers["Content-Type"], "application/json");
  assert.equal(request.init.credentials, "omit");
  assert.equal(request.init.redirect, "error");
  assert.equal(request.init.mode, "same-origin");
  assert.equal(request.init.referrerPolicy, "no-referrer");
  assert.equal(request.init.cache, "no-store");
  assert.deepEqual(JSON.parse(request.init.body), { token });
  assert.ok(!request.url.includes(token));
  await drainPromises();
  assert.equal(page.status.textContent, "Email verified. Verification does not sign you in.");
  assert.ok(!page.status.textContent.includes(token));
});

test("malformed, missing, query-like, extra and encoded fragments never reach fetch", () => {
  const fragments = [
    "", "#", "#token=", "#token=" + token + "&tracking=1",
    "#token=" + token + "%3D", "#token=invalid",
    "?token=" + token, "#subject=alice&token=" + token,
  ];
  for (const fragment of fragments) {
    const page = startPage(fragment);
    assert.equal(page.scrubCalls, 1);
    assert.equal(page.browser.location.hash, "");
    page.ready();
    assert.equal(page.button.hidden, true, fragment);
    assert.equal(page.calls.length, 0, fragment);
    assert.ok(!page.status.textContent.includes(token), fragment);
  }
});

test("history.replaceState failure fails closed and never posts the bearer", () => {
  const page = startPage("#token=" + token, { scrubFails: true });
  assert.equal(page.scrubCalls, 1);
  page.ready();
  assert.equal(page.button.hidden, true);
  assert.equal(page.calls.length, 0);
  assert.match(page.status.textContent, /could not safely open/);
});

test("400, 429, 503 and unexpected responses never claim verification success", async () => {
  for (const httpStatus of [400, 429, 503, 404, 500]) {
    const page = startPage("#token=" + token, { httpStatus });
    page.ready();
    page.click();
    await drainPromises();
    assert.equal(page.calls.length, 1);
    assert.equal(page.button.hidden, true);
    assert.ok(!page.status.textContent.includes("Email verified."), String(httpStatus));
    assert.ok(!page.status.textContent.includes(token), String(httpStatus));
    if (httpStatus === 400) assert.match(page.status.textContent, /already used/);
    if (httpStatus === 429) assert.match(page.status.textContent, /Too many attempts/);
    if (httpStatus >= 500) assert.match(page.status.textContent, /may have succeeded/);
    page.click();
    assert.equal(page.calls.length, 1, "no auto retry after a server result");
  }
});

test("network failure is ambiguous and never automatically retries", async () => {
  const page = startPage("#token=" + token, { networkFailure: true });
  page.ready();
  page.click();
  await drainPromises();
  assert.equal(page.calls.length, 1);
  assert.match(page.status.textContent, /may have succeeded/);
  assert.ok(!page.status.textContent.includes("synthetic"));
  assert.ok(!page.status.textContent.includes(token));
  page.click();
  assert.equal(page.calls.length, 1);
});
