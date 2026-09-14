// The intake box's draft, and nothing else.
//
// MUS-F-0152: the owner leaves the box to look something up in a session or
// the queue, and came back to an empty box. The owner took this on MUS-Q-0120,
// modelled on the composer: the text is kept per browser under its own key,
// and let go on filing or on Clear. Pictures are not kept. The form files with
// this blocked; the Clear row is hidden in the markup and shown from here.
(function () {
  "use strict";

  var text = document.getElementById("jot");
  if (!text) return;
  var row = document.getElementById("draft");
  var kept = document.getElementById("kept");
  var clear = document.getElementById("clear");

  // Not the composer's key. A report half-typed here is not a message to a
  // session, and turning up in that box would invite sending it to one.
  var DRAFT = "mustur.intake.draft";

  // null when the browser refuses, which is not an empty draft.
  function read() {
    try {
      return window.localStorage.getItem(DRAFT) || "";
    } catch (e) {
      return null;
    }
  }
  function write(v) {
    try {
      if (v) window.localStorage.setItem(DRAFT, v);
      else window.localStorage.removeItem(DRAFT);
      return true;
    } catch (e) {
      return false;
    }
  }
  var storable = read() !== null && write(read());

  function show() {
    var has = !!text.value.trim();
    if (row) row.hidden = !has;
    if (kept) kept.hidden = !storable;
  }

  // The server marks the box when a jot was filed on the way here. Cleared
  // before anything is restored, and done= comes off the address, so going
  // back to this page after typing a new draft does not throw that one away.
  if (text.hasAttribute("data-filed")) {
    write("");
    if (window.history.replaceState && /[?&]done=/.test(location.search)) {
      var q = location.search.slice(1).split("&").filter(function (p) {
        return p.indexOf("done=") !== 0;
      }).join("&");
      history.replaceState(null, "", location.pathname + (q ? "?" + q : "") + location.hash);
    }
  } else if (!text.value) {
    // Text the server handed back after a refusal wins: it is what was just
    // tried, and it is kept from here on as the draft.
    var saved = read();
    if (saved) text.value = saved;
  } else {
    storable = write(text.value);
  }
  show();

  text.addEventListener("input", function () {
    storable = write(text.value);
    show();
  });

  if (clear) {
    clear.addEventListener("click", function () {
      text.value = "";
      write("");
      show();
      text.focus();
    });
  }
})();
