(function () {
  "use strict";

  var TAWK_SRC = "https://embed.tawk.to/6b02383ab8412e3ce43fafbb/1k80an9sg";
  var PUBLIC_HOSTS = ["aistrun.cc", "www.aistrun.cc"];
  var LOCAL_HOSTS = ["localhost", "127.0.0.1", "::1", "[::1]"];

  function isDebugLocalhost(host) {
    if (LOCAL_HOSTS.indexOf(host) === -1) return false;
    return /(?:^\?|&)contact=1(?:&|$)/.test(location.search || "");
  }

  function isPublicOrDebugHost() {
    var host = (location.hostname || "").toLowerCase();
    return PUBLIC_HOSTS.indexOf(host) !== -1 || isDebugLocalhost(host);
  }

  if (!isPublicOrDebugHost()) return;

  Array.prototype.forEach.call(
    document.querySelectorAll("[data-contact-only]"),
    function (item) {
      item.hidden = false;
    }
  );

  if (location.hash === "#contact") {
    var target = document.getElementById("contact");
    if (target && typeof target.scrollIntoView === "function") {
      target.scrollIntoView();
    }
  }

  if (!TAWK_SRC) return;

  var readyItems = Array.prototype.slice.call(
    document.querySelectorAll("[data-chat-ready]")
  );
  var buttons = Array.prototype.slice.call(
    document.querySelectorAll("[data-chat-open]")
  );

  function openChat() {
    if (window.Tawk_API && typeof window.Tawk_API.maximize === "function") {
      window.Tawk_API.maximize();
    }
  }

  buttons.forEach(function (button) {
    button.addEventListener("click", openChat);
  });

  var Tawk_API = window.Tawk_API || {};
  var previousOnLoad = Tawk_API.onLoad;
  Tawk_API.onLoad = function () {
    if (typeof previousOnLoad === "function") {
      previousOnLoad();
    }
    window.AIST_CHAT = { open: openChat };
    readyItems.forEach(function (item) {
      item.hidden = false;
    });
    buttons.forEach(function (button) {
      button.hidden = false;
    });
  };
  window.Tawk_API = Tawk_API;
  window.Tawk_LoadStart = new Date();

  var s1 = document.createElement("script");
  var s0 = document.getElementsByTagName("script")[0];
  s1.async = true;
  s1.src = TAWK_SRC;
  s1.charset = "UTF-8";
  s1.setAttribute("crossorigin", "*");
  if (s0 && s0.parentNode) {
    s0.parentNode.insertBefore(s1, s0);
  } else {
    document.head.appendChild(s1);
  }
})();
