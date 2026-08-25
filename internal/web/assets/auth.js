// The passkey ceremony, and nothing else.
//
// One of four scripts in Mustur and the only one with no alternative:
// `navigator.credentials` is a browser API and cannot be reached from a form.
// Choosing passkeys chose this; it is not a precedent for a fifth.
//
// What it does: ask the server for a challenge, hand it to the authenticator,
// post back what the authenticator signed. It renders nothing, holds no state,
// and every decision is the server's.
(function () {
  "use strict";

  // The account page is the fourth surface this runs on: adding a passkey used
  // to have a page of its own, which was a heading and one button, so the
  // ceremony happens where the passkeys are listed instead (MUS-Q-0047).
  var go = document.getElementById("go") || document.getElementById("addkey");
  var said = document.getElementById("said") || document.getElementById("ceremony");
  if (!go) return;

  // Three modes, two ceremonies. Registering from an invitation and adding a
  // passkey to an account that already exists are both `create`; signing in is
  // `get`. What differs between the first two is only which path is posted to.
  var add = go.id === "addkey";
  var invite = !add && document.body.getAttribute("data-invite") === "1";
  var creating = invite || add;
  var base = add
    ? "/account/passkey"
    : invite
    ? location.pathname.replace(/\/$/, "")
    : "/signin";

  function say(msg) {
    if (!said) return;
    said.textContent = msg;
    said.hidden = false;
  }

  // WebAuthn speaks base64url in JSON and ArrayBuffers in the API, and the
  // conversion is the only fiddly part of this file.
  function toBytes(s) {
    s = s.replace(/-/g, "+").replace(/_/g, "/");
    while (s.length % 4) s += "=";
    var raw = atob(s);
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out.buffer;
  }

  function toText(buf) {
    var bytes = new Uint8Array(buf);
    var s = "";
    for (var i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
    return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }

  function post(url, body) {
    return fetch(url, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: body === undefined ? "{}" : JSON.stringify(body),
    });
  }

  go.addEventListener("click", function () {
    if (!window.PublicKeyCredential) {
      say("This browser cannot use passkeys.");
      return;
    }
    go.disabled = true;
    say(creating ? "Waiting for your device…" : "Choose your passkey…");

    post(base + "/begin")
      .then(function (res) {
        if (!res.ok) throw new Error("begin");
        return res.json();
      })
      .then(function (options) {
        var pk = options.publicKey;
        pk.challenge = toBytes(pk.challenge);
        if (pk.user && pk.user.id) pk.user.id = toBytes(pk.user.id);
        (pk.excludeCredentials || []).forEach(function (c) { c.id = toBytes(c.id); });
        (pk.allowCredentials || []).forEach(function (c) { c.id = toBytes(c.id); });
        return creating
          ? navigator.credentials.create({ publicKey: pk })
          : navigator.credentials.get({ publicKey: pk });
      })
      .then(function (cred) {
        var r = cred.response;
        var body = {
          id: cred.id,
          rawId: toText(cred.rawId),
          type: cred.type,
          response: creating
            ? {
                clientDataJSON: toText(r.clientDataJSON),
                attestationObject: toText(r.attestationObject),
              }
            : {
                clientDataJSON: toText(r.clientDataJSON),
                authenticatorData: toText(r.authenticatorData),
                signature: toText(r.signature),
                userHandle: r.userHandle ? toText(r.userHandle) : null,
              },
        };
        return post(base + "/finish", body);
      })
      .then(function (res) {
        if (!res.ok) throw new Error("finish");
        return res.json();
      })
      .then(function (out) {
        location.href = out.to || "/records";
      })
      .catch(function () {
        // Deliberately one message for every failure. The server already
        // refuses to say which check failed; saying it here would undo that.
        go.disabled = false;
        say(creating
          ? "That did not complete. Try again, or ask for another invitation."
          : "That passkey was not recognised on this site.");
      });
  });
})();
