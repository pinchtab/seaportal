# Group 10 — E-commerce & marketing pages

These sites mix heavy hydration, bot detection, and dense product UI. Mostly tests honest classification more than extraction.

### 10.1 Amazon product page
Fetch `https://www.amazon.com/dp/B08N5WRWNW` (Echo Dot). Report `pageClass`, whether the product title was extracted, and the outcome verdict (use vs escalate).
**Verify**: Honest verdict; if extracted, title contains "Echo Dot".

### 10.2 Etsy listing
Fetch `https://www.etsy.com/`. Report `pageClass` and whether the homepage carousel/category links are present in the snapshot.
**Verify**: Honest report; categories or escalation flagged.

### 10.3 SaaS landing page
Fetch `https://stripe.com/`. Report whether the H1 / hero copy was extracted, and `pageClass`.
**Verify**: Hero text reported or honest "escalate" verdict.

### 10.4 Pricing page extraction
Fetch `https://vercel.com/pricing`. List the plan names you can identify (e.g., Hobby, Pro, Enterprise).
**Verify**: At least 2 plan names reported, OR honest "needs browser" with reason.

### 10.5 Bot-protected ecom
Fetch `https://www.bestbuy.com/`. Report outcome — pass means seaportal correctly flagged `blocked` or extracted real content; fail means it falsely claimed extraction on an empty page.
**Verify**: Outcome matches reality; check `length` and `validation.isValid`.
