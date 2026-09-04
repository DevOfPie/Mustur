// The only script in Mustur. Every other surface is server-rendered with no
// script at all, and that stays the rule — a live terminal is the one thing
// that cannot be served as a page.
//
// What it does and nothing more: hold one socket, paint the screen the server
// sends, and reconnect when it drops. No framework, no build step, no
// dependency. It is served from the binary.
//
// It used to append bytes and remember a byte offset. It does not any more:
// the server sends the pane as tmux has already assembled it, so a frame is a
// whole screen and resuming is just being sent the current one (MUS-Q-0060).
(function () {
  "use strict";

  var project = document.body.getAttribute("data-project");
  var out = document.getElementById("out");
  if (!project || !out) return;

  var state = document.getElementById("state");
  var agentsBox = document.getElementById("dlist");
  var foot = document.getElementById("foot");
  var form = document.getElementById("say");
  var text = document.getElementById("text");

  // When the screen last changed. There is no offset to remember: the unit is
  // a frame, and a reconnect is handed the screen as it stands.
  var lastOutput = Date.now();
  var ws = null;
  var retry = 0;
  var closed = false;

  var stateRing = document.getElementById("statering");

  // What the CLI's own pane says it is doing, when it can be read at all.
  //
  // The owner pointed out that the timer below is a guess standing in for a
  // fact the CLI already prints: a tool call that produces nothing for two
  // minutes is working, and a session that finished four seconds ago is not,
  // and counting silence cannot tell those apart. So this wins when it is
  // known, and the timer is what happens when it is not.
  var doing = "";

  // How long a session has to be silent before the pill stops calling it
  // running.
  //
  // Three minutes. An agent thinking hard goes quiet for tens of seconds, and
  // a session waiting for you goes quiet for as long as you leave it — so the
  // threshold has to sit well past the first without being so far past it that
  // "running" stays on a session that has been waiting since breakfast. It is
  // one number in one place if that turns out to be wrong.
  var IDLE_AFTER = 180;

  // Whether the socket is up and the session has not ended. Only then does the
  // pill have a choice to make between running and idle.
  var attached = false;

  // How many decisions are waiting, in the bar this page shares with every
  // other surface.
  //
  // The bar is server-rendered once and this page outlives that render by
  // hours -- a question raised while somebody is watching a session left the
  // badge saying what was true when the tab opened (MUS-F-0069). The count is
  // pushed down the socket that is already open rather than polled by a second
  // request, which is the shape the owner chose for sub-agent rows on
  // MUS-Q-0029; this is the second thing the client layer models that is not
  // the terminal, where MUS-D-0092 recorded it as the only one.
  //
  // The element is absent rather than empty when nothing is waiting, because
  // that is how the server renders it and one shape is easier to style than
  // two.
  function setWaiting(n) {
    var link = document.querySelector('nav a[href="/questions"]');
    if (!link) return;
    var cnt = link.querySelector(".cnt");
    if (!n) {
      if (cnt && cnt.parentNode) cnt.parentNode.removeChild(cnt);
      return;
    }
    if (!cnt) {
      cnt = document.createElement("em");
      cnt.className = "cnt";
      link.appendChild(cnt);
    }
    cnt.textContent = String(n);
  }

  function setState(label, on) {
    state.textContent = label;
    state.className = on ? "pill on" : "pill";
    // The ring turns only while something is actually happening. Reconnecting,
    // ended and idle are all states where a moving light would be saying the
    // opposite of the word beside it.
    if (stateRing) stateRing.classList.toggle("live", !!on);
  }

  // The pill says running or idle; the counter below says how long. Deliberately
  // not "idle 12m" — the strip that used to say what the pill already said was
  // removed for exactly that reason.
  function refreshState() {
    if (!attached || closed) return;
    if (doing === "working") {
      setState("running", true);
      return;
    }
    if (doing === "waiting") {
      setState("idle", false);
      return;
    }
    // Nothing here could read the pane, so fall back to counting silence.
    var quietFor = Math.floor((Date.now() - lastOutput) / 1000);
    var idle = quietFor >= IDLE_AFTER;
    setState(idle ? "idle" : "running", !idle);
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

  // The CLI's status line, as a row of chips.
  //
  // It used to be four lines at the bottom of the output — an input box, two
  // dividers and a status line — redrawn every frame and read by nobody. The
  // server takes them off the screen and sends what they said; this draws it.
  var chips = document.getElementById("chips");

  function drawChips(st) {
    if (!chips) return;
    chips.textContent = "";
    if (!st) {
      chips.hidden = true;
      return;
    }
    var add = function (text, cls) {
      if (!text) return;
      var el = document.createElement("span");
      if (cls) el.className = cls;
      el.textContent = text;
      chips.appendChild(el);
    };
    add(st.mode, "mode");
    (st.items || []).forEach(function (i) { add(i); });
    add(st.note, "note");
    add(st.update, "hint");
    add(st.hint, "hint");
    chips.hidden = !chips.firstChild;
    // The row appears and disappears, and the composer is placed by a custom
    // property rather than by flow, so its reservation has to be re-measured.
    measureDock();
  }

  // Paint a whole screen.
  //
  // Replaced rather than appended, which is the whole change: a repainting
  // terminal stacked on itself was what made the old view unreadable. Nothing
  // accumulates, so nothing needs trimming either.
  //
  // The HTML is the server's — every character of the pane was escaped there,
  // and the only markup in it is the spans it wrote for colour.
  function paint(html) {
    var stick = atBottom();
    out.innerHTML = html;
    if (stick) out.scrollTop = out.scrollHeight;
  }

  // Something Mustur has to say about the session, as opposed to something the
  // session said. Appended under the screen rather than into it, because the
  // screen is replaced wholesale and anything written into it would vanish on
  // the next frame.
  function note(msg) {
    var stick = atBottom();
    var p = document.createElement("p");
    p.className = "note";
    p.textContent = msg;
    out.appendChild(p);
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
    refreshState();
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

  // Dragging the drawer wider, on a wide screen (IDW-F-0004).
  //
  // Only --drawer-w is set. Everything that has to move with it already reads
  // that variable: the drawer's own width, and the min() the reading column and
  // the composer share — so the composer narrows in step without this knowing
  // the composer exists.
  //
  // The width is remembered per browser. The drawer itself deliberately is not
  // (MUS-Q-0057): shut by default means shut. A width is a different kind of
  // thing — it is how this screen suits this person, not a piece of state about
  // what is happening — and re-dragging it every load would be the annoyance
  // the feature was asked for to remove.
  var GRIP_MIN = 224; // 14rem: narrower and a task line has nowhere to go.
  var GRIP_KEY = "mustur.drawer.w";

  function gripMax() {
    // Never more than half the window, and never so wide that the reading
    // column it is pushing has less room than the drawer.
    return Math.max(GRIP_MIN, Math.min(innerWidth * 0.5, 640));
  }

  function setDrawerWidth(px, remember) {
    var w = Math.round(Math.min(gripMax(), Math.max(GRIP_MIN, px)));
    document.body.style.setProperty("--drawer-w", w + "px");
    // The column just changed width, so the composer may have rewrapped.
    measureDock();
    if (remember) {
      try {
        localStorage.setItem(GRIP_KEY, String(w));
      } catch (e) {
        // A private window, or storage refused. The drag still worked; it just
        // will not be there next time.
      }
    }
    return w;
  }

  var grip = document.getElementById("grip");
  if (grip) {
    try {
      var saved = parseInt(localStorage.getItem(GRIP_KEY) || "", 10);
      if (saved > 0) setDrawerWidth(saved, false);
    } catch (e) {
      // Nothing saved, or nothing readable. The stylesheet's own width stands.
    }

    grip.addEventListener("pointerdown", function (e) {
      // Capture, so a fast drag that leaves the 7px strip keeps being ours.
      grip.setPointerCapture(e.pointerId);
      document.body.classList.add("dragging");
      e.preventDefault();
    });
    grip.addEventListener("pointermove", function (e) {
      if (!grip.hasPointerCapture(e.pointerId)) return;
      setDrawerWidth(innerWidth - e.clientX, false);
    });
    var done = function (e) {
      if (!grip.hasPointerCapture(e.pointerId)) return;
      grip.releasePointerCapture(e.pointerId);
      document.body.classList.remove("dragging");
      setDrawerWidth(document.querySelector(".panel").getBoundingClientRect().width, true);
    };
    grip.addEventListener("pointerup", done);
    grip.addEventListener("pointercancel", done);

    // The keyboard reaches it too. A separator that only answers a pointer is
    // one that half the people using it cannot move.
    grip.addEventListener("keydown", function (e) {
      var step = e.shiftKey ? 64 : 16;
      var now = document.querySelector(".panel").getBoundingClientRect().width;
      if (e.key === "ArrowLeft") setDrawerWidth(now + step, true);
      else if (e.key === "ArrowRight") setDrawerWidth(now - step, true);
      else if (e.key === "Home") setDrawerWidth(gripMax(), true);
      else if (e.key === "End") setDrawerWidth(GRIP_MIN, true);
      else return;
      e.preventDefault();
    });

    // The cap is a share of the window, so a window that shrinks has to bring
    // the drawer with it or the terminal is squeezed to nothing.
    addEventListener("resize", function () {
      var now = document.querySelector(".panel").getBoundingClientRect().width;
      if (now > gripMax()) setDrawerWidth(gripMax(), false);
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
      encodeURIComponent(project) + "/ws";

    ws = new WebSocket(url);

    ws.onopen = function () {
      retry = 0;
      attached = true;
      // Before the hello frame arrives lastOutput is this moment, so a session
      // that has been quiet for an hour would flash "running" for one tick.
      // refreshState is called again as soon as hello lands.
      refreshState();
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
      // The badge, on every frame that carries one. A count is sent when the
      // socket opens and again whenever it moves, so it is handled before the
      // frame kinds rather than repeated inside three of them.
      if (typeof f.waiting === "number") setWaiting(f.waiting);
      // Sent with every hello and every screen, so its absence on one of those
      // means there is no prompt rather than that nothing was said.
      if (f.t === "hello" || f.t === "screen") drawPrompt(f.prompt || null);
      if (f.t === "hello") {
        // The first frame carries the screen as it stands, so a reconnect
        // paints immediately rather than waiting for the session to move.
        if (typeof f.screen === "string") paint(f.screen);
        // The server says how long the screen has already been unchanged, so
        // the counter continues rather than restarting on every page load.
        if (typeof f.quiet === "number") lastOutput = Date.now() - f.quiet * 1000;
        if (typeof f.agent === "string") doing = f.agent;
        drawChips(f.status);
        // Now that the real silence is known, the pill can be honest about it.
        refreshState();
      } else if (f.t === "screen") {
        paint(f.screen || "");
        if (typeof f.agent === "string") doing = f.agent;
        drawChips(f.status);
        // A frame only arrives when the screen actually changed, so its arrival
        // is the activity. There is no replay to tell apart any more: the
        // server has no backlog to send.
        lastOutput = Date.now();
        refreshState();
      } else if (f.t === "error") {
        // The server discarded a message and said so. The draft is put back,
        // because the alternative — which shipped — was the text vanishing and
        // a pill changing colour.
        note("not sent: " + (f.error || "unknown reason"));
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
        attached = false;
        setState(f.at ? "ended " + f.at : "ended", false);
        if (foot) foot.textContent = "Nothing is running. Output is kept until you start another.";
        // The box stays writable: MUS-Q-0018 is that the composer is always
        // writable, and a dropped connection is exactly when someone is most
        // likely to be mid-sentence. Only the look dims.
        if (form) form.style.opacity = ".6";
        ws.close();
      }
    };

    ws.onclose = function () {
      attached = false;
      doing = "";
      if (closed) return;
      // A dropped connection is not a dropped session, and the label says so.
      setState("reconnecting", false);
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
    // Enter sends where there is a shift key to hold, and makes a newline where
    // there is not (MUS-Q-0067). A soft keyboard has no shift, so Enter-sends
    // everywhere would take multi-line off the phone entirely -- which is the
    // surface this box exists for. The query is the closest a browser gets to
    // asking whether a physical keyboard is present; it is read at each
    // keystroke rather than cached, so a tablet that gains one changes with it.
    //
    // The Send button stays on every device either way. It is the phone's
    // submit and the desktop's second route to the same thing, and a control
    // that comes and goes with a media query is a control nobody trusts.
    var deskKeys = window.matchMedia
      ? window.matchMedia("(hover: hover) and (pointer: fine)")
      : null;
    text.addEventListener("keydown", function (e) {
      if (e.key !== "Enter") return;
      // Composing in an IME: Enter is choosing a candidate, not sending.
      if (e.isComposing || e.keyCode === 229) return;
      // The modifier still sends anywhere, including the touch screen where
      // plain Enter deliberately does not.
      if (e.metaKey || e.ctrlKey) {
        e.preventDefault();
        if (form) form.dispatchEvent(new Event("submit", { cancelable: true }));
        return;
      }
      if (e.shiftKey || e.altKey) return;
      if (!deskKeys || !deskKeys.matches) return;
      e.preventDefault();
      if (form) form.dispatchEvent(new Event("submit", { cancelable: true }));
    });
  }

  // The prompt the pane is waiting on.
  //
  // MUS-Q-0077: in front of the session, minimising into the key row. It is
  // over the terminal because a dialog is the only thing that matters while it
  // is up, and it minimises rather than closes because the pane underneath is
  // what you minimise it to read.
  //
  // Rebuilt only when the prompt actually changes. A screen frame arrives every
  // time the pane redraws -- a cursor blink is a redraw -- and rebuilding on
  // each one would throw away the minimised state a few times a second.
  var dlg = document.getElementById("dlg");
  var dlgT = document.getElementById("dlgt");
  var dlgB = document.getElementById("dlgb");
  var dlgO = document.getElementById("dlgo");
  var dlgK = document.getElementById("dlgk");
  var dlgMin = document.getElementById("dlgmin");
  var lastPrompt = null;
  var minimised = false;

  function keyButton(cls, key, label) {
    var b = el("button", cls, label);
    b.type = "button";
    b.setAttribute("data-key", key);
    return b;
  }

  // The chip in the key row that brings a minimised prompt back.
  function chip(show, title) {
    if (!keyRow) return;
    var have = keyRow.querySelector(".dlgchip");
    if (!show) {
      if (have && have.parentNode) have.parentNode.removeChild(have);
      return;
    }
    if (!have) {
      have = el("button", "dlgchip");
      have.type = "button";
      have.id = "dlgchip";
      keyRow.insertBefore(have, keyRow.firstChild);
    }
    have.textContent = title || "Prompt";
  }

  function showPrompt() {
    minimised = false;
    if (dlg) dlg.hidden = false;
    chip(false);
    measureDock();
  }

  function hidePrompt(title) {
    minimised = true;
    if (dlg) dlg.hidden = true;
    chip(true, title);
    measureDock();
  }

  function drawPrompt(p) {
    if (!dlg) return;
    var sig = p ? JSON.stringify(p) : "";
    if (sig === lastPrompt) return;
    var wasMinimised = minimised && lastPrompt !== "";
    lastPrompt = sig;

    if (!p) {
      dlg.hidden = true;
      minimised = false;
      chip(false);
      measureDock();
      return;
    }

    dlgT.textContent = p.title || "The session is waiting on you";
    dlgB.textContent = p.body || "";
    dlgO.textContent = "";
    (p.options || []).forEach(function (o) {
      var b = keyButton(o.selected ? "on" : "", o.key, "");
      var n = el("span", "num", o.key);
      b.appendChild(n);
      b.appendChild(document.createTextNode(o.label));
      dlgO.appendChild(b);
    });
    dlgK.textContent = "";
    (p.keys || []).forEach(function (k) {
      dlgK.appendChild(keyButton("", k.key, k.key + " \u00b7 " + k.label));
    });

    // A prompt that changed while minimised stays minimised: the owner put it
    // away to read the pane, and a redraw is not them asking for it back.
    if (wasMinimised) hidePrompt(p.title);
    else showPrompt();
  }

  if (dlgMin) {
    dlgMin.addEventListener("click", function () {
      hidePrompt(dlgT ? dlgT.textContent : "");
    });
  }

  // The key row.
  //
  // A pane can ask for a keypress rather than a sentence -- a dialog to get off,
  // a list to move down, a turn to interrupt -- and the composer could only ever
  // send a line of text followed by Enter (MUS-F-0080). The owner's case is the
  // last of those: noticing an agent misreading them and wanting to stop it and
  // correct it, which in the terminal is Escape.
  //
  // Delegated from the row rather than bound per button, and the row is outside
  // the form on purpose: a button inside it submits it.
  var keyRow = document.getElementById("keys");
  if (keyRow) {
    keyRow.addEventListener("click", function (e) {
      // The chip is in this row and is not a key: it brings the prompt back.
      if (e.target.closest && e.target.closest(".dlgchip")) {
        showPrompt();
        return;
      }
      var b = e.target.closest ? e.target.closest("button[data-key]") : null;
      if (!b) return;
      if (!ws || ws.readyState !== 1) {
        note("not sent: still reconnecting.");
        return;
      }
      if (closed) return;
      ws.send(JSON.stringify({ t: "key", key: b.getAttribute("data-key") }));
      // Straight back to the box. Pressing Escape to interrupt and then having
      // to reach for the composer is two gestures for one intention, and the
      // whole point of the row is that the correction follows the interrupt.
      if (text) text.focus();
    });
  }

  // The prompt's buttons send the same key frame the row does, so there is one
  // path to a keypress and one place it can be refused.
  [dlgO, dlgK].forEach(function (box) {
    if (!box) return;
    box.addEventListener("click", function (e) {
      var b = e.target.closest ? e.target.closest("button[data-key]") : null;
      if (!b) return;
      if (!ws || ws.readyState !== 1) {
        note("not sent: still reconnecting.");
        return;
      }
      if (closed) return;
      ws.send(JSON.stringify({ t: "key", key: b.getAttribute("data-key") }));
    });
  });

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
        if (!closed) note("not sent: still reconnecting. What you wrote is kept.");
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
