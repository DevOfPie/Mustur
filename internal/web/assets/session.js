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
  var agentsBox = document.getElementById("dlist");
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

  // Follow the tail unless the reader has scrolled up to look at something.
  //
  // This measures #out rather than the page, and always did — but until
  // MUS-F-0032 the pane had no overflow, so it never scrolled, scrollTop was
  // always 0 and this always answered true. The script was written for the
  // shell the stylesheet described and never built. Now that #out owns its own
  // scroll track, it starts doing what it says.
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
    var running = 0;
    var i;
    for (i = 0; i < agents.length; i++) if (!agents[i].done) running++;
    badge(agents.length, running);

    // Rebuilt rather than diffed. A handful of rows is not worth a reconciler,
    // and a rebuild cannot leave a stale row behind.
    agentsBox.textContent = "";
    if (!agents.length) {
      agentsBox.appendChild(
        el("p", "none", "Nothing has been launched from this session.")
      );
      shutRead();
      return;
    }
    for (i = 0; i < agents.length; i++) {
      var a = agents[i];
      var row = el("button", "agent");
      row.type = "button";
      row.dataset.id = a.id;
      row.appendChild(
        a.title ? el("span", "what", a.title) : el("span", "what untitled", a.type)
      );
      row.appendChild(el("span", "pill" + (a.done ? " done" : ""), a.state));
      row.appendChild(el("span", "age", age(a.started, a.ended)));
      row.appendChild(el("span", "more", "\u203a"));
      agentsBox.appendChild(row);
      // Out of view, not out of the page: the reading pane reads from here, so
      // it shows the same text whether or not a frame has arrived yet.
      if (a.said) {
        var say = el("div", "say", a.said);
        say.setAttribute("data-for", a.id);
        agentsBox.appendChild(say);
      }
    }
    // The rows were just thrown away, so anything being read is now pointing
    // at a button that no longer exists. Re-read it rather than close it: a
    // sub-agent finishing while you read it should fill the pane in.
    if (openID && !fill(openID)) shutRead();
  }

  // The strip.
  //
  // The badge counts what is running and falls back to the total, because a
  // count of only the active ones goes blank the moment they all finish —
  // which is when their reports are worth reading, and with the drawer shut
  // nothing else says they exist. The ring turns only while something runs.
  var ring = document.getElementById("ring");
  var badgeEl = document.getElementById("badge");
  var toggle = document.getElementById("toggle");
  var dcount = document.getElementById("dcount");

  function badge(total, running) {
    if (badgeEl) {
      badgeEl.hidden = !total;
      badgeEl.textContent = String(running || total || "");
    }
    if (ring) ring.classList.toggle("live", running > 0);
    if (toggle) {
      if (total) toggle.removeAttribute("data-empty");
      else toggle.setAttribute("data-empty", "");
    }
    if (dcount) {
      dcount.textContent = total
        ? total + (running ? " \u00b7 " + running + " running" : "")
        : "";
    }
  }

  // The drawer.
  //
  // Shut on every load. The owner chose that over remembering it (MUS-Q-0057):
  // shut by default means shut, and the badge is what says whether opening it
  // is worth it. The pushed class does nothing below 60rem — the stylesheet
  // only acts on it on a wide screen, where the drawer takes a column instead
  // of covering one.
  var drawer = document.getElementById("drawer");
  var openID = null;

  function openDrawer(yes) {
    if (!drawer) return;
    drawer.hidden = !yes;
    document.body.classList.toggle("pushed", yes);
    if (toggle) toggle.setAttribute("aria-expanded", yes ? "true" : "false");
    // The composer is placed by custom properties rather than by flow, so its
    // height has to be re-measured once the column it sits in has changed
    // width and its text may have rewrapped.
    measureDock();
    if (yes) {
      var shutBtn = document.getElementById("shut");
      if (shutBtn) shutBtn.focus();
    } else {
      shutRead();
      if (toggle) toggle.focus();
    }
  }

  // Reading one sub-agent, in the same drawer rather than a second layer over
  // it. Everything shown is read back out of the row it was opened from: one
  // code path for the server's first paint and every rebuild after it, so a
  // tap before the first frame is answered the same as one after it.
  function fill(id) {
    var row = agentsBox && agentsBox.querySelector('.agent[data-id="' + id + '"]');
    if (!row) return false;
    var what = row.querySelector(".what");
    var pill = row.querySelector(".pill");
    var ageTxt = row.querySelector(".age");
    var done = !!(pill && pill.classList.contains("done"));

    var title = document.getElementById("dtitle");
    title.textContent = what ? what.textContent : "";
    title.className = what && what.classList.contains("untitled") ? "untitled" : "";

    var meta = document.getElementById("dmeta");
    meta.hidden = false;
    meta.textContent =
      (pill ? pill.textContent : "") +
      (ageTxt ? " \u00b7 " + (done ? "ran " : "running ") + ageTxt.textContent : "");

    var say = agentsBox.querySelector('.say[data-for="' + id + '"]');
    var read = document.getElementById("dread");
    if (say) {
      read.textContent = say.textContent;
      read.className = "dread";
    } else {
      // Nothing said yet. What it is doing and no more (MUS-Q-0056): a
      // sub-agent is a call inside the CLI's own process, so there is no
      // second pane to stream, and inventing one would mean the hook carrying
      // output — which milestone 4c deliberately did not build.
      var state = pill ? pill.textContent : "";
      read.textContent = done
        ? "It finished without a final message."
        : "Nothing said yet \u2014 it is " +
          (state === "working" ? "between tool calls" : "in " + state) + ".";
      read.className = "dread quiet";
    }
    read.hidden = false;
    agentsBox.hidden = true;
    document.getElementById("back").hidden = false;
    if (dcount) dcount.hidden = true;
    openID = id;
    return true;
  }

  function shutRead() {
    if (!drawer) return;
    openID = null;
    var read = document.getElementById("dread");
    if (read) read.hidden = true;
    var meta = document.getElementById("dmeta");
    if (meta) meta.hidden = true;
    if (agentsBox) agentsBox.hidden = false;
    var back = document.getElementById("back");
    if (back) back.hidden = true;
    if (dcount) dcount.hidden = false;
    var title = document.getElementById("dtitle");
    if (title) {
      title.textContent = "Sub-agents";
      title.className = "";
    }
  }

  if (toggle) {
    toggle.addEventListener("click", function () {
      openDrawer(drawer.hidden);
    });
  }
  if (agentsBox) {
    // Delegated, because drawAgents throws the rows away and rebuilds them.
    agentsBox.addEventListener("click", function (e) {
      var row = e.target.closest && e.target.closest(".agent");
      if (row && row.dataset.id) fill(row.dataset.id);
    });
  }
  if (drawer) {
    document.getElementById("shut").addEventListener("click", function () {
      openDrawer(false);
    });
    document.getElementById("back").addEventListener("click", shutRead);
    document.getElementById("veil").addEventListener("click", function () {
      openDrawer(false);
    });
    document.addEventListener("keydown", function (e) {
      if (e.key !== "Escape" || drawer.hidden) return;
      // One step at a time: out of the reading pane first, then out of the
      // drawer. Escape emptying the whole thing in one press loses your place
      // in a list you may have scrolled.
      if (openID) shutRead();
      else openDrawer(false);
    });
  }

  // The picker navigates on change. Nothing here touches the submit button
  // beside it, because there is none to touch: it lives in a noscript element,
  // so if this line is running the browser has already left it out. The first
  // version drew it and hid it from here, and a control the server draws and
  // the script removes is one that can fail visible — which is how the owner
  // met it, on a stale page holding new markup beside old script.
  var picker = document.getElementById("pick");
  if (picker) {
    picker.addEventListener("change", function () {
      if (picker.value) location.href = "/sessions/" + encodeURIComponent(picker.value);
    });
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
    measureDock();
  }

  // The dock is locked to the bottom of the screen and the output runs behind
  // it, so the output needs to know how tall it is or its last lines come to
  // rest underneath. CSS cannot measure a sibling; this can, and the composer
  // changes height as it is typed into.
  function measureDock() {
    var dock = document.querySelector(".dock");
    if (!dock) return;
    var stick = atBottom();
    document.body.style.setProperty("--dock-h", dock.offsetHeight + "px");
    // Keeping the tail in view: growing the dock shortens the visible output,
    // which would otherwise silently scroll the newest line out of sight.
    if (stick) out.scrollTop = out.scrollHeight;
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
      // The box stays writable while the socket is down, so Send in that window
      // has to say something. It used to return silently, which is the failure
      // the error channel was added to end.
      if (!ws || ws.readyState !== 1) {
        if (!closed) append("\n[not sent: still reconnecting. What you wrote is kept.]\n");
        return;
      }
      if (closed) return;
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

  // Once at load, and again whenever the viewport changes shape — a phone's
  // URL bar sliding away is a resize, and it moves the dock.
  measureDock();
  window.addEventListener("resize", measureDock);
  if (window.visualViewport) {
    window.visualViewport.addEventListener("resize", measureDock);
  }

  connect();
})();
