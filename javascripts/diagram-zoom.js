// Diagram controls for mkdocs-material mermaid diagrams.
//
// mkdocs-material replaces <pre class="mermaid"> with <div class="mermaid">
// containing a CLOSED shadow DOM with the SVG inside. We cannot access the
// SVG directly, so we operate on the outer <div> element.
//
// Key design decisions:
// - Wrap each div.mermaid in a .diagram-wrapper for scroll containment
// - Use CSS "zoom" (not transform:scale) so layout reflows on zoom
// - Fullscreen MOVES the original div (not clone) to preserve shadow DOM
(function () {
  var ZOOM_STEP = 0.25;
  var MIN_ZOOM = 0.5;
  var MAX_ZOOM = 5;

  function addControls(div) {
    if (div._hasControls) return;
    if (div.tagName !== "DIV") return;
    div._hasControls = true;
    div._zoomLevel = 1;

    // Wrap in clipping container (overflow:hidden by default, fullscreen for detail)
    var wrapper = document.createElement("div");
    wrapper.className = "diagram-wrapper";
    div.parentNode.insertBefore(wrapper, div);
    wrapper.appendChild(div);

    var bar = document.createElement("div");
    bar.className = "diagram-actions";
    bar.innerHTML =
      '<button data-action="zoom-in" title="Zoom in">&#43;</button>' +
      '<button data-action="zoom-out" title="Zoom out">&#8722;</button>' +
      '<button data-action="reset" title="Reset zoom">1:1</button>' +
      '<button data-action="fullscreen" title="View full screen">&#9974;</button>';
    wrapper.after(bar);
  }

  function scanAll() {
    document.querySelectorAll("div.mermaid").forEach(addControls);
  }

  function openFullscreen(mermaidDiv) {
    var overlay = document.createElement("div");
    overlay.className = "diagram-overlay";

    var toolbar = document.createElement("div");
    toolbar.className = "diagram-overlay-toolbar";
    toolbar.innerHTML =
      '<button data-action="ol-zoom-in" title="Zoom in">&#43;</button>' +
      '<button data-action="ol-zoom-out" title="Zoom out">&#8722;</button>' +
      '<button data-action="ol-reset" title="Reset">1:1</button>' +
      '<button data-action="ol-close" title="Close">&#10005;</button>';
    overlay.appendChild(toolbar);

    var container = document.createElement("div");
    container.className = "diagram-overlay-content";

    // Insert placeholder so we can restore the div later
    var wrapper = mermaidDiv.parentNode;
    var placeholder = document.createElement("div");
    placeholder.style.display = "none";
    wrapper.insertBefore(placeholder, mermaidDiv);

    // Save original inline styles
    var origStyle = mermaidDiv.style.cssText;

    // MOVE (not clone) the div so the closed shadow DOM stays attached
    mermaidDiv.style.cssText =
      "max-width:none; max-height:none; overflow:visible; zoom:1;";
    container.appendChild(mermaidDiv);
    overlay.appendChild(container);
    document.body.appendChild(overlay);

    // Auto-scale to fill the viewport
    var naturalW = mermaidDiv.scrollWidth || mermaidDiv.offsetWidth;
    var naturalH = mermaidDiv.scrollHeight || mermaidDiv.offsetHeight;
    var availW = container.clientWidth - 64;
    var availH = container.clientHeight - 32;
    var fitZoom = 1;
    if (naturalW > 0 && naturalH > 0) {
      fitZoom = Math.min(availW / naturalW, availH / naturalH, 3);
      fitZoom = Math.max(fitZoom, 0.5);
    }
    mermaidDiv.style.zoom = fitZoom;
    var olZoom = fitZoom;

    function close() {
      mermaidDiv.style.cssText = origStyle;
      wrapper.insertBefore(mermaidDiv, placeholder);
      placeholder.remove();
      overlay.remove();
      document.removeEventListener("keydown", escHandler);
    }

    function escHandler(e) {
      if (e.key === "Escape") close();
    }
    document.addEventListener("keydown", escHandler);
    overlay.addEventListener("click", function (e) {
      if (e.target === overlay || e.target === container) close();
    });

    toolbar.addEventListener("click", function (e) {
      var btn = e.target.closest("button");
      if (!btn) return;
      var action = btn.dataset.action;
      if (action === "ol-close") {
        close();
        return;
      }
      if (action === "ol-zoom-in")
        olZoom = Math.min(olZoom + ZOOM_STEP, MAX_ZOOM);
      if (action === "ol-zoom-out")
        olZoom = Math.max(olZoom - ZOOM_STEP, MIN_ZOOM);
      if (action === "ol-reset") olZoom = 1;
      mermaidDiv.style.zoom = olZoom === 1 ? "" : olZoom;
    });
  }

  // Click delegation (capture phase)
  document.addEventListener(
    "click",
    function (e) {
      var btn = e.target.closest(".diagram-actions button");
      if (!btn) return;
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();

      var bar = btn.closest(".diagram-actions");
      // Action bar is sibling AFTER the wrapper
      var wrapper = bar.previousElementSibling;
      if (!wrapper || !wrapper.classList.contains("diagram-wrapper")) return;
      var mermaidDiv = wrapper.querySelector("div.mermaid");
      if (!mermaidDiv) return;

      var action = btn.dataset.action;

      if (action === "fullscreen") {
        openFullscreen(mermaidDiv);
        return;
      }

      if (!mermaidDiv._zoomLevel) mermaidDiv._zoomLevel = 1;
      if (action === "zoom-in")
        mermaidDiv._zoomLevel = Math.min(
          mermaidDiv._zoomLevel + ZOOM_STEP,
          MAX_ZOOM
        );
      if (action === "zoom-out")
        mermaidDiv._zoomLevel = Math.max(
          mermaidDiv._zoomLevel - ZOOM_STEP,
          MIN_ZOOM
        );
      if (action === "reset") mermaidDiv._zoomLevel = 1;

      // CSS zoom affects layout (unlike transform:scale which causes overlap)
      mermaidDiv.style.zoom =
        mermaidDiv._zoomLevel === 1 ? "" : mermaidDiv._zoomLevel;

      // Allow scrolling when zoomed beyond natural size
      if (mermaidDiv._zoomLevel > 1) {
        wrapper.style.overflow = "auto";
      } else {
        wrapper.style.overflow = "";
      }
    },
    true
  );

  // Poll for rendered diagrams (mkdocs-material renders asynchronously)
  var pollCount = 0;
  var pollId = setInterval(function () {
    scanAll();
    if (++pollCount > 30) clearInterval(pollId);
  }, 1000);

  // MutationObserver as backup (throttled)
  var raf = false;
  new MutationObserver(function () {
    if (raf) return;
    raf = true;
    requestAnimationFrame(function () {
      raf = false;
      scanAll();
    });
  }).observe(document.body, { childList: true, subtree: true });

  // mkdocs-material instant navigation
  if (typeof document$ !== "undefined") {
    document$.subscribe(function () {
      pollCount = 0;
      pollId = setInterval(function () {
        scanAll();
        if (++pollCount > 30) clearInterval(pollId);
      }, 1000);
    });
  }
})();
