package engine

import "testing"

// regression: authwall-content-rich-social-login-wall — shape mirrors a real
// LinkedIn logged-out fetch (title "Log In or Sign Up", 13 paragraphs, ~2.1KB,
// 3 auth links). Its marketing prose defeats the thin-body signals, so it
// slipped through as usable `ssr`; title (6) + auth-link (5) now reach quorum.
func TestDetectAuthWall_ContentRichSocialWallFires(t *testing.T) {
	linkedIn := Result{
		URL:            "https://www.linkedin.com/",
		Title:          "Log In or Sign Up",
		ParagraphCount: 13,
		Length:         2166,
		Content: "Welcome to your professional community. " +
			"Join now to build your network. [Sign in](https://www.linkedin.com/login) " +
			"or [Join now](https://www.linkedin.com/signup) to continue. " +
			"[Sign in](https://www.linkedin.com/login) to see who's viewed your profile.",
	}
	got, reason := detectAuthWallByContent(linkedIn)
	if !got {
		t.Fatalf("expected LinkedIn-shaped logged-out wall to be detected, got false")
	}
	if reason != "auth-wall-content" {
		t.Fatalf("reason = %q, want auth-wall-content", reason)
	}
}

func TestDetectAuthWall_TitlePatterns(t *testing.T) {
	// Pair each title with two auth links so quorum is reached via the title.
	authLinks := " [a](https://x.test/login) [b](https://x.test/signup) "
	fire := []string{
		"Log In or Sign Up",
		"Facebook – log in or sign up",
		"Sign in",
		"Sign in - Google Accounts",
		"Log in to Threads",
		"Sign Up | SomeApp",
	}
	for _, title := range fire {
		got, _ := detectAuthWallByContent(Result{URL: "https://x.test/", Title: title, Content: authLinks, Length: 50})
		if !got {
			t.Errorf("title %q: expected auth-wall, got false", title)
		}
	}

	noFire := []string{
		"How to sign in to GitHub",
		"Signing bonuses explained",
		"Sign language for beginners",
		"The Login Incident: a postmortem",
	}
	for _, title := range noFire {
		got, _ := detectAuthWallByContent(Result{URL: "https://x.test/", Title: title, Content: " [a](https://x.test/login) ", Length: 4000, ParagraphCount: 20})
		if got {
			t.Errorf("title %q: expected NO auth-wall (title mentions auth in passing), got true", title)
		}
	}
}

// Guard: a content-rich article with auth links in nav/footer (common on SaaS)
// must NOT be flagged — auth-link dominance alone doesn't reach quorum.
func TestDetectAuthWall_ContentPageWithAuthNavDoesNotFire(t *testing.T) {
	article := Result{
		URL:            "https://blog.example.com/posts/scaling-postgres",
		Title:          "Scaling Postgres to 10M writes/sec",
		ParagraphCount: 24,
		Length:         18000,
		Content: "A deep dive into partitioning and connection pooling. " +
			"... long article body ... " +
			"[Sign in](https://app.example.com/login) [Sign up](https://app.example.com/signup)",
	}
	if got, _ := detectAuthWallByContent(article); got {
		t.Fatalf("content-rich article with auth nav links should NOT be flagged as auth-wall")
	}
}
