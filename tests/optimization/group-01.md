# Group 1 — Reading Wikipedia (SSR, dense)

### 1.1 Infobox extraction
Fetch the Markdown for `https://en.wikipedia.org/wiki/Go_(programming_language)`. Report who designed Go and the year it first appeared.
**Verify**: Designer names include Robert Griesemer; year is 2009.

### 1.2 Multilingual
Fetch `https://fr.wikipedia.org/wiki/Paris`. Report the first paragraph's first sentence (in French).
**Verify**: Sentence is recognizable French prose about Paris.

### 1.3 Simple English variant
Fetch `https://simple.wikipedia.org/wiki/Computer`. Compare its `length` to the regular English article `https://en.wikipedia.org/wiki/Computer` — which is longer?
**Verify**: Both extracted; lengths are compared.

### 1.4 Link discovery
From the Go article, list five outbound links to other Wikipedia articles. Use the interactive snapshot, not full Markdown.
**Verify**: Five links to `/wiki/...` paths are listed with their `href` values.
