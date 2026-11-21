(function () {
  "use strict";

  var i18n = window.AIST_I18N;

  function translate(key, fallback) {
    return i18n ? i18n.t(key) : fallback;
  }

  var reduceMotion =
    window.matchMedia &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  var navToggle = document.querySelector(".nav-toggle");
  var nav = document.querySelector(".nav");

  if (navToggle && nav) {
    function setNavOpen(open) {
      nav.classList.toggle("open", open);
      navToggle.setAttribute("aria-expanded", String(open));
      navToggle.setAttribute(
        "aria-label",
        translate(open ? "nav.close" : "nav.open", open ? "关闭菜单" : "打开菜单")
      );
    }

    navToggle.addEventListener("click", function () {
      setNavOpen(!nav.classList.contains("open"));
    });

    nav.querySelectorAll("a").forEach(function (link) {
      link.addEventListener("click", function () {
        setNavOpen(false);
      });
    });

    document.addEventListener("aist:langchange", function () {
      setNavOpen(nav.classList.contains("open"));
    });
  }

  document.querySelectorAll("[data-tabs]").forEach(function (tabs) {
    var buttons = Array.prototype.slice.call(
      tabs.querySelectorAll("[data-tab]")
    );
    var panels = Array.prototype.slice.call(
      tabs.querySelectorAll("[data-panel]")
    );
    if (!buttons.length || !panels.length) return;

    function select(name) {
      buttons.forEach(function (button) {
        var active = button.getAttribute("data-tab") === name;
        button.classList.toggle("is-active", active);
        button.setAttribute("aria-selected", active ? "true" : "false");
        button.tabIndex = active ? 0 : -1;
      });
      panels.forEach(function (panel) {
        panel.hidden = panel.getAttribute("data-panel") !== name;
      });
    }

    buttons.forEach(function (button, index) {
      button.addEventListener("click", function () {
        select(button.getAttribute("data-tab"));
      });

      button.addEventListener("keydown", function (event) {
        var key = event.key;
        if (
          key !== "ArrowLeft" &&
          key !== "ArrowRight" &&
          key !== "Home" &&
          key !== "End"
        ) {
          return;
        }
        event.preventDefault();

        var next = index;
        if (key === "ArrowLeft") next = (index - 1 + buttons.length) % buttons.length;
        if (key === "ArrowRight") next = (index + 1) % buttons.length;
        if (key === "Home") next = 0;
        if (key === "End") next = buttons.length - 1;

        buttons[next].focus();
        select(buttons[next].getAttribute("data-tab"));
      });
    });

    select(buttons[0].getAttribute("data-tab"));
  });

  document.querySelectorAll("[data-copy]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var text = btn.getAttribute("data-copy-text");
      if (!text) {
        var block = btn.closest(".code-block");
        var code = block ? block.querySelector("code") : null;
        if (!code) return;
        text = code.innerText;
      }

      function done() {
        btn.textContent = translate("copy.done", "已复制");
        btn.classList.add("copied");
        setTimeout(function () {
          btn.textContent = translate("copy.label", "复制");
          btn.classList.remove("copied");
        }, 1600);
      }

      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, fallback);
      } else {
        fallback();
      }

      function fallback() {
        var ta = document.createElement("textarea");
        ta.value = text;
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        try {
          document.execCommand("copy");
          done();
        } catch (e) {
          /* ignore */
        }
        document.body.removeChild(ta);
      }
    });
  });

  var revealItems = Array.prototype.slice.call(
    document.querySelectorAll("[data-reveal]")
  );
  if (revealItems.length) {
    if (!reduceMotion && "IntersectionObserver" in window) {
      document.documentElement.classList.add("reveal-ready");
      var observer = new IntersectionObserver(
        function (entries) {
          entries.forEach(function (entry) {
            if (entry.isIntersecting) {
              entry.target.classList.add("is-visible");
              observer.unobserve(entry.target);
            }
          });
        },
        { rootMargin: "0px 0px -8% 0px", threshold: 0.08 }
      );
      revealItems.forEach(function (item) {
        observer.observe(item);
      });
    } else {
      revealItems.forEach(function (item) {
        item.classList.add("is-visible");
      });
    }
  }

  var header = document.querySelector(".site-header");
  if (header) {
    window.addEventListener(
      "scroll",
      function () {
        if (window.scrollY > 8) {
          header.style.boxShadow = "0 6px 24px -12px rgba(0,0,0,0.8)";
        } else {
          header.style.boxShadow = "none";
        }
      },
      { passive: true }
    );
  }
})();
