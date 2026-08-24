// The composer's client layer — the second script in this repository, and the
// first addition to a rule that used to be a count of one.
//
// The owner took that on MUS-Q-0034 for one reason: a draft that does not
// survive a backgrounded phone is not a draft, and nothing server-rendered can
// keep it. So this does that and stops. The form posts and works with the
// script blocked; everything here is an improvement on a page that already
// functions without it.
(function () {
  "use strict";

  var text = document.getElementById("text");
  if (!text) return;
  var kept = document.getElementById("kept");
  var form = text.form;

  // One draft, and the same key the session view's reply box uses. What is
  // being written is a thought, and which page it was started on should not
  // decide whether it is still there.
  var DRAFT = "mustur.draft";

  // read returns null when the browser refuses, which is not the same as an
  // empty draft: the indicator must not claim a draft is kept when nothing is.
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

  // Whether this browser will keep anything at all, established by trying once
  // rather than by assuming. A private window says no, and the header then says
  // nothing rather than a comforting lie.
  var storable = read() !== null && write(read());

  function show() {
    if (kept) kept.hidden = !(storable && text.value.trim());
  }

  // A draft handed back by the server after a failed send wins over the stored
  // one: it is the text the owner just tried to send.
  var saved = read();
  if (!text.value && saved) text.value = saved;
  show();

  text.addEventListener("input", function () {
    storable = write(text.value);
    show();
  });

  // Ctrl or Cmd with Enter sends, for the desktop where a modifier is at hand.
  // Plain Enter is a newline, because this is a composer and not a chat box.
  text.addEventListener("keydown", function (e) {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && form) {
      if (form.requestSubmit) form.requestSubmit();
      else form.submit();
    }
  });

  // The destination pills are labels wrapping radios, so the browser does the
  // choosing and this only moves the highlight.
  var routes = document.querySelectorAll(".routes label");
  Array.prototype.forEach.call(routes, function (label) {
    label.addEventListener("click", function () {
      Array.prototype.forEach.call(routes, function (other) {
        other.className = other === label ? "on" : "";
      });
    });
  });

  // Cleared only once the server says it went. The page comes back with sent=
  // in the query string on success and with the draft on failure, so a message
  // that did not arrive is still here to try again.
  if (/[?&]sent=/.test(location.search)) write("");
})();
