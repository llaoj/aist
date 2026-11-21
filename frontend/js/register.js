(function () {
  "use strict";

  var i18n = window.AIST_I18N;

  function translate(key) {
    return i18n ? i18n.t(key) : key;
  }

  function resolveServerURL() {
    var configured = (document.body.getAttribute("data-server-url") || "").trim();
    if (configured && configured.indexOf("__AIST_") === -1) {
      return configured.replace(/\/+$/, "");
    }
    return window.location.origin.replace(/\/+$/, "");
  }

  var serverURL = resolveServerURL();

  var stepsBar = document.getElementById("register-steps");
  var errorBox = document.getElementById("auth-error");
  var resetSuccessBox = document.getElementById("reset-success");

  var registerForm = document.getElementById("register-form");
  var inviteInput = document.getElementById("invite-code");
  var emailInput = document.getElementById("register-email");
  var passwordInput = document.getElementById("register-password");
  var togglePasswordBtn = document.getElementById("toggle-password");
  var registerSubmitBtn = document.getElementById("register-submit");

  var doneEmailTarget = document.getElementById("done-email-target");
  var doneVerifyNotice = document.getElementById("done-verify-notice");
  var resendVerificationBtn = document.getElementById("resend-verification");
  var doneCurlCmd = document.getElementById("done-curl-cmd");
  var doneWgetCmd = document.getElementById("done-wget-cmd");

  var resetForm = document.getElementById("reset-form");
  var resetEmailInput = document.getElementById("reset-email");
  var resetSubmitBtn = document.getElementById("reset-submit");
  var gotoResetLink = document.getElementById("goto-reset");
  var resetBackLink = document.getElementById("reset-back");

  var state = {
    view: "form",
    errorCode: "",
    registeredEmail: "",
    resendRemaining: 0,
    resendTimer: null
  };

  function showErrorByCode(code) {
    state.errorCode = code || "";
    if (!state.errorCode) {
      errorBox.hidden = true;
      errorBox.textContent = "";
      return;
    }
    var key = "register.error." + state.errorCode;
    var text = translate(key);
    if (text === key) {
      text = translate("register.error.generic");
    }
    errorBox.textContent = text;
    errorBox.hidden = false;
  }

  function setView(viewName) {
    state.view = viewName;
    showErrorByCode("");
    if (resetSuccessBox && viewName !== "reset") {
      resetSuccessBox.hidden = true;
    }

    document.querySelectorAll("[data-view]").forEach(function (el) {
      el.hidden = el.getAttribute("data-view") !== viewName;
    });

    if (stepsBar) {
      stepsBar.hidden = viewName === "reset";
      var order = ["form", "done"];
      var activeIndex = order.indexOf(viewName);
      document.querySelectorAll("[data-step-indicator]").forEach(function (item) {
        var stepName = item.getAttribute("data-step-indicator");
        var idx = order.indexOf(stepName);
        item.classList.toggle("is-active", stepName === viewName);
        item.classList.toggle("is-done", activeIndex > idx && idx >= 0);
      });
    }
  }

  function updateResendButton() {
    if (!resendVerificationBtn) return;
    if (state.resendRemaining > 0) {
      resendVerificationBtn.disabled = true;
      resendVerificationBtn.textContent = translate(
        "register.done.resend_wait"
      ).replace("{s}", String(state.resendRemaining));
    } else {
      resendVerificationBtn.disabled = false;
      resendVerificationBtn.textContent = translate("register.done.resend");
    }
  }

  function startResendCountdown(seconds) {
    if (state.resendTimer) {
      clearInterval(state.resendTimer);
    }
    state.resendRemaining = seconds;
    updateResendButton();
    state.resendTimer = setInterval(function () {
      state.resendRemaining -= 1;
      if (state.resendRemaining <= 0) {
        state.resendRemaining = 0;
        clearInterval(state.resendTimer);
        state.resendTimer = null;
      }
      updateResendButton();
    }, 1000);
  }

  function showDoneView(email, verificationSent) {
    state.registeredEmail = email;
    doneEmailTarget.textContent = email;
    doneCurlCmd.textContent = "curl -fL -# " + serverURL + "/run.sh | sh";
    doneWgetCmd.textContent = "wget -qO- " + serverURL + "/run.sh | sh";
    if (doneVerifyNotice) {
      doneVerifyNotice.hidden = !verificationSent;
    }
    if (verificationSent) {
      startResendCountdown(120);
    }
    setView("done");
  }

  function postJSON(path, payload) {
    return fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    }).then(function (resp) {
      if (resp.status === 204) {
        return { ok: true, status: 204, data: {} };
      }
      return resp
        .json()
        .catch(function () {
          return {};
        })
        .then(function (data) {
          return { ok: resp.ok, status: resp.status, data: data };
        });
    });
  }

  function setButtonBusy(btn, busy, idleKey) {
    btn.disabled = busy;
    btn.textContent = translate(busy ? "register.submitting" : idleKey);
  }

  function setPasswordVisible(visible) {
    if (!passwordInput || !togglePasswordBtn) return;
    passwordInput.type = visible ? "text" : "password";
    togglePasswordBtn.setAttribute("aria-pressed", visible ? "true" : "false");
    togglePasswordBtn.setAttribute(
      "aria-label",
      translate(visible ? "register.password.hide" : "register.password.show")
    );
    var eyeIcon = togglePasswordBtn.querySelector(".icon-eye");
    var eyeOffIcon = togglePasswordBtn.querySelector(".icon-eye-off");
    if (eyeIcon) eyeIcon.hidden = !visible;
    if (eyeOffIcon) eyeOffIcon.hidden = visible;
  }

  if (togglePasswordBtn) {
    togglePasswordBtn.addEventListener("click", function () {
      setPasswordVisible(passwordInput.type === "password");
    });
  }

  if (registerForm) {
    registerForm.addEventListener("submit", function (event) {
      event.preventDefault();
      setPasswordVisible(false);
      showErrorByCode("");
      setButtonBusy(registerSubmitBtn, true, "register.submit");

      var payload = {
        invite_code: inviteInput.value.trim(),
        email: emailInput.value.trim(),
        password: passwordInput.value
      };

      postJSON("v1/register", payload)
        .then(function (result) {
          setButtonBusy(registerSubmitBtn, false, "register.submit");
          if (!result.ok) {
            if (result.status === 429) {
              showErrorByCode("rate_limited");
              return;
            }
            var errCode =
              result.data && result.data.error && result.data.error.code
                ? result.data.error.code
                : "generic";
            showErrorByCode(errCode);
            return;
          }

          showDoneView(
            result.data.email || payload.email,
            Boolean(result.data.verification_sent)
          );
        })
        .catch(function () {
          setButtonBusy(registerSubmitBtn, false, "register.submit");
          showErrorByCode("generic");
        });
    });
  }

  if (resendVerificationBtn) {
    resendVerificationBtn.addEventListener("click", function () {
      if (state.resendRemaining > 0 || !state.registeredEmail) return;
      showErrorByCode("");
      setButtonBusy(resendVerificationBtn, true, "register.done.resend");

      postJSON("api/collections/users/request-verification", {
        email: state.registeredEmail
      })
        .then(function (result) {
          setButtonBusy(resendVerificationBtn, false, "register.done.resend");
          if (!result.ok) {
            showErrorByCode("generic");
            return;
          }
          startResendCountdown(120);
        })
        .catch(function () {
          setButtonBusy(resendVerificationBtn, false, "register.done.resend");
          showErrorByCode("generic");
        });
    });
  }

  if (resetForm) {
    resetForm.addEventListener("submit", function (event) {
      event.preventDefault();
      var email = resetEmailInput.value.trim();
      if (!email) {
        showErrorByCode("invalid_email");
        return;
      }
      showErrorByCode("");
      resetSuccessBox.hidden = true;
      setButtonBusy(resetSubmitBtn, true, "register.reset.submit");

      postJSON("api/collections/users/request-password-reset", { email: email })
        .then(function (result) {
          setButtonBusy(resetSubmitBtn, false, "register.reset.submit");
          if (!result.ok && result.status === 400) {
            showErrorByCode("invalid_email");
            return;
          }
          if (!result.ok) {
            showErrorByCode("generic");
            return;
          }
          resetSuccessBox.hidden = false;
        })
        .catch(function () {
          setButtonBusy(resetSubmitBtn, false, "register.reset.submit");
          showErrorByCode("generic");
        });
    });
  }

  if (gotoResetLink) {
    gotoResetLink.addEventListener("click", function (event) {
      event.preventDefault();
      if (emailInput && emailInput.value && !resetEmailInput.value) {
        resetEmailInput.value = emailInput.value.trim();
      }
      if (window.history && window.history.replaceState) {
        window.history.replaceState(null, "", "#reset");
      } else {
        window.location.hash = "reset";
      }
      setView("reset");
      resetEmailInput.focus();
    });
  }

  if (resetBackLink) {
    resetBackLink.addEventListener("click", function (event) {
      event.preventDefault();
      if (window.history && window.history.replaceState) {
        window.history.replaceState(
          null,
          "",
          window.location.pathname + window.location.search
        );
      } else {
        window.location.hash = "";
      }
      setView("form");
    });
  }

  document.addEventListener("aist:langchange", function () {
    if (state.errorCode) {
      showErrorByCode(state.errorCode);
    }
    updateResendButton();
    if (passwordInput) {
      setPasswordVisible(passwordInput.type === "text");
    }
  });

  setPasswordVisible(false);

  function initFromURL() {
    var params = new URLSearchParams(window.location.search);
    var prefillCode = params.get("code") || params.get("invite") || "";
    if (prefillCode && inviteInput) {
      inviteInput.value = prefillCode.trim();
    }

    if (
      window.location.hash === "#reset" ||
      params.get("mode") === "reset"
    ) {
      setView("reset");
    } else {
      setView("form");
    }
  }

  window.addEventListener("hashchange", function () {
    if (window.location.hash === "#reset") {
      setView("reset");
    } else if (state.view === "reset") {
      setView("form");
    }
  });

  initFromURL();
})();
