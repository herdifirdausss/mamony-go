(function () {
  "use strict";

  // This test-only, unmounted page never stores a credential, a session
  // bearer or an in-flight mutation in localStorage, cookies or a URL.
  // Both uncertain network outcomes and HTTP 503 permanently lock the form:
  // the password rotation may already have committed.
  window.addEventListener("DOMContentLoaded", function () {
    var status = document.getElementById("status");
    var form = document.getElementById("change-form");
    var submit = document.getElementById("change");
    var email = document.getElementById("email");
    var current = document.getElementById("current");
    var replacement = document.getElementById("replacement");
    var confirmation = document.getElementById("confirmation");
    if (!status || !form || !submit || !email || !current || !replacement || !confirmation) {
      return;
    }

    var csrf = "";
    var state = "loading";
    var message = {
      signedOut: "Your session is not active. Sign in using your usual trusted Mamony entry point before changing your password.",
      uncertain: "We could not confirm the result. Your password may already have changed. Do not submit this change again. Use your usual trusted sign-in or account recovery path.",
      denied: "We could not complete this change. The account, current password, request limits, or replacement rules may require attention. Return through your trusted account entry point.",
      complete: "Your password has been changed. All existing sessions are no longer valid. Sign in again through your usual trusted Mamony entry point."
    };

    function lock(text) {
      state = "locked";
      csrf = "";
      submit.disabled = true;
      form.hidden = true;
      current.value = "";
      replacement.value = "";
      confirmation.value = "";
      status.textContent = text;
    }

    function request(path, options) {
      return fetch(path, Object.assign({
        method: "GET",
        credentials: "same-origin",
        redirect: "error",
        mode: "same-origin",
        cache: "no-store",
        referrerPolicy: "no-referrer"
      }, options || {}));
    }

    // Do not present the form until the real session and its session-bound
    // CSRF capability are both available. Never renew a session implicitly.
    request("/v1/identity/session").then(function (response) {
      if (response.status !== 200) {
        lock(response.status === 401 ? message.signedOut :
          "We could not safely check your session. Return through your trusted account entry point.");
        return null;
      }
      return request("/v1/identity/csrf");
    }).then(function (response) {
      if (!response || state === "locked") {
        return;
      }
      if (response.status !== 200) {
        lock(response.status === 401 ? message.signedOut :
          "We could not prepare this request safely. Return through your trusted account entry point.");
        return;
      }
      return response.json().then(function (body) {
        if (state === "locked" || !body || typeof body.csrfToken !== "string" ||
            !/^[A-Za-z0-9_-]{54}$/.test(body.csrfToken)) {
          lock("We could not prepare this request safely. Return through your trusted account entry point.");
          return;
        }
        csrf = body.csrfToken;
        state = "ready";
        form.hidden = false;
        submit.disabled = false;
        status.textContent = "Your session is active. Enter your current password and choose a new one.";
      });
    }).catch(function () {
      lock("We could not safely check your session. Return through your trusted account entry point.");
    });

    submit.addEventListener("click", function () {
      if (state !== "ready" || !csrf) {
        return;
      }
      if (!form.reportValidity()) {
        return;
      }
      if (replacement.value !== confirmation.value) {
        status.textContent = "The new password entries do not match. Check them before continuing.";
        return;
      }
      if (current.value === replacement.value) {
        status.textContent = "Choose a different new password.";
        return;
      }

      // One deliberate action authorizes exactly one mutation attempt.
      // Clear the DOM values and disable the form BEFORE the network call.
      var body = JSON.stringify({
        email: email.value,
        currentPassword: current.value,
        nextPassword: replacement.value
      });
      var proof = csrf;
      csrf = "";
      state = "submitted";
      current.value = "";
      replacement.value = "";
      confirmation.value = "";
      form.hidden = true;
      submit.disabled = true;
      status.textContent = "Submitting your password change. Do not close this page.";

      request("/v1/identity/change-password", {
        method: "POST",
        headers: {"Content-Type": "application/json", "X-Mamony-CSRF": proof},
        body: body
      }).then(function (response) {
        if (response.status === 204) {
          lock(message.complete);
        } else if (response.status === 401) {
          lock(message.denied);
        } else {
          // 503 can be a post-commit acknowledgement failure. Likewise a
          // 403/400 response does not authorize automatic resubmission.
          lock(message.uncertain);
        }
      }).catch(function () {
        lock(message.uncertain);
      });
      body = "";
      proof = "";
    });
  }, {once: true});
}());
