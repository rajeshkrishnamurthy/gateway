(function () {
  var storageKey = "services-health-theme";
  var root = document.documentElement;

  function applyTheme(theme, toggle) {
    if (theme === "dark") {
      root.setAttribute("data-theme", "dark");
      toggle.classList.remove("is-off");
      toggle.classList.add("is-on");
      toggle.setAttribute("aria-checked", "true");
      toggle.textContent = "Dark";
      return;
    }

    root.removeAttribute("data-theme");
    toggle.classList.remove("is-on");
    toggle.classList.add("is-off");
    toggle.setAttribute("aria-checked", "false");
    toggle.textContent = "Light";
  }

  function resolveTheme() {
    var stored = null;
    try {
      stored = window.localStorage.getItem(storageKey);
    } catch (err) {
      stored = null;
    }

    var prefersDark = false;
    if (window.matchMedia) {
      prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    }

    return stored || (prefersDark ? "dark" : "light");
  }

  function initThemeToggle() {
    var toggle = document.getElementById("theme-toggle");
    if (!toggle) {
      return;
    }
    if (toggle.dataset.themeBound === "true") {
      applyTheme(resolveTheme(), toggle);
      return;
    }
    toggle.dataset.themeBound = "true";
    applyTheme(resolveTheme(), toggle);
    toggle.addEventListener("click", function () {
      var next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
      try {
        window.localStorage.setItem(storageKey, next);
      } catch (err) {
      }
      applyTheme(next, toggle);
    });
  }

  document.addEventListener("htmx:afterSwap", function (event) {
    if (event && event.target && event.target.id === "ui-root") {
      initThemeToggle();
    }
  });

  initThemeToggle();
})();
