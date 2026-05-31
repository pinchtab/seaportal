# Group 4 — Blogs & long-form

### 4.1 Static blog
Fetch `https://danluu.com/`. Report `pageClass` and list five post titles from the index.
**Verify**: `pageClass` is `static`; five titles reported.

### 4.2 Long-form article
Pick any post from the danluu.com index, navigate to it, and report the article's first heading and the approximate word count from the `length` field.
**Verify**: Heading and word count reported.

### 4.3 Personal essays
Fetch `https://www.paulgraham.com/articles.html`. Count how many essays are linked.
**Verify**: A specific number (>50) is reported.

### 4.4 React-rendered blog
Fetch `https://overreacted.io/`. Report `pageClass` and whether the content was extracted successfully.
**Verify**: Honest outcome (likely `hydrated` or `ssr`, should extract).
