package web

import (
	_ "embed"
	"net/http"
)

// The chrome every surface shares: the tab bar, and the rail it becomes.
//
// Six templates carried their own copy of these rules and had already drifted —
// one used a 1px border where the rest used 1.4px, three made the page a
// full-height column and three did not, and one had lost the rule that marks
// the current tab. That drift is how the records surface ended up shipping a
// different bar from every other surface (MUS-Q-0052). Adding a second layout
// to six copies would have doubled it, so the shared half lives here and each
// template keeps only the CSS that is genuinely its own.
//
// **The bar holds the bottom edge.** It used to be an ordinary element in flow,
// sitting after the content and receding as the content grew — worst on the
// session view, where output arrives forever and the thing you navigate by
// walks off the screen while you watch (MUS-F-0032). `margin-top: auto` holds
// it down on a short page and `position: sticky` holds it there on a long one.
//
// **On a wide screen it is a rail instead** (MUS-D-0118). Not a second
// navigation: the same `<nav>`, the same four links, moved by a media query. One
// nav in the DOM at every width means the tab set cannot drift between them,
// and it keeps the promise that every surface but two works with script blocked.
//
// The rail is positioned rather than placed in a grid, deliberately. Grid would
// have been tidier, but making `body` a grid breaks the session view: its
// output pane fills the page with `flex: 1`, which means nothing in a grid
// container, and the composer would lose its bottom edge. Taking the rail out
// of flow leaves every surface's internal layout exactly as it was.
//
//go:embed assets/bar.js
var barJS string

// BarRoutes serves the one script the shared bar needs.
//
// Registered once by the caller rather than by each page type, because the bar
// is shared and a second copy of its script is how MUS-F-0086 happened: the
// session view had its own badge code, so fixing the badge fixed one surface.
func BarRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /assets/bar.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(barJS))
	})
}

const shellCSS = `
  /* Canvas follows color-scheme, so the bar is opaque in both themes without
     this file having to know either one's colour. */
  :root { --paper: Canvas;
          /* The rail's whole width, border included, and the air between it
             and the text. Both are named because the content's left margin is
             computed from them — when they were separate numbers the rail was
             13rem of content plus 1rem of padding plus a border, about 14rem
             wide, sitting on top of a column that started at 13rem. */
          --shell-rail: 13rem; --shell-gutter: 1rem;
          /* Everything the rail and its gutters leave. One gutter each side of
             the content, and the rail itself. */
          --shell-full: calc(100vw - var(--shell-rail) - var(--shell-gutter) * 2);
          /* Tall enough for the bar; asserted against the rendered height in
             TestTheBarIsNotCoveringTheContent rather than eyeballed. */
          --shell-bar: 3rem;
          /* Where a docked bottom section sits. Below the breakpoint it spans
             the screen above the bar; beside a rail it lines up with the
             reading column instead, or it runs underneath the rail and takes
             its first inch of text with it. */
          --shell-dock-offset: var(--shell-bar);
          --shell-dock-left: 0px;
          --shell-dock-width: 100%; }

  /* A column as tall as the screen, with the bar on its bottom edge. Three
     surfaces already did this and three did not. */
  body { display: flex; flex-direction: column;
         min-height: 100vh; min-height: 100dvh; }

  /* MUS-D-0041's four destinations, and they stay put.
     
     Fixed rather than sticky. Sticky put the bar at the end of the document and
     asked the browser to pull it up into view, which works until it does not:
     on a phone the owner had to scroll before the bar appeared at all, because
     a sticky element only slides within its own containing block and the bar is
     the last thing in it. Fixed asks nothing of the layout — the bar is
     anchored to the viewport and has no flow position to be dragged away from.
     
     left/right rather than width, so it spans the frame even on the surfaces
     whose body carries horizontal padding; sticky inherited that padding and
     left the bar inset by a centimetre on intake. */
  nav { display: flex; white-space: nowrap;
        position: fixed; left: 0; right: 0; bottom: 0; z-index: 2;
        background: var(--paper);
        border-top: 1.4px solid var(--edge); }

  /* The room the bar occupies, given back to the page. A pseudo-element rather
     than padding on body: padding would have to fight each surface's own box,
     and one of them (intake) sets padding on all four sides. In the session
     view's capped column this is what keeps the composer above the bar rather
     than under it. */
  body::after { content: ""; display: block; flex: none;
                height: var(--shell-bar); }
  /* Below the breakpoint the nav is a bar of four, and a fifth entry would
     squeeze the four that name where you are. The account link stays in the
     header, in words. */
  nav a.me { display: none; }
  nav a { flex: 1; padding: .7rem .25rem; font-size: .85em;
          text-decoration: none; color: inherit; opacity: .6;
          display: flex; align-items: center; justify-content: center; }
  nav a.here { opacity: 1; font-weight: 600; }
  /* The bar carries the drawing and the rail carries both. The word is still
     in the markup either way and every tab names itself in aria-label, so what
     leaves the bar is the word on screen and not the word on the page. */
  nav a > span { display: none; }
  /* The count stays visible in the bar, where the word does not. How many
     decisions are waiting is the one thing on this row worth reading at a
     glance, and hiding it with the word would have been the drawing costing
     something rather than replacing something. */
  nav a .cnt { font-style: normal; font-size: .8em; font-weight: 600;
               border: 1px solid var(--accent, currentColor); border-radius: 999px;
               padding: 0 .3rem; margin-left: .3rem;
               background: var(--accent-soft, #8881); }

  /* The five icons.

     Borders, radii and two pseudo-elements each. Not a style preference: the
     tool the owner reviews these in refuses SVG in every block it has, and an
     icon approved as a picture of something else is the wrong kind of
     approval. It suits the binary too — nothing embedded, no viewBox to keep
     in step with a stroke width, and currentColor on a border inherits the
     theme for free.

     One 22px box and one 1.7px border across all five, so they read as a set.
     Nothing is filled with a colour of its own: the first speech bubble had a
     white tail, which is a white block on a dark page. */
  nav .ic { position: relative; box-sizing: border-box; flex: none;
            width: 22px; height: 22px; display: block; }

  /* Sessions: a prompt. */
  nav .ic-sess::before { content: ""; position: absolute; left: 3px; top: 5.5px;
      width: 7px; height: 7px; box-sizing: border-box;
      border-right: 1.7px solid currentColor; border-bottom: 1.7px solid currentColor;
      transform: rotate(-45deg); }
  nav .ic-sess::after { content: ""; position: absolute; right: 2px; bottom: 4px;
      width: 8px; height: 1.7px; background: currentColor; }

  /* Decisions: a question in a circle. The character is the markup's, so the
     glyph follows whatever the page is set in. */
  nav .ic-dec { border: 1.7px solid currentColor; border-radius: 50%;
      display: flex; align-items: center; justify-content: center;
      font-size: 12px; font-weight: 700; line-height: 1; }

  /* Intake: a speech bubble with three dots.

     Chosen over a tray, an envelope and every kind of arrow. A downward arrow
     is the download glyph and an envelope is something that arrives; this page
     is the opposite of both — you write a thought and post it.

     An ellipse with a tail, and the layering is the whole trick.

     A filled tail drawn over an outlined bubble always shows its top edge
     inside the outline — a wedge cutting across the interior. Two versions
     shipped with that before it was measured at six times size and became
     obvious. So the order is: the tail paints first, the bubble paints over it
     with an opaque fill and hides the half that is inside, and the dots paint
     last on top of the fill. Nothing has to line up with a curve.

     The fill is Canvas, which is the same system colour the surface's own
     background resolves to, so it follows the theme. That is what an earlier
     version got wrong by writing #fff, which is a white block on a dark page.

     The bubble is a child rather than the element's own border, because the
     element has to stay the shared 22px box and the ellipse has to sit high
     inside it — a bubble filling the box leaves the tail nowhere to be, which
     is how one version came out with no tail at all.

     The dots are centred by arithmetic rather than by eye: the group is
     1.9 + 7.2 wide, so it starts half of that left of centre. Guessing put
     them 1.85px off on a 22px icon, which the owner saw. */
  nav .ic-in::before { content: ""; position: absolute; left: 5.4px; top: 11px;
      width: 0; height: 0; border-right: 4.4px solid transparent;
      border-top: 8px solid currentColor; }
  nav .ic-in > b { position: absolute; left: 1px; right: 1px; top: 1.5px;
      height: 13px; box-sizing: border-box; background: Canvas;
      border: 1.7px solid currentColor; border-radius: 50%; }
  nav .ic-in::after { content: ""; position: absolute; left: 50%;
      margin-left: -4.55px; top: 7.1px; width: 1.9px; height: 1.9px;
      border-radius: 50%; background: currentColor;
      box-shadow: 3.6px 0 0 0 currentColor, 7.2px 0 0 0 currentColor; }

  /* Records: a document. */
  nav .ic-rec { border: 1.7px solid currentColor; border-radius: 2.5px; }
  nav .ic-rec::before { content: ""; position: absolute; left: 3.5px; right: 3.5px;
      top: 4px; height: 1.5px; background: currentColor; }
  nav .ic-rec::after { content: ""; position: absolute; left: 3.5px; right: 6.5px;
      top: 8.5px; height: 1.5px; background: currentColor;
      box-shadow: 0 4px 0 0 currentColor; }

  /* The account entry, redrawn the same way so all five match. */
  nav .ic-acc::before { content: ""; position: absolute; left: 50%; top: 3px;
      width: 7.5px; height: 7.5px; margin-left: -3.75px; box-sizing: border-box;
      border: 1.7px solid currentColor; border-radius: 50%; }
  nav .ic-acc::after { content: ""; position: absolute; left: 2.5px; right: 2.5px;
      bottom: 2.5px; height: 7.5px; box-sizing: border-box;
      border: 1.7px solid currentColor; border-bottom: 0; border-radius: 8px 8px 0 0; }

  /* Wide enough for both, so the bar becomes a rail and the bottom edge is
     free. 60rem is derived rather than picked, and it adds up exactly: a 13rem
     rail, 1rem of air, and the 46rem reading column the widest surfaces
     already had. The narrower surfaces reach it at the same moment, which is a
     simpler promise than a breakpoint per page. */
  @media (min-width: 60rem) {
    /* The width the rail leaves, on every surface.

       It used to be a 46rem reading column, with two surfaces capping
       themselves tighter at 40rem. That is the right instinct for prose and it
       was applied to everything: on a 1366px laptop a page was 736px wide with
       406px of nothing beside it, whatever was on it — a terminal, a table of
       people, a queue of questions. The owner asked for the width, first on
       the session view and then on the rest.

       A page whose content wants a narrower measure can still say so by
       setting --shell-content; the default is no longer to assume every page
       is an essay. */
    body { margin-inline: calc(var(--shell-rail) + var(--shell-gutter)) auto;
           max-width: var(--shell-content, var(--shell-full));
           /* Set here rather than on :root so --shell-content resolves to
              whatever this surface's reading column actually is. */
           --shell-dock-left: calc(var(--shell-rail) + var(--shell-gutter));
           --shell-dock-width: var(--shell-content, var(--shell-full)); }
    /* border-box, or the padding and the border are added to the width and the
       rail sits on top of the first inch of every page. */
    nav  { position: fixed; left: 0; top: 0; bottom: 0;
           width: var(--shell-rail); box-sizing: border-box;
           flex-direction: column; align-items: stretch;
           margin: 0; padding: .75rem .5rem; gap: .15rem;
           overflow-y: auto;
           border-top: 0; border-right: 1.4px solid var(--edge); }
    nav a { flex: none; padding: .55rem .8rem; border-radius: .5rem;
            justify-content: flex-start; gap: .55rem; }
    /* The rail has room for both, and the pairing is what makes the drawing
       learnable on a phone where the word is gone. */
    nav a > span { display: inline; }
    nav a.here { background: var(--accent-soft, #8881); }
    /* The account entry, at the foot of the rail.

       margin-top: auto is what puts it there: the rail is a column flex, so one
       item claiming the free space above itself sinks to the bottom without a
       position or a height being named anywhere.

       It is an icon here and words in the bar, because the rail has room for a
       glyph to sit alone and the bar does not — and because the owner asked for
       exactly that. The label is on the element rather than beside it, so a
       screen reader gets the word either way. */
    nav a.me { display: flex; align-items: center; justify-content: flex-start;
               margin-top: auto; padding: .55rem .8rem; }
    /* The header link is the same destination said twice on a wide screen. */
    .acct { display: none; }
    /* No bar, so no room reserved for one and nothing under a docked
       section. */
    body::after { display: none; }
    body { --shell-dock-offset: 0px; }
  }
`
