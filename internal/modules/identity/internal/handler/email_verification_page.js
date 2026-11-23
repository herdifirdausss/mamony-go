(function () {
  "use strict";

  // The email URL fragment is never sent to the server by the browser.
  // Scrub it before DOM-ready handlers, fetch, third-party code or UI.
  var fragment = window.location.hash;
  var token = "";
  var scrubbed = false;
  try {
    window.history.replaceState(null, "", "/verify-email");
    scrubbed = true;
  } catch (_) {
    // Without reliable history cleanup, never send the bearer.
  }
  if (scrubbed && /^#token=[A-Za-z0-9_-]{43}$/.test(fragment)) {
    token = fragment.slice(7);
  }
  fragment = "";

  window.addEventListener("DOMContentLoaded", function () {
    var status = document.getElementById("status");
    var button = document.getElementById("confirm");
    if (!status || !button) {
      token = "";
      return;
    }
    if (!scrubbed) {
      status.textContent = "We could not safely open this link. Please return to your email.";
      return;
    }
    if (!token) {
      status.textContent = "This link is missing or cannot be used. If you have already verified, no further action is needed.";
      return;
    }

    // Email scanners can preview a GET. Consuming a single-use token MUST
    // require a deliberate user click, never automatic on page load.
    status.textContent = "Ready to verify your email. Select the button to continue.";
    button.hidden = false;
    button.addEventListener("click", function () {
      if (!token) {
        return;
      }
      var proof = token;
      token = "";
      button.hidden = true;
      button.disabled = true;
      status.textContent = "Checking your verification request...";

      // No cookies, referrer, redirect, storage, analytics or extra fields.
      // A 503/network error is ambiguous: the transaction may have committed,
      // so never retry or rotate a proof automatically in this page.
      var payload = JSON.stringify({token: proof});
      proof = "";
      fetch("/v1/identity/verify-email", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: payload,
        credentials: "omit",
        redirect: "error",
        mode: "same-origin",
        cache: "no-store",
        referrerPolicy: "no-referrer"
      }).then(function (reply) {
        if (reply.status === 204) {
          status.textContent = "Email verified. Verification does not sign you in.";
        } else if (reply.status === 400) {
          status.textContent = "This link cannot be confirmed. It may be expired, already used, or invalid. Check account status when sign-in becomes available.";
        } else if (reply.status === 429) {
          status.textContent = "Too many attempts. Try again later using your original email link.";
        } else {
          status.textContent = "We could not confirm the result. Verification may have succeeded. Check account status when sign-in becomes available.";
        }
      }).catch(function () {
        status.textContent = "We could not confirm the result. Verification may have succeeded. Check account status when sign-in becomes available.";
      });
    }, {once: true});
  }, {once: true});
}());
