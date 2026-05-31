# Group 13 — Internationalization, RTL, alt-script

Tests that extraction preserves non-Latin scripts and right-to-left layouts.

### 13.1 Japanese SSR
Fetch `https://ja.wikipedia.org/wiki/日本`. Report the first heading and the first sentence verbatim. Confirm Japanese characters survived round-trip.
**Verify**: Heading and sentence contain CJK characters; no mojibake.

### 13.2 Arabic / RTL
Fetch `https://ar.wikipedia.org/wiki/مصر`. Report the first heading and the article `length`. Note whether bidi handling produced sensible output.
**Verify**: Heading is Arabic; length > 0; brief note on readability.

### 13.3 Chinese news
Fetch `https://www.zaobao.com/`. Report `pageClass` and 3 headlines verbatim.
**Verify**: 3 Chinese-language headlines OR honest escalation verdict.

### 13.4 Right-to-left news
Fetch `https://www.aljazeera.net/`. Report `pageClass` and 3 headlines.
**Verify**: 3 Arabic headlines OR escalation verdict with reason.

### 13.5 Non-Latin URL
Fetch `https://ru.wikipedia.org/wiki/Москва`. Report whether the URL was accepted and the first heading.
**Verify**: Cyrillic URL handled; Russian heading reported.

### 13.6 Hebrew Wikipedia (RTL)
Fetch `https://he.wikipedia.org/wiki/ירושלים`. Report the first heading and the first sentence verbatim. Confirm Hebrew characters survived round-trip and that RTL produced sensible output.
**Verify**: Heading is Hebrew (`ירושלים` or similar); first sentence quoted; brief note on readability.

### 13.7 Korean Wikipedia (Hangul)
Fetch `https://ko.wikipedia.org/wiki/서울`. Report the first heading and the first sentence verbatim. Confirm Hangul characters survived round-trip.
**Verify**: Heading is Korean (`서울` or similar); first sentence quoted; no mojibake.
