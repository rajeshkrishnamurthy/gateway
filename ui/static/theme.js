(function () {
  var storageKey = "services-health-theme";
  var root = document.documentElement;
  var toggle = document.getElementById("theme-toggle");

  if (!toggle) {
    return;
  }

  function applyTheme(theme) {
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

  var initial = stored || (prefersDark ? "dark" : "light");
  applyTheme(initial);

  toggle.addEventListener("click", function () {
    var next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    try {
      window.localStorage.setItem(storageKey, next);
    } catch (err) {
    }
    applyTheme(next);
  });
})();
