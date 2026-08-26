package web

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
const shellCSS = `
  /* Canvas follows color-scheme, so the bar is opaque in both themes without
     this file having to know either one's colour. */
  :root { --paper: Canvas; }

  /* A column as tall as the screen, with the bar on its bottom edge. Three
     surfaces already did this and three did not. */
  body { display: flex; flex-direction: column;
         min-height: 100vh; min-height: 100dvh; }

  /* MUS-D-0041's four destinations, and they stay put. dvh rather than vh
     because a phone's URL bar makes 100vh taller than what you can see, which
     would tuck the bar behind the browser's own chrome — the failure this is
     meant to end. */
  nav { display: flex; white-space: nowrap; margin-top: auto;
        position: sticky; bottom: 0; z-index: 2;
        background: var(--paper);
        border-top: 1.4px solid var(--edge); }
  nav a { flex: 1; padding: .7rem .25rem; text-align: center; font-size: .85em;
          text-decoration: none; color: inherit; opacity: .6; }
  nav a.here { opacity: 1; font-weight: 600; }

  /* Wide enough for both, so the bar becomes a rail and the bottom edge is
     free. 60rem is derived rather than picked: the widest reading column here
     is 46rem, a 13rem rail beside it needs 59rem, and 60 is the first round
     width at which the column survives the rail unchanged. The narrower
     surfaces reach it at the same moment, which is a simpler promise than a
     breakpoint per page. */
  @media (min-width: 60rem) {
    body { margin-inline: 13rem auto;
           max-width: var(--shell-content, 46rem); }
    nav  { position: fixed; left: 0; top: 0; bottom: 0; width: 13rem;
           flex-direction: column; align-items: stretch;
           margin: 0; padding: .75rem .5rem; gap: .15rem;
           overflow-y: auto;
           border-top: 0; border-right: 1.4px solid var(--edge); }
    nav a { flex: none; text-align: left; padding: .55rem .8rem;
            border-radius: .5rem; }
    nav a.here { background: var(--accent-soft, #8881); }
  }
`
