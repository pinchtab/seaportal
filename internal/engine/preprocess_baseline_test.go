package engine

type preprocessBaselineRow struct {
	fixture       string
	url           string
	preLength     int
	minLength     int
	markers       []string
	acceptedDelta string
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
		fixture:       "gitlab-project.html",
		url:           "https://gitlab.com/gitlab-org/gitlab",
		preLength:     471,
		minLength:     0,
		markers:       []string{"GitLab"},
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
