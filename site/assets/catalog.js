// Keep displayed versions in sync with the same catalog consumed by the Store.
(async () => {
  const path = document.body.dataset.catalogPath;
  if (!path) return;
  try {
    const response = await fetch(path, { cache: "no-cache" });
    if (!response.ok) return;
    const { apps } = await response.json();
    for (const app of apps) {
      document.querySelectorAll(`[data-version="${app.id}"]`).forEach(el => {
        el.textContent = `v${app.version}`;
      });
    }
  } catch (_) {
    // Static fallback versions remain visible offline.
  }
})();
