// Two conveniences on the account surface, both optional.
//
// Neither is a feature: without this file the copy button does nothing and the
// role select needs its Save button, which is why that button is still in the
// markup behind a <noscript>. Everything the page can actually do, it does
// with a form post.
(function () {
  "use strict";

  // The invitation link is shown whole and exactly once — it was never stored,
  // so somebody who fails to copy it needs a new invitation, not a second look.
  var copy = document.getElementById("copy");
  if (copy) {
    copy.addEventListener("click", function () {
      var link = copy.getAttribute("data-link") || "";
      var url = link.indexOf("http") === 0 ? link : location.origin + link;
      if (navigator.clipboard) {
        navigator.clipboard.writeText(url).then(
          function () { copy.textContent = "Copied"; },
          select
        );
        return;
      }
      select();
    });
  }

  // No clipboard API, or it refused: select the text so copying is one
  // keystroke rather than a careful drag across a long secret.
  function select() {
    var shown = document.getElementById("invite-link");
    if (shown && window.getSelection) {
      var range = document.createRange();
      range.selectNodeContents(shown);
      var sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
    }
    copy.textContent = "Press copy";
  }

  // Changing a role saves it. The drawing has one control per row, not a
  // select and a button, and a role left unsaved because nobody pressed a
  // second thing is a permission that silently did not change.
  var saves = document.querySelectorAll("select[data-save]");
  for (var i = 0; i < saves.length; i++) {
    saves[i].addEventListener("change", function (e) {
      e.target.form.submit();
    });
  }
})();
