# Group 7 — Government & academic

### 7.1 Government
Fetch `https://www.usa.gov/`. Report `pageClass` and the top-level section headings.
**Verify**: Section headings reported.

### 7.2 UK gov
Fetch `https://www.gov.uk/`. Report the first 3 service links from the page.
**Verify**: Three service links with hrefs.

### 7.3 arXiv paper
Fetch `https://arxiv.org/abs/1706.03762`. Report the paper title and authors.
**Verify**: Title is "Attention Is All You Need"; at least 3 authors listed.
Authors are surfaced directly in the extracted Markdown — `byline` carries the
joined list (e.g. "Vaswani, Ashish; Shazeer, Noam; …") and `content` starts
with a `**Authors:** …` line. The `--snapshot --filter=interactive` fallback
remains a valid alternative for cross-checking, but is no longer required.

### 7.4 Publisher
Fetch `https://www.nature.com/`. Honest verdict: extracted or escalate?
**Verify**: Outcome with supporting fields.
