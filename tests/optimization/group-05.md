# Group 5 — Code hosting

### 5.1 GitHub repo (HTML)
Fetch `https://github.com/golang/go`. Report whether the README content is present in the extracted Markdown. Report `pageClass`.
**Verify**: Honest answer about README presence; `pageClass` recorded.

### 5.2 Raw markdown
Fetch `https://raw.githubusercontent.com/golang/go/master/README.md`. Report the first heading.
**Verify**: First heading is from the actual Go README.

### 5.3 Compare HTML vs raw
For the same repo: which mode gave more usable Markdown content — HTML or raw? Report `length` for both.
**Verify**: Both lengths reported.

### 5.4 Repo with rich README (CDATA / nested tags)
Fetch `https://github.com/CloakHQ/CloakBrowser`. Report 3 distinct README sections that survived extraction (e.g., Install, Test Results, Comparison).
**Verify**: 3 sections named with their headings.

### 5.5 GitLab equivalent
Fetch `https://gitlab.com/gitlab-org/gitlab`. Report `pageClass` and confirm the project name ("GitLab") appears in the Markdown. Note that the README itself is async-loaded by Vue and won't be present — this is expected, and the host-agnostic preprocess no longer synthesises a "Project information" panel from the SSR fragments.
**Verify**: Project name is present in the extracted Markdown; `pageClass` recorded.
