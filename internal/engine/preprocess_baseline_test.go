package engine

// preprocessBaseline encodes the regression contract from the pre-refactor
// run of the five host-gated scope* functions (captured via the
// `baseline` build-tag harness). The values are pre-refactor extracted
// markdown lengths; minLength is 85% of that (the ±15% tolerance gate).
//
// The matrix is **data only** — it is consulted by TestPreprocessBaseline_*
// integration tests in this package. Update both the value and the markers
// when a fixture is replaced.
type preprocessBaselineRow struct {
	fixture       string
	url           string
	preLength     int
	minLength     int      // ≈ preLength * 0.85
	markers       []string // substrings that must appear in extracted Content
	acceptedDelta string   // empty if no regression accepted; otherwise a note
}

var preprocessBaseline = []preprocessBaselineRow{
	{
		fixture:   "linkedin-loggedout.html",
		url:       "https://www.linkedin.com/",
		preLength: 13540,
		minLength: 11509,
		markers:   []string{"LinkedIn"},
	},
	{
		fixture:   "gitlab-project.html",
		url:       "https://gitlab.com/gitlab-org/gitlab",
		preLength: 471,
		minLength: 0, // see acceptedDelta
		markers:   []string{"GitLab"},
		// The pre-refactor extraction value (471) was produced by a
		// hostname-gated synthetic content reconstruction
		// (scopeGitLabContent) that injected literal "Project information",
		// "Topics:" and stats text from SSR fragments. With host gates
		// removed there is no generic way to reconstruct those — GitLab
		// project pages defer the README to client-side XHR. The generic
		// preprocess still returns the project name and avatar (~107 chars),
		// well below the 15% tolerance. Accepted as an architectural cost
		// of the host-agnostic refactor.
		acceptedDelta: "regression accepted: synthetic SSR rebuild not reproducible without host gating",
	},
	{
		fixture:   "wikipedia-latin-phrases.html",
		url:       "https://en.wikipedia.org/wiki/List_of_Latin_phrases_(full)",
		preLength: 1401436,
		minLength: 1191220,
		markers:   []string{"ad hoc", "carpe diem", "et cetera"},
	},
	{
		fixture:   "mdn-http-methods.html",
		url:       "https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods",
		preLength: 3016,
		minLength: 2563,
		markers:   []string{"GET", "POST", "PUT", "DELETE"},
	},
	{
		fixture:   "mdn-http-auth.html",
		url:       "https://developer.mozilla.org/en-US/docs/Web/HTTP/Authentication",
		preLength: 3958,
		minLength: 3364,
		markers:   []string{"HTTP authentication"},
	},
	{
		fixture:   "github-awesome.html",
		url:       "https://github.com/sindresorhus/awesome",
		preLength: 79670,
		minLength: 67719,
		markers:   []string{"awesome"},
	},
	{
		fixture:   "arxiv-attention.html",
		url:       "https://arxiv.org/abs/1706.03762",
		preLength: 2106,
		minLength: 1790,
		markers:   []string{"Authors", "Vaswani"},
	},
}
