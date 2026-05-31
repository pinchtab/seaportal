//go:build ignore

// gen-charset-fixtures writes synthetic Latin-1 and Shift-JIS HTML fixtures.
// Run from repo root: `go run scripts/gen-charset-fixtures.go`.
package main

import (
	"log"
	"os"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
)

func main() {
	// Latin-1: accented French phrase, with HTTP-equiv meta declaration.
	latin1HTML := `<!DOCTYPE html>
<html lang="fr">
<head>
<meta http-equiv="Content-Type" content="text/html; charset=ISO-8859-1">
<title>Café français</title>
</head>
<body>
<h1>Le café à Paris</h1>
<p>Voici une histoire à propos d'un café français situé près de la Seine. Les clients préfèrent le café noir avec un croissant frais le matin. Le propriétaire, François, sert également des spécialités régionales très appréciées.</p>
<p>Les habitués viennent ici depuis des décennies pour discuter, lire le journal et déguster une pâtisserie. Le décor est typiquement parisien : tables rondes en marbre, chaises en rotin et grandes vitrines donnant sur la rue.</p>
</body>
</html>
`
	latin1Bytes, err := charmap.ISO8859_1.NewEncoder().Bytes([]byte(latin1HTML))
	if err != nil {
		log.Fatalf("latin1 encode: %v", err)
	}
	if err := os.WriteFile("testdata/static/charset-latin1.html", latin1Bytes, 0o644); err != nil {
		log.Fatalf("write latin1: %v", err)
	}
	log.Printf("wrote testdata/static/charset-latin1.html (%d bytes)", len(latin1Bytes))

	// Shift-JIS: Japanese page with <meta charset> declaration.
	sjisHTML := `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="Shift_JIS">
<title>日本語のページ</title>
</head>
<body>
<h1>日本語の文章</h1>
<p>これは日本語のテストページです。文字化けせずに正しく表示されるかを確認するためのサンプルです。日本語の文章をたくさん書いて、Readabilityが本文として認識できるようにします。</p>
<p>東京や大阪、京都など、日本にはたくさんの素晴らしい都市があります。それぞれの都市には独自の文化と歴史があり、世界中から多くの観光客が訪れています。</p>
</body>
</html>
`
	sjisBytes, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(sjisHTML))
	if err != nil {
		log.Fatalf("sjis encode: %v", err)
	}
	if err := os.WriteFile("testdata/static/charset-shiftjis.html", sjisBytes, 0o644); err != nil {
		log.Fatalf("write sjis: %v", err)
	}
	log.Printf("wrote testdata/static/charset-shiftjis.html (%d bytes)", len(sjisBytes))
}
