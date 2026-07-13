package fixture

import (
	"fmt"
	"net/http"
)

func MultiPageSite() *Server {
	s := New()
	base := s.URL()

	robots := fmt.Sprintf("User-agent: *\nDisallow: /private\nSitemap: %s/sitemap.xml\n", base)
	s.Route("GET", "/robots.txt", Body([]byte(robots), "text/plain; charset=utf-8"))

	pages := []string{"/", "/about", "/blog/first-post", "/blog/second-post", "/products/1", "/products/2", "/private/secret"}
	sitemap := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`
	for _, p := range pages {
		sitemap += "<url><loc>" + base + p + "</loc></url>"
	}
	sitemap += `</urlset>`
	s.Route("GET", "/sitemap.xml", Body([]byte(sitemap), "application/xml"))

	s.Route("GET", "/{$}", Body([]byte(`<html><head><title>Home</title></head><body>`+
		`<h1>Welcome</h1>`+
		`<a href="/about">About</a>`+
		`<a href="/blog/first-post">First</a><a href="/blog/second-post">Second</a>`+
		`<a href="/products/1">P1</a><a href="/products/2">P2</a>`+
		`<a href="https://external.example/x">External</a>`+
		`</body></html>`), "text/html; charset=utf-8"))

	s.Route("GET", "/about", article("About Us", "Company background and mission."))
	s.Route("GET", "/blog/first-post", article("First Post", "The first article body with plenty of readable words."))
	s.Route("GET", "/blog/second-post", article("Second Post", "The second article body, also with enough words."))
	s.Route("GET", "/products/1", product("Widget One", base+"/products/1"))
	s.Route("GET", "/products/2", product("Widget Two", base+"/products/2"))
	s.Route("GET", "/private/secret", Body([]byte(`<html><head><title>Secret</title></head><body><p>hidden</p></body></html>`), "text/html; charset=utf-8"))

	return s
}

func article(title, body string) http.HandlerFunc {
	html := `<html><head><title>` + title + `</title>` +
		`<meta name="description" content="` + title + ` description">` +
		`<meta property="og:type" content="article">` +
		`<script type="application/ld+json">{"@context":"https://schema.org","@type":"Article","headline":"` + title + `"}</script>` +
		`</head><body><h1>` + title + `</h1><p>` + body + `</p>` +
		`<a href="/about">About</a></body></html>`
	return Body([]byte(html), "text/html; charset=utf-8")
}

func product(name, url string) http.HandlerFunc {
	html := `<html><head><title>` + name + `</title>` +
		`<meta property="og:type" content="product">` +
		`<script type="application/ld+json">{"@context":"https://schema.org","@type":"Product","url":"` + url + `"}</script>` +
		`</head><body><h1>` + name + `</h1><p>A fine product with a detailed description.</p></body></html>`
	return Body([]byte(html), "text/html; charset=utf-8")
}
