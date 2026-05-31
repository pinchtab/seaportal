# Classifier

`./dev bench classify` reports **40/40** on `tests/eval/corpus.yaml`. That number
is in-corpus: the classifier rules were tuned against those same 40 fixtures, so
it measures fit, not generalization. This page records an **independent held-out**
check — real sites that are in neither the corpus nor `tests/optimization/sites.tsv`
— to see how the classifier does on data it was never shaped by.

Reproduce (live, point-in-time):

```bash
./dev bench sweep --sites tests/optimization/holdout.tsv --timeout 25s
```

Ground-truth labels were assigned by inspecting the actual HTTP fetch: main
content present → `static`/`ssr`/`hydrated`; absent / JS-only → `spa`/`dynamic`;
bot wall → `blocked`. App sites are labelled by their server-rendered marketing
homepage, not the in-app SPA.

## Result (snapshot `2026-05-31T07:46:41Z`, git `bdacbf0`, 31 labelled sites)

| Metric | In-corpus (tuned) | Held-out (this set) |
|---|---|---|
| 6-way pageClass exact-match accuracy | 1.00 (40/40) | **0.71** (22/31) |
| Browser-routing decision (`browserRecommended`) | — | **1.00 (31/31)** |

The 6-way number drops on held-out data — but **every one of the 9 class
disagreements is routing-equivalent**, i.e. it does not change the
extract-vs-browser decision:

| Site | Hand label | Classifier `class` | Routing impact |
|---|---|---|---|
| notion.so, canva.com, cnbc.com, theverge.com, arstechnica.com, walmart.com | ssr | hydrated | none — both `extract` |
| nasa.gov | ssr | static | none — both `extract` |
| imdb.com, espn.com | blocked (HTTP 202 wall) | spa | none — both `needs-browser` |

`ssr`/`static`/`hydrated` all route to **extract**; `spa`/`dynamic`/`blocked` all
route to **needs-browser**. Measured at the decision level — the signal the product
actually routes on — the classifier flagged **7/7** needs-browser sites with **zero
false positives and zero false negatives**.

## Takeaway

This is the empirical backing for the routing guidance in
[api.md](api.md) / [browser-discriminator.md](browser-discriminator.md):
the **browser-routing decision generalizes** (held-out 31/31), while the raw 6-way
`pageClass` (like the `quality` float) is a softer signal that blurs on the
`ssr`/`static`/`hydrated` boundary. Consumers should route on
`profile.decision` / `profile.browserRecommended`, not on the raw class or quality.

Caveats: small sample (31), live and point-in-time (sites change, and a couple of
labels — `imdb`/`espn` bot interstitials — are themselves fetch-dependent), and
two sites (`target.com`, `nginx.com`) were left `any` as unresolved-thin. The set
is a generalization probe, not a fixed regression gate; re-run the sweep to refresh.
