// The only script in Mustur. Every other surface is server-rendered with no
// script at all, and that stays the rule — a live terminal is the one thing
// that cannot be served as a page.
//
// What it does and nothing more: hold one socket, append output, remember the
// byte offset it last saw, and reconnect from there. No framework, no build
// step, no dependency. It is served from the binary.
(function () {
  "use strict";

  var project = document.body.getAttribute("data-project");
  var out = document.getElementById("out");
  if (!project || !out) return;

  var state = document.getElementById("state");
  var scrollback = document.getElementById("scrollback");
  var foot = document.getElementById("foot");
  var form = document.getElementById("say");
  var text = document.getElementById("text");

  // The byte offset last seen. Reconnecting asks to resume from here, which is
  // what makes a dropped connection lose nothing rather than start over.
  var seq = 0;
  var lastOutput = Date.now();
  var ws = null;
  var retry = 0;
  var closed = false;

  function setState(label, on) {
    state.textContent = label;
    state.className = on ? "pill on" : "pill";
  }

  function atBottom() {
    return out.scrollHeight - out.scrollTop - out.clientHeight < 40;
  }

  function append(s) {
    var stick = atBottom();
    out.textContent += s;
    // Trimming the DOM as well as the server buffer: a tab left open for a day
    // should not hold a megabyte of text the server has already forgotten.
    if (out.textContent.length > 262144) {
      out.textContent = out.textContent.slice(-262144);
    }
    if (stick) out.scrollTop = out.scrollHeight;
  }

  function quiet() {
    var s = Math.floor((Date.now() - lastOutput) / 1000);
    // Time since the last output and nothing more. Whether the session is
    // waiting for input or thinking hard is not knowable from here, so this
    // never says.
    if (s < 60) return "quiet " + s + "s";
    if (s < 3600) return "quiet " + Math.floor(s / 60) + "m";
    return "quiet " + Math.floor(s / 3600) + "h";
  }

  setInterval(function () {
    if (foot && !closed) foot.textContent = quiet();
  }, 1000);

  function connect() {
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    var url =
      proto + "//" + location.host + "/sessions/" +
      encodeURIComponent(project) + "/ws?from=" + seq;

    ws = new WebSocket(url);

    ws.onopen = function () {
      retry = 0;
      setState("running", true);
      if (form) form.style.opacity = "1";
      if (text) text.disabled = false;
    };

    ws.onmessage = function (ev) {
      var f;
      try {
        f = JSON.parse(ev.data);
      } catch (e) {
        return;
      }
      if (f.t === "hello") {
        if (typeof f.seq === "number") seq = f.seq;
        if (scrollback) scrollback.textContent = "live";
      } else if (f.t === "out") {
        append(f.text || "");
        if (typeof f.seq === "number") seq = f.seq;
        lastOutput = Date.now();
      } else if (f.t === "gap") {
        // Told what was missed rather than shown a hole where it was.
        append(
          "\n[" + (f.lostBytes || 0) + " bytes of output are older than the " +
            "buffer and were not kept]\n"
        );
      } else if (f.t === "ended") {
        closed = true;
        setState("ended", false);
        if (scrollback) scrollback.textContent = "session ended" + (f.at ? " " + f.at : "");
        if (foot) foot.textContent = "Nothing is running. Output is kept until you start another.";
        if (text) text.disabled = true;
        if (form) form.style.opacity = ".5";
        ws.close();
      }
    };

    ws.onclose = function () {
      if (closed) return;
      // A dropped connection is not a dropped session, and the label says so.
      setState("reconnecting", false);
      if (scrollback) scrollback.textContent = "reconnecting — the session kept running";
      if (text) text.disabled = true;
      if (form) form.style.opacity = ".5";
      retry = Math.min(retry + 1, 6);
      setTimeout(connect, Math.min(500 * Math.pow(2, retry - 1), 15000));
    };

    ws.onerror = function () {
      try {
        ws.close();
      } catch (e) {}
    };
  }

  if (form) {
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (!ws || ws.readyState !== 1 || closed) return;
      var v = text.value.trim();
      if (!v) return;
      ws.send(JSON.stringify({ t: "input", text: v }));
      text.value = "";
    });
  }

  connect();
})();
