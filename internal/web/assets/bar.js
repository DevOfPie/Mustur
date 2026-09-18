// The counts in the tab bar, kept true on every surface.
//
// The bar is server-rendered once and a page can sit open for hours. The
// session view already learns about a change over the socket it has; every
// other surface showed the count it was rendered with until somebody navigated,
// and the owner missed a question being raised because of it (MUS-F-0086).
//
// So this is the one implementation of "put a number in a badge", and it has
// two callers: this file's poll, and the session view's socket. The session
// view had its own copy, which is how the fix ended up living on one surface.
//
// There are two badges. Decisions counts questions waiting on the owner;
// Records counts records needing attention (MUS-D-0193), and wears the warn
// tone so the two are never read as one number. Each is polled from its own
// count and written by the same function.
(function () {
  "use strict";

  // How often a page asks. Ten seconds is the latency on a badge, not on
  // anything anybody is waiting for, and the server caches each answer so a
  // handful of open tabs cost one count between them.
  var EVERY = 10000;

  // Each badge: the tab it sits on, the count that feeds it, and the class
  // the server renders it with.
  var BADGES = [
    { tab: "/questions", count: "/questions/count", cls: "cnt" },
    { tab: "/records", count: "/records/attention/count", cls: "cnt att" }
  ];

  function link(b) {
    return document.querySelector('nav a[href="' + b.tab + '"]');
  }

  // Absent rather than empty when nothing is waiting, because that is how the
  // server renders it and one shape is easier to style than two.
  function write(b, n) {
    var a = link(b);
    if (!a) return;
    var cnt = a.querySelector(".cnt");
    if (!n) {
      if (cnt && cnt.parentNode) cnt.parentNode.removeChild(cnt);
      return;
    }
    if (!cnt) {
      cnt = document.createElement("em");
      cnt.className = b.cls;
      a.appendChild(cnt);
    }
    cnt.textContent = String(n);
  }

  // Exposed so the session view's socket sets the Decisions badge through the
  // same code rather than through a second copy of it. Its contract is
  // unchanged: one number, the decisions waiting.
  window.musturBadge = function (n) {
    write(BADGES[0], n);
  };

  // A page with no bar has nothing to keep true.
  if (!link(BADGES[0])) return;

  var stop = false;
  function ask(b) {
    return fetch(b.count, { credentials: "same-origin" })
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
        if (j && typeof j.waiting === "number") write(b, j.waiting);
      })
      .catch(function () {
        // Offline, or the server is restarting. The next tick tries again.
      });
  }
  function poll() {
    if (stop) return;
    for (var i = 0; i < BADGES.length; i++) ask(BADGES[i]);
  }

  // Once on load as well as on the timer. The server has just rendered the
  // counts into this page, so the first ask is nearly always the same answer
  // -- but a page restored from the back/forward cache was rendered a while
  // ago, and waiting ten seconds to correct it is the defect this file exists
  // for.
  poll();
  setInterval(poll, EVERY);
  // Asked on return rather than only on a timer: a phone that was in a pocket
  // for an hour should not show an hour-old count for ten more seconds.
  document.addEventListener("visibilitychange", function () {
    if (!document.hidden) poll();
  });
})();
