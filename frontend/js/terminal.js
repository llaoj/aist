(function () {
  "use strict";

  var el = document.querySelector(".terminal-body");
  if (!el) return;

  var i18n = window.AIST_I18N;
  if (!i18n) return;

  var SPINNER = ["-", "\\", "|", "/"];
  var generation = 0;

  var reduce =
    window.matchMedia &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  var cursor = document.createElement("span");
  cursor.className = "term-cursor";
  cursor.textContent = "\u2588";

  function currentMessages() {
    return i18n.messages[i18n.getLanguage()] || i18n.messages.zh;
  }

  function flatten(script) {
    var tokens = [];
    script.forEach(function (seg) {
      if (seg.think || seg.confirm) {
        tokens.push({ think: !!seg.think, confirm: !!seg.confirm });
        return;
      }
      var cls = seg.c || "";
      for (var i = 0; i < seg.t.length; i++) {
        tokens.push({ ch: seg.t[i], cls: cls });
      }
    });
    return tokens;
  }

  function optionLine(selected, i, options) {
    return (
      (selected === i ? ">" : " ") +
      " [" +
      (selected === i ? "x" : " ") +
      "] " +
      options[i]
    );
  }

  function lastConfirmIndex(tokens) {
    for (var i = tokens.length - 1; i >= 0; i--) {
      if (tokens[i].confirm) return i;
    }
    return -1;
  }

  // Mirrors the client: the key hint disappears once a choice is confirmed,
  // so only the final, still-pending prompt keeps it.
  function renderAll(tokens, options, hint) {
    var lastConfirm = lastConfirmIndex(tokens);
    el.textContent = "";
    var span = null;
    var cls = null;

    function append(text, className) {
      span = document.createElement("span");
      if (className) span.className = className;
      span.appendChild(document.createTextNode(text));
      el.appendChild(span);
      cls = className || "";
    }

    tokens.forEach(function (item, index) {
      if (item.think) return;
      if (item.confirm) {
        options.forEach(function (_, i) {
          append(optionLine(0, i, options) + "\n", "t-opt");
        });
        if (hint && index === lastConfirm) append(hint + "\n", "t-hint");
        return;
      }
      if (item.cls !== cls || !span) {
        span = document.createElement("span");
        if (item.cls) span.className = item.cls;
        el.appendChild(span);
        cls = item.cls;
      }
      span.appendChild(document.createTextNode(item.ch));
    });
  }

  function init() {
    var run = ++generation;
    var dict = currentMessages();
    var script = dict["terminal.script"] || [];
    var options = dict["terminal.options"] || [];
    var thinking = dict["terminal.thinking"] || "Thinking";
    var interrupt = dict["terminal.interrupt"] || "ESC to cancel";
    var hint = dict["terminal.confirm_hint"] || "";
    var tokens = flatten(script);
    var lastConfirm = lastConfirmIndex(tokens);

    el.style.minHeight = "";
    renderAll(tokens, options, hint);
    el.appendChild(cursor);
    var fixedHeight = el.offsetHeight;
    if (fixedHeight) el.style.minHeight = fixedHeight + "px";

    if (reduce) {
      cursor.remove();
      return;
    }

    el.textContent = "";

    var span = null;
    var cls = null;
    var index = 0;

    function later(fn, ms) {
      setTimeout(function () {
        if (run === generation) fn();
      }, ms);
    }

    function start() {
      if (run !== generation) return;
      el.textContent = "";
      el.appendChild(cursor);
      span = null;
      cls = null;
      index = 0;
      later(step, 450);
    }

    function step() {
      if (index >= tokens.length) {
        later(start, 5000);
        return;
      }

      var item = tokens[index++];

      if (item.think) {
        runThinking(step);
        return;
      }
      if (item.confirm) {
        runConfirm(step, index - 1 === lastConfirm);
        return;
      }

      if (item.cls !== cls || !span) {
        span = document.createElement("span");
        if (item.cls) span.className = item.cls;
        el.insertBefore(span, cursor);
        cls = item.cls;
      }
      span.appendChild(document.createTextNode(item.ch));

      later(step, item.ch === "\n" ? 120 : 22);
    }

    function runThinking(done) {
      var line = document.createElement("span");
      line.className = "t-think";
      el.insertBefore(line, cursor);
      span = null;
      cls = null;

      var frame = 0;
      var ticks = 0;
      var timer = setInterval(function () {
        if (run !== generation) {
          clearInterval(timer);
          return;
        }
        var seconds = Math.floor(ticks / 8);
        line.textContent =
          SPINNER[frame % SPINNER.length] +
          " " +
          thinking +
          "(" +
          seconds +
          "s) " +
          interrupt;
        frame++;
        if (++ticks >= 16) {
          clearInterval(timer);
          line.remove();
          done();
        }
      }, 120);
    }

    function runConfirm(done, keepHint) {
      span = null;
      cls = null;

      var rows = options.map(function () {
        var row = document.createElement("span");
        row.className = "t-opt";
        el.insertBefore(row, cursor);
        el.insertBefore(document.createTextNode("\n"), cursor);
        return row;
      });

      var hintRow = null;
      if (hint) {
        hintRow = document.createElement("span");
        hintRow.className = "t-hint";
        hintRow.textContent = hint + "\n";
        el.insertBefore(hintRow, cursor);
      }

      function draw(selected) {
        rows.forEach(function (row, i) {
          row.textContent = optionLine(selected, i, options);
        });
      }

      var sequence = [-1, 1, 2, 1, 0];
      var i = 0;
      draw(sequence[0]);

      function next() {
        i++;
        if (i < sequence.length) {
          draw(sequence[i]);
          later(next, 320);
          return;
        }
        later(function () {
          if (hintRow && !keepHint) hintRow.remove();
          done();
        }, 700);
      }
      later(next, 450);
    }

    start();
  }

  document.addEventListener("aist:langchange", init);
  init();
})();
