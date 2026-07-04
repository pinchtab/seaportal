# Scrape e2e fixture site

A multi-page static site that exercises the whole `seaportal scrape` pipeline
(discovery → grouping → sampling → fetch → assemble → summary). It is the shared
foundation for the scrape e2e scenarios (ALP-015…020).

## Serving

Static files only — serve the **contents of this directory at a host root** so
`/robots.txt` and `/sitemap.xml` resolve at the top level (robots.txt is always
fetched from the host root). The sitemaps use absolute URLs under
`http://fixtures` to match the `fixtures` service in `tests/e2e/docker-compose.yml`;
mount this directory as that service's nginx root (or a dedicated `fixtures`
root) so the sitemap `<loc>` hosts match the base URL you scrape.

```bash
seaportal scrape http://fixtures/ --max-pages 40
```

## Layout

| Path | Group / purpose |
|------|-----------------|
| `robots.txt` | `Sitemap:` directive + `Disallow: /private/` (tests robots filtering) |
| `sitemap.xml` | **sitemap-index** → `sitemap-blog.xml` + `sitemap-products.xml` |
| `sitemap-blog.xml` | large `/blog/*` group: 300 `post-NNN.html` entries + one missing `post-404.html` (4xx) |
| `sitemap-products.xml` | `/products/*/detail` group (50 entries) + top-level pages |
| `index.html`, `about.html`, `contact.html` | flat top-level pages |
| `blog/post-001.html … post-012.html` | representative articles (JSON-LD `Article`, `og:type=article`) |
| `products/001/detail.html … 008/detail.html` | representative products (JSON-LD `Product`, `og:type=product`) |
| `spa.html` | JS-only page — near-empty HTML body, content injected by script (thin under HTTP extraction → SPA/JS-only recommendation) |
| `private/secret.html` | disallowed by robots.txt — must never appear in results |

## What each part exercises

- **Discovery**: robots.txt `Sitemap:` directive + sitemap-index flattening.
- **Grouping**: hundreds of `/blog/post-NNN.html` collapse to `/blog/*`; numeric
  product ids collapse to `/products/*/detail`.
- **Sampling / "doesn't explode"**: the sitemap lists 356 URLs but only real
  files exist for a representative sample; balanced sampling (sorted) selects the
  low-numbered ids, which are backed by real HTML.
- **Robots**: `/private/secret.html` is listed in a sitemap but disallowed, so it
  must be filtered out.
- **Summary signals**: `spa.html` yields a thin/JS-only page and `post-404.html`
  yields a 4xx, so `summary.recommendations` has content to report.
