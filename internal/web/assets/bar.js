// The count in the tab bar, kept true on every surface.
//
// The bar is server-rendered once and a page can sit open for hours. The
// session view already learns about a change over the socket it has; every
// other surface showed the count it was rendered with until somebody navigated,
// and the owner missed a question being raised because of it (MUS-F-0086).
//
// So this is the one implementation of "put a number in the badge", and it has
// two callers: this file's poll, and the session view's socket. The session
// view had its own copy, which is how the fix ended up living on one surface.
(function () {
  "use strict";

  // How often a page asks. Ten seconds is the latency on a badge, not on
  // anything anybody is waiting for, and the server caches the answer so a
  // handful of open tabs cost one count between them.
  var EVERY = 10000;

  function link() {
    return document.querySelector('nav a[href="/questions"]');
  }

  // Absent rather than empty when nothing is waiting, because that is how the
  // server renders it and one shape is easier to style than two.
  function set(n) {
    var a = link();
    if (!a) return;
    var cnt = a.querySelector(".cnt");
    if (!n) {
      if (cnt && cnt.parentNode) cnt.parentNode.removeChild(cnt);
      return;
    }
    if (!cnt) {
      cnt = document.createElement("em");
      cnt.className = "cnt";
      a.appendChild(cnt);
    }
    cnt.textContent = String(n);
  }

  // Exposed so the session view's socket sets the badge through the same code
  // rather than through a second copy of it.
  window.musturBadge = set;

  // A page with no bar has nothing to keep true.
  if (!link()) return;

  var stop = false;
  function poll() {
    if (stop) return;
    fetch("/questions/count", { credentials: "same-origin" })
      .then(function (r) {
        // A 401 or 403 means the session went away while the tab sat there.
        // Stop asking: the badge is the least of it, and a page that polls a
        // sign-in screen every ten seconds is a page doing harm.
        if (r.status === 401 || r.status === 403) {
          stop = true;
          return null;
        }
        return r.ok ? r.json() : null;
      })
      .then(function (j) {
        if (j && typeof j.waiting === "number") set(j.waiting);
      })
      .catch(function () {
        // Offline, or the server is restarting. The next tick tries again.
      });
  }

  // Once on load as well as on the timer. The server has just rendered the
  // count into this page, so the first ask is nearly always the same answer --
  // but a page restored from the back/forward cache was rendered a while ago,
  // and waiting ten seconds to correct it is the defect this file exists for.
  poll();
  setInterval(poll, EVERY);
  // Asked on return rather than only on a timer: a phone that was in a pocket
  // for an hour should not show an hour-old count for ten more seconds.
  document.addEventListener("visibilitychange", function () {
    if (!document.hidden) poll();
  });
})();
