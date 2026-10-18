/* termino landing page — progressive enhancement only.
 * Every piece of content is already in the HTML; this script adds copy
 * buttons, keyboard tabs, the theme switcher and the demo animations.
 * It makes no network requests and stores nothing. */
(function () {
  "use strict";

  var root = document.documentElement;
  root.classList.add("js");

  var reduce = window.matchMedia ? window.matchMedia("(prefers-reduced-motion: reduce)") : { matches: false };

  var DEFAULT_SYMBOLS = { high: "▲", critical: "!!!", low: "▼", done: "✓" };
  var THEMES = [
    { id: "default", symbols: DEFAULT_SYMBOLS, border: "rounded" },
    { id: "spooky", symbols: { high: "🔥", critical: "💀", low: "🌙", done: "👻" }, border: "rounded" },
    { id: "dracula", symbols: DEFAULT_SYMBOLS, border: "rounded" },
    { id: "nord", symbols: DEFAULT_SYMBOLS, border: "rounded" },
    { id: "gruvbox", symbols: DEFAULT_SYMBOLS, border: "normal" },
    { id: "catppuccin-mocha", symbols: DEFAULT_SYMBOLS, border: "rounded" },
    { id: "tokyo-night", symbols: DEFAULT_SYMBOLS, border: "rounded" },
    { id: "solarized-light", symbols: DEFAULT_SYMBOLS, border: "rounded" },
    { id: "high-contrast", symbols: { high: "^", critical: "!!!", low: "v", done: "x" }, border: "thick" }
  ];
  var currentTheme = THEMES[0];

  function findTheme(id) {
    for (var i = 0; i < THEMES.length; i++) {
      if (THEMES[i].id === id) return THEMES[i];
    }
    return null;
  }

  function onReduceChange(fn) {
    if (reduce.addEventListener) reduce.addEventListener("change", fn);
    else if (reduce.addListener) reduce.addListener(fn);
  }

  /* ---------- C1: copy buttons + shared live region ---------- */
  var statusEl = document.getElementById("copy-status");

  function announce(msg) {
    if (!statusEl) return;
    // Clear first so the same message is announced again on repeat clicks.
    statusEl.textContent = "";
    window.setTimeout(function () { statusEl.textContent = msg; }, 50);
  }

  function fallbackCopy(text) {
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    var ok = false;
    try { ok = document.execCommand("copy"); } catch (e) { ok = false; }
    document.body.removeChild(ta);
    return ok;
  }

  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text).then(
        function () { return true; },
        function () { return fallbackCopy(text); }
      );
    }
    return Promise.resolve(fallbackCopy(text));
  }

  Array.prototype.forEach.call(document.querySelectorAll("button.copy[data-copy-target]"), function (btn) {
    var target = document.getElementById(btn.getAttribute("data-copy-target"));
    if (!target) return;
    var timer = null;
    btn.hidden = false;
    btn.addEventListener("click", function () {
      copyText(target.textContent.trim()).then(function (ok) {
        window.clearTimeout(timer);
        btn.setAttribute("data-state", ok ? "ok" : "err");
        btn.textContent = ok ? "Copied" : "Failed";
        announce(ok ? "Copied to clipboard" : "Copy failed. Select the text manually.");
        timer = window.setTimeout(function () {
          btn.textContent = "Copy";
          btn.removeAttribute("data-state");
        }, 2000);
      });
    });
  });

  /* ---------- C2: install tabs (WAI-ARIA tabs, automatic activation) ---------- */
  Array.prototype.forEach.call(document.querySelectorAll("[data-tabs]"), function (wrap) {
    var list = wrap.querySelector('[role="tablist"]');
    if (!list) return;
    var tabs = Array.prototype.slice.call(list.querySelectorAll('[role="tab"]'));
    list.hidden = false;

    function select(index, focus) {
      tabs.forEach(function (tab, i) {
        var on = i === index;
        var panel = document.getElementById(tab.getAttribute("aria-controls"));
        tab.setAttribute("aria-selected", on ? "true" : "false");
        tab.tabIndex = on ? 0 : -1;
        if (panel) panel.hidden = !on;
      });
      if (focus) tabs[index].focus();
    }

    tabs.forEach(function (tab, i) {
      tab.addEventListener("click", function () { select(i, false); });
      tab.addEventListener("keydown", function (e) {
        var next = null;
        if (e.key === "ArrowRight") next = (i + 1) % tabs.length;
        else if (e.key === "ArrowLeft") next = (i - 1 + tabs.length) % tabs.length;
        else if (e.key === "Home") next = 0;
        else if (e.key === "End") next = tabs.length - 1;
        if (next !== null) {
          e.preventDefault();
          select(next, true);
        }
      });
    });
    select(0, false);
  });

  /* ---------- C5: theme switcher ---------- */
  var boards = Array.prototype.slice.call(document.querySelectorAll(".tui"));
  var heroStatus = document.querySelector(".tui--hero [data-status]");
  var heroStatusTimer = null;
  var celebrating = false;

  function applyTheme(theme, announceOnBoard) {
    currentTheme = theme;
    boards.forEach(function (b) {
      b.setAttribute("data-theme", theme.id);
      b.setAttribute("data-border", theme.border);
    });
    Array.prototype.forEach.call(document.querySelectorAll(".tui [data-sym]"), function (el) {
      var sym = theme.symbols[el.getAttribute("data-sym")];
      if (sym) el.textContent = sym;
    });
    Array.prototype.forEach.call(document.querySelectorAll("[data-theme-name]"), function (el) {
      el.textContent = theme.id;
    });
    var cmd = document.querySelector("[data-theme-cmd]");
    if (cmd) cmd.textContent = "termino theme " + theme.id;
    if (announceOnBoard && heroStatus && !celebrating) {
      heroStatus.classList.remove("is-celebrating");
      heroStatus.textContent = "✓ Theme set to " + theme.id;
      window.clearTimeout(heroStatusTimer);
      heroStatusTimer = window.setTimeout(function () {
        if (!celebrating) heroStatus.textContent = "\u00a0";
      }, 3000);
    }
  }

  var switcher = document.querySelector("[data-switcher]");
  if (switcher) {
    // Native radios: arrow keys, Tab and Space work without extra code.
    switcher.hidden = false;
    switcher.addEventListener("change", function (e) {
      var theme = e.target && e.target.name === "board-theme" ? findTheme(e.target.value) : null;
      if (theme) applyTheme(theme, true);
    });

    // ?theme=<id> preselects a theme (handy for sharing a link or a screenshot).
    var match = /[?&]theme=([a-z-]+)/.exec(window.location.search);
    var preset = match && findTheme(match[1]);
    if (preset) {
      var input = switcher.querySelector('input[value="' + preset.id + '"]');
      if (input) input.checked = true;
    }
    // Sync with whatever is checked (the preset, or a radio the browser restored on reload).
    var checked = switcher.querySelector('input[name="board-theme"]:checked');
    var initial = checked && findTheme(checked.value);
    if (initial && initial.id !== "default") applyTheme(initial, false);
  }

  /* ---------- C3: hero board animation (WEB-4 → Done) ---------- */
  var hero = document.querySelector(".tui--hero");
  var board = (function () {
    if (!hero) return null;
    var card = hero.querySelector('[data-card="WEB-4"]');
    var progressList = hero.querySelector('[data-col="progress"] .col__list');
    var doneCol = hero.querySelector('[data-col="done"]');
    var doneList = doneCol && doneCol.querySelector(".col__list");
    var wip = hero.querySelector("[data-wip]");
    if (!card || !progressList || !doneList) return null;

    var timers = [];
    var running = false;
    var visible = false;
    var SPARKLES = ["✦", "✧", "★", "✧"];

    function later(fn, ms) { timers.push(window.setTimeout(fn, ms)); }
    function clearTimers() { timers.forEach(window.clearTimeout); timers = []; }

    // FLIP: measure, move the node, invert with a transform, then play.
    function moveCard(list) {
      var first = card.getBoundingClientRect();
      list.appendChild(card);
      var last = card.getBoundingClientRect();
      card.classList.remove("is-flying");
      card.style.transform = "translate(" + (first.left - last.left) + "px," + (first.top - last.top) + "px)";
      void card.offsetWidth; // force a reflow so the transition starts from the inverted spot
      card.classList.add("is-flying");
      card.style.transform = "";
      later(function () { card.classList.remove("is-flying"); }, 500);
    }

    function setStatus(text, party) {
      if (!heroStatus) return;
      heroStatus.textContent = text;
      heroStatus.classList.toggle("is-celebrating", !!party);
    }

    function reset() {
      clearTimers();
      celebrating = false;
      card.classList.remove("is-flying");
      card.style.transform = "";
      if (card.parentNode !== progressList) progressList.appendChild(card);
      if (wip) wip.textContent = "[1/3]";
      if (doneCol) doneCol.classList.remove("is-celebrating");
      setStatus("\u00a0", false);
    }

    function cycle() {
      later(function () {
        moveCard(doneList);
        if (wip) wip.textContent = "[0/3]";
        celebrating = true;
        if (doneCol) doneCol.classList.add("is-celebrating");
        var frame = 0;
        var party = function () {
          var s = SPARKLES[frame % SPARKLES.length];
          setStatus(s + " " + currentTheme.symbols.done + " WEB-4 done! " + s, true);
          frame++;
          if (frame < 8) later(party, 120);
          else {
            celebrating = false;
            if (doneCol) doneCol.classList.remove("is-celebrating");
            setStatus("Moved WEB-4 to Done", false);
          }
        };
        party();
      }, 1200);
      later(function () { setStatus("\u00a0", false); }, 1200 + 960 + 2800);
      later(function () {
        moveCard(progressList);
        if (wip) wip.textContent = "[1/3]";
      }, 1200 + 960 + 3600);
      later(cycle, 1200 + 960 + 3600 + 600);
    }

    function update() {
      var should = visible && !reduce.matches && document.visibilityState === "visible";
      if (should && !running) {
        running = true;
        reset();
        cycle();
      } else if (!should && running) {
        running = false;
        reset();
      }
    }

    if ("IntersectionObserver" in window) {
      new IntersectionObserver(function (entries) {
        visible = entries[entries.length - 1].isIntersecting;
        update();
      }, { threshold: 0.35 }).observe(hero);
    } else {
      visible = true;
    }
    document.addEventListener("visibilitychange", update);
    return { update: update };
  })();

  /* ---------- C4: CLI typing (runs once when scrolled into view) ---------- */
  var term = document.querySelector("[data-typing]");
  var typing = (function () {
    if (!term) return null;
    var lines = Array.prototype.slice.call(term.querySelectorAll(".ln"));
    var timers = [];
    var started = false;

    function finish() {
      timers.forEach(window.clearTimeout);
      timers = [];
      term.classList.remove("is-typing");
      lines.forEach(function (l) { l.classList.remove("on", "is-current"); });
    }

    function start() {
      if (started || reduce.matches) return;
      started = true;
      term.classList.add("is-typing");
      var t = 300;
      lines.forEach(function (line, i) {
        var isCmd = line.classList.contains("ln--cmd");
        var chars = line.textContent.length;
        line.style.setProperty("--chars", String(chars));
        timers.push(window.setTimeout(function () {
          lines.forEach(function (l) { l.classList.remove("is-current"); });
          line.classList.add("on");
          if (isCmd) line.classList.add("is-current");
        }, t));
        t += isCmd ? chars * 28 + 450 : 70;
        if (i === lines.length - 1) {
          // Leave the final state as plain text so nothing keeps animating.
          timers.push(window.setTimeout(function () {
            lines.forEach(function (l) { l.classList.remove("is-current"); });
          }, t + 1500));
        }
      });
    }

    if (reduce.matches) return { finish: finish };
    if ("IntersectionObserver" in window) {
      var io = new IntersectionObserver(function (entries) {
        if (entries[entries.length - 1].isIntersecting) {
          io.disconnect();
          start();
        }
      }, { threshold: 0.4 });
      io.observe(term);
    }
    return { finish: finish };
  })();

  /* ---------- C0: react to the OS motion setting changing ---------- */
  onReduceChange(function () {
    if (board) board.update();
    if (reduce.matches && typing) typing.finish();
  });
})();
