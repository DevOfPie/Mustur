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
  var agentsBox = document.querySelector(".agents");
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
    // The ages move on their own, so the server does not send a frame to move
    // them.
    if (!closed) drawAgents();
  }, 1000);

  // Sub-agent rows.
  //
  // The one thing this script models that is not the terminal. The owner chose
  // that on MUS-Q-0029, against the recommendation to leave the rows static —
  // so it is kept as small as it can be: the server sends the rows, this draws
  // them, and the only thing computed here is the age, from the stamps, so a
  // running sub-agent's clock moves without a frame per second to move it.
  var agents = null;

  function age(from, to) {
    var d = Math.max(0, Math.round((to ? to * 1000 : Date.now()) - from * 1000) / 1000);
    if (d < 60) return Math.round(d) + "s";
    if (d < 3600) return Math.round(d / 60) + "m";
    return Math.floor(d / 3600) + "h " + (Math.round(d / 60) % 60) + "m";
  }

  function el(tag, cls, txt) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (txt !== undefined) n.textContent = txt;
    return n;
  }

  function drawAgents() {
    if (!agentsBox || agents === null) return;
    if (!agents.length) {
      agentsBox.hidden = true;
      return;
    }
    agentsBox.hidden = false;
    var running = 0;
    var i;
    for (i = 0; i < agents.length; i++) if (!agents[i].done) running++;

    // Rebuilt rather than diffed. Four rows of three spans is not worth a
    // reconciler, and a rebuild cannot leave a stale row behind.
    agentsBox.textContent = "";
    agentsBox.appendChild(
      el("div", "count",
         agents.length + " sub-agent" + (agents.length === 1 ? "" : "s") +
         (running ? " · " + running + " running" : ""))
    );
    for (i = 0; i < agents.length; i++) {
      var a = agents[i];
      var row = el("div", "agent");
      row.appendChild(
        a.title ? el("span", "what", a.title) : el("span", "what untitled", a.type)
      );
      row.appendChild(el("span", "pill" + (a.done ? " done" : ""), a.state));
      row.appendChild(el("span", "age", age(a.started, a.ended)));
      agentsBox.appendChild(row);
      if (a.said) agentsBox.appendChild(el("p", "said", a.said));
    }
  }

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
        // The server says how long the session has already been silent, so the
        // counter continues rather than restarting at zero on every page load.
        if (typeof f.quiet === "number") lastOutput = Date.now() - f.quiet * 1000;
        if (scrollback) scrollback.textContent = "live";
      } else if (f.t === "out") {
        append(f.text || "");
        if (typeof f.seq === "number") seq = f.seq;
        lastOutput = Date.now();
      } else if (f.t === "gap") {
        // Told what was missed rather than shown a hole where it was. A zero
        // means the reader restarted while we were away and how much was
        // produced in between is not knowable — saying "some" is honest where
        // a number would not be.
        var lost = f.lostBytes || 0;
        append(
          lost > 0
            ? "\n[" + lost + " bytes of earlier output were not kept]\n"
            : "\n[output produced while this tab was away was not kept]\n"
        );
      } else if (f.t === "error") {
        // The server discarded a message and said so. The draft is put back,
        // because the alternative — which shipped — was the text vanishing and
        // a pill changing colour.
        append("\n[not sent: " + (f.error || "unknown reason") + "]\n");
        if (text && !text.value && lastSent) {
          text.value = lastSent;
          writeDraft(lastSent);
          grow();
          showKept();
        }
      } else if (f.t === "agents") {
        agents = f.agents || [];
        drawAgents();
      } else if (f.t === "ended") {
        closed = true;
        setState("ended", false);
        if (scrollback) scrollback.textContent = "session ended" + (f.at ? " " + f.at : "");
        if (foot) foot.textContent = "Nothing is running. Output is kept until you start another.";
        // The box stays writable: MUS-Q-0018 is that the composer is always
        // writable, and a dropped connection is exactly when someone is most
        // likely to be mid-sentence. Only the look dims.
        if (form) form.style.opacity = ".6";
        ws.close();
      }
    };

    ws.onclose = function () {
      if (closed) return;
      // A dropped connection is not a dropped session, and the label says so.
      setState("reconnecting", false);
      if (scrollback) scrollback.textContent = "reconnecting — the session kept running";
      // The box stays writable while the socket comes back: MUS-Q-0018 is that
      // the composer is always writable, and a dropped connection is exactly
      // when someone is most likely to be mid-sentence. Milestone 5's own row
      // says a draft survives a dropped connection, and a draft that cannot be
      // added to during one only half survives it.
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

  // The composer.
  //
  // Milestone 5's whole clause is that a draft survives a dropped connection
  // and a backgrounded phone, so the draft is written to localStorage on every
  // keystroke rather than on some event that a phone being backgrounded might
  // never deliver.
  //
  // **One draft, not one per session.** Thought first, destination second: what
  // is being written is a thought, and which session it goes to is a separate
  // choice that can change after it is written. Keying the draft per project
  // would lose it at exactly the moment the design exists to protect — the
  // owner deciding, mid-sentence, that this belongs somewhere else.
  var DRAFT = "mustur.draft";
  // The last message handed to the socket, kept until the server has had the
  // chance to say it could not deliver it.
  var lastSent = "";
  var kept = document.getElementById("kept");
  var dest = document.getElementById("dest");

  // localStorage throws rather than returning null in a private window, and a
  // composer that cannot save a draft must still be a composer.
  function readDraft() {
    try {
      return window.localStorage.getItem(DRAFT) || "";
    } catch (e) {
      return "";
    }
  }
  // Returns whether the browser actually kept it. A private window refuses,
  // and the indicator must not say "draft kept" when nothing is.
  var storable = true;
  function writeDraft(v) {
    try {
      if (v) window.localStorage.setItem(DRAFT, v);
      else window.localStorage.removeItem(DRAFT);
      storable = true;
    } catch (e) {
      storable = false;
    }
    return storable;
  }

  function grow() {
    if (!text) return;
    // The cap is 9rem in the stylesheet, so it is read from the stylesheet:
    // hardcoding 9 * 16 was right only for a reader whose root font-size is 16.
    var rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
    text.style.height = "auto";
    text.style.height = Math.min(text.scrollHeight, 9 * rem) + "px";
  }

  function showKept() {
    if (!kept) return;
    kept.hidden = !(storable && text.value.trim());
  }

  if (text) {
    var saved = readDraft();
    if (saved) text.value = saved;
    grow();
    showKept();
    text.addEventListener("input", function () {
      writeDraft(text.value);
      grow();
      showKept();
    });
    // Enter is a newline, because this is a composer and not a chat box. The
    // Send button is the phone's submit, and the keyboard shortcut is for the
    // desktop where a modifier is at hand.
    text.addEventListener("keydown", function (e) {
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        if (form) form.dispatchEvent(new Event("submit", { cancelable: true }));
      }
    });
  }

  if (dest && project) dest.textContent = "Send to " + project;

  // The draft is shared with the composer, so a thought started there and not
  // sent is still here — and the reply box says it is being kept for the same
  // reason that screen does.

  if (form) {
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (!ws || ws.readyState !== 1 || closed) return;
      var v = text.value.trim();
      if (!v) return;
      lastSent = v;
      ws.send(JSON.stringify({ t: "input", text: v }));
      // Cleared only once it is on the wire. A send that failed leaves the
      // draft where it was, which is the difference between a composer and a
      // thing that eats what you wrote.
      text.value = "";
      writeDraft("");
      grow();
      showKept();
    });
  }

  connect();
})();
