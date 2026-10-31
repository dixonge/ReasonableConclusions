(function () {
  var root = document.documentElement;
  var stored = localStorage.getItem("rc-theme");
  if (stored === "light" || stored === "dark") root.dataset.theme = stored;

  var themeBtn = document.getElementById("theme");
  if (themeBtn) {
    themeBtn.addEventListener("click", function () {
      var dark = root.dataset.theme
        ? root.dataset.theme === "dark"
        : window.matchMedia("(prefers-color-scheme: dark)").matches;
      var next = dark ? "light" : "dark";
      root.dataset.theme = next;
      localStorage.setItem("rc-theme", next);
      themeBtn.textContent = next === "dark" ? "Light" : "Dark";
    });
    var isDark = root.dataset.theme
      ? root.dataset.theme === "dark"
      : window.matchMedia("(prefers-color-scheme: dark)").matches;
    themeBtn.textContent = isDark ? "Light" : "Dark";
  }

  var menuBtn = document.getElementById("menu");
  var drawer = document.getElementById("site-nav");
  if (menuBtn && drawer) {
    menuBtn.addEventListener("click", function () {
      var open = drawer.classList.toggle("is-open");
      menuBtn.setAttribute("aria-expanded", open ? "true" : "false");
      menuBtn.textContent = open ? "Close" : "Menu";
    });
  }

  var navKey = "rc-sidebar";
  document.querySelectorAll("details[data-nav]").forEach(function (node) {
    var id = node.getAttribute("data-nav");
    var saved = {};
    try { saved = JSON.parse(localStorage.getItem(navKey) || "{}"); } catch (e) { saved = {}; }
    var holdsCurrent = node.querySelector("[aria-current='page']");
    if (!holdsCurrent && Object.prototype.hasOwnProperty.call(saved, id)) {
      node.open = !!saved[id];
    }
    node.addEventListener("toggle", function () {
      var state = {};
      try { state = JSON.parse(localStorage.getItem(navKey) || "{}"); } catch (e) { state = {}; }
      state[id] = node.open;
      localStorage.setItem(navKey, JSON.stringify(state));
    });
  });

  function openHash() {
    var id = decodeURIComponent((location.hash || "").slice(1));
    if (!id) return;
    var el = document.getElementById(id);
    if (el && el.tagName === "DETAILS") el.open = true;
  }
  openHash();
  window.addEventListener("hashchange", openHash);

  var indexPromise = null;
  function loadIndex() {
    if (!indexPromise) {
      indexPromise = fetch("/search-index.json").then(function (res) { return res.json(); });
    }
    return indexPromise;
  }

  function escapeHtml(s) {
    return s.replace(/[&<>"']/g, function (c) {
      return ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c];
    });
  }

  function snippet(text, words) {
    var lower = text.toLowerCase();
    var at = -1;
    for (var i = 0; i < words.length; i++) {
      at = lower.indexOf(words[i]);
      if (at >= 0) break;
    }
    if (at < 0) return escapeHtml(text.slice(0, 160));
    var start = Math.max(0, at - 60);
    var end = Math.min(text.length, at + 120);
    var slice = text.slice(start, end);
    var html = escapeHtml(slice);
    words.forEach(function (w) {
      html = html.replace(new RegExp("(" + w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + ")", "ig"), "<mark>$1</mark>");
    });
    return (start ? "…" : "") + html + (end < text.length ? "…" : "");
  }

  function matches(item, words) {
    var hay = (item.title + " " + (item.summary || "") + " " + item.text).toLowerCase();
    return words.every(function (w) { return hay.indexOf(w) >= 0; });
  }

  function renderHits(items, words, limit) {
    var hits = items.filter(function (item) { return matches(item, words); }).slice(0, limit || 12);
    if (!hits.length) return "<p class='hit-meta'>No notes match.</p>";
    return hits.map(function (hit) {
      return "<a href='" + hit.url + "'><span class='hit-title'>" + escapeHtml(hit.title) + "</span>"
        + "<span class='hit-meta'>" + escapeHtml(hit.category + " · " + hit.topic + (hit.when ? " · " + hit.when : "")) + "</span>"
        + "<span class='hit-snip'>" + snippet(hit.text, words) + "</span></a>";
    }).join("");
  }

  var input = document.getElementById("q");
  var box = document.querySelector(".search-results");
  var page = document.getElementById("search-page");

  function run(q, into, limit) {
    var words = q.toLowerCase().split(/\s+/).filter(Boolean);
    if (!words.length) {
      if (into) into.innerHTML = "";
      return;
    }
    loadIndex().then(function (items) {
      into.innerHTML = renderHits(items, words, limit);
      into.hidden = false;
    }).catch(function () {
      into.innerHTML = "<p class='hit-meta'>Search index failed to load.</p>";
      into.hidden = false;
    });
  }

  if (input && box) {
    input.addEventListener("input", function () {
      var q = input.value.trim();
      if (q.length < 2) { box.hidden = true; box.innerHTML = ""; return; }
      run(q, box, 8);
    });
    document.addEventListener("click", function (ev) {
      if (!box.contains(ev.target) && ev.target !== input) box.hidden = true;
    });
  }

  if (page) {
    var params = new URLSearchParams(location.search);
    var q = params.get("q") || "";
    if (input && q) input.value = q;
    var out = document.getElementById("search-results");
    if (q && out) run(q, out, 50);
    var form = document.querySelector(".search");
    if (form) {
      form.addEventListener("submit", function (ev) {
        if (location.pathname.indexOf("/search") === 0) return;
        ev.preventDefault();
        location.href = "/search/?q=" + encodeURIComponent(input.value.trim());
      });
    }
  }
})();
