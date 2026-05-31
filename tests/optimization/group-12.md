# Group 12 — Cross-site information gathering

Real research workflow: take one topic, query multiple independent properties, consolidate, and **honestly flag the sources where seaportal couldn't deliver**. Tests the agent's judgment as much as the tool.

### 12.1 News topic across 3 outlets (SSR-friendly)
Topic: "hantavirus". Query the on-site search of three SSR-friendly news properties — `https://www.repubblica.it/ricerca/?query=hantavirus`, `https://www.bbc.co.uk/search?q=hantavirus`, and `https://text.npr.org/search?query=hantavirus`. Report up to 5 headlines per source.
**Verify**: Three sections in the report; each lists 0-5 headlines or a clear escalation note.

### 12.2 Cross-site with at least one expected escalation
Same topic, but include a property where escalation is expected: add `https://edition.cnn.com/search?q=hantavirus`. Report headlines per source AND record which sources were unusable and why.
**Verify**: Honest per-source verdict; CNN flagged escalate (JS search).

### 12.3 Same story, three angles
Pick any current top story (use HN front page to identify one). Find coverage of the same story on two more sources (e.g. one mainstream news, one blog or aggregator). Report the headline + outlet for each.
**Verify**: 3 distinct sources cited with hrefs; same underlying story.

### 12.4 Compare extraction quality
For one Wikipedia article (your choice), fetch it from `en.wikipedia.org`, `simple.wikipedia.org`, and one non-English Wikipedia. Report `length` and the first heading for each.
**Verify**: Three rows; lengths and headings reported.

### 12.5 Fail-fast multi-source
Topic: "compound interest". Query `https://www.investopedia.com/terms/c/compoundinterest.asp`, `https://en.wikipedia.org/wiki/Compound_interest`, and `https://www.bloomberg.com/`. For each, decide use vs escalate **using `--fast`**, and only do a full fetch on the survivors.
**Verify**: Per-source outcome from the fast probe is recorded before any full fetch; total fetch count justified.
