package engine

import "testing"

// Per-language paragraphs are short prose chunks rich in function words so
// the stopword classifier has enough signal. They are paraphrased from
// public-domain encyclopedic-style descriptions; no copyrighted text.

func TestDetectLanguage_English(t *testing.T) {
	in := `The quick brown fox jumps over the lazy dog. This is a sentence which has been used for many years to test fonts and keyboards. It contains every letter of the alphabet and is one of the most famous pangrams in the English language. There are other pangrams, but this one would always be the first that comes to mind for those who have ever typed on a typewriter. They have used it for decades.`
	if got := DetectLanguage(in); got != "en" {
		t.Errorf("English: got %q, want %q", got, "en")
	}
}

func TestDetectLanguage_Spanish(t *testing.T) {
	in := `El idioma español es una lengua romance que se habla en muchos países del mundo. Los hablantes nativos son más de quinientos millones, mientras que las personas que lo aprenden como segunda lengua también son numerosas. Esta lengua tiene una rica historia y sus raíces están en el latín vulgar. Durante siglos ha evolucionado, pero todavía conserva muchas palabras de sus orígenes. Por esta razón, son muchos los que la estudian.`
	if got := DetectLanguage(in); got != "es" {
		t.Errorf("Spanish: got %q, want %q", got, "es")
	}
}

func TestDetectLanguage_French(t *testing.T) {
	in := `La langue française est une langue romane parlée par des millions de personnes dans le monde. Les locuteurs natifs sont nombreux et cette langue est aussi enseignée comme langue étrangère dans de nombreux pays. Elle est utilisée pour le commerce, la diplomatie et la culture. Avec ses deux genres grammaticaux, le français peut paraître complexe aux apprenants. Mais avec plus de pratique, ces difficultés deviennent moins importantes.`
	if got := DetectLanguage(in); got != "fr" {
		t.Errorf("French: got %q, want %q", got, "fr")
	}
}

func TestDetectLanguage_German(t *testing.T) {
	in := `Die deutsche Sprache ist eine westgermanische Sprache, die von etwa hundert Millionen Menschen als Muttersprache gesprochen wird. Der Wortschatz ist sehr umfangreich und wird durch zusammengesetzte Wörter ständig erweitert. Sie wird nicht nur in Deutschland gesprochen, sondern auch in Österreich, der Schweiz und anderen Ländern. Mit dem Englischen ist sie eng verwandt, aber die Grammatik ist eine ganz andere. Eine weitere Besonderheit sind die vier grammatischen Fälle.`
	if got := DetectLanguage(in); got != "de" {
		t.Errorf("German: got %q, want %q", got, "de")
	}
}

func TestDetectLanguage_Italian(t *testing.T) {
	in := `La lingua italiana è una lingua romanza parlata principalmente in Italia. Gli italiani che vivono all'estero hanno mantenuto la loro lingua per molte generazioni. Anche nella Svizzera italiana e in altre regioni del mondo si parla questa lingua. Sono molto numerosi i prestiti dalla lingua latina, dalla quale deriva. Questo fatto rende lo studio della lingua italiana interessante per quelli che hanno studiato anche il latino. Della cultura italiana fa parte questa lingua.`
	if got := DetectLanguage(in); got != "it" {
		t.Errorf("Italian: got %q, want %q", got, "it")
	}
}

func TestDetectLanguage_Portuguese(t *testing.T) {
	in := `A língua portuguesa é uma das línguas mais faladas no mundo. Os países lusófonos são vários e estão espalhados por todos os territórios. Mais de duzentos milhões de pessoas têm o português como língua materna. Esse idioma tem origem no latim vulgar, mas também recebeu influências de outras línguas ao longo dos séculos. Depois da colonização, foi levado para o Brasil, onde estão atualmente os falantes mais numerosos. Essa língua tem muitas variações.`
	if got := DetectLanguage(in); got != "pt" {
		t.Errorf("Portuguese: got %q, want %q", got, "pt")
	}
}

func TestDetectLanguage_Dutch(t *testing.T) {
	in := `Het Nederlands is een West-Germaanse taal die wordt gesproken door ongeveer vierentwintig miljoen mensen. Een groot deel van deze mensen woont in Nederland, maar ook in België en Suriname wordt deze taal gesproken. Voor wie de taal wil leren, zijn er veel cursussen beschikbaar. Niet alle dialecten zijn even gemakkelijk te begrijpen, maar de standaardtaal wordt overal verstaan. Deze taal heeft veel leenwoorden, ook uit het Frans en het Engels. Tijdens de gouden eeuw hadden veel handelaren contact met andere landen.`
	if got := DetectLanguage(in); got != "nl" {
		t.Errorf("Dutch: got %q, want %q", got, "nl")
	}
}

func TestDetectLanguage_Russian(t *testing.T) {
	in := `Русский язык — это восточнославянский язык, который является официальным языком Российской Федерации. Он используется как для повседневного общения, так и для научной работы. Очень многие люди изучают его как иностранный, потому что он открывает доступ к богатой литературе. Также этот язык распространён в странах, которые когда-то были частью Советского Союза. Между языками славянской группы существуют большие сходства, но также есть и различия. После распада СССР статус языка изменился во многих странах.`
	if got := DetectLanguage(in); got != "ru" {
		t.Errorf("Russian: got %q, want %q", got, "ru")
	}
}

func TestDetectLanguage_Japanese(t *testing.T) {
	in := `日本語はにほんで話されている言語です。ひらがなとカタカナと漢字を使います。`
	if got := DetectLanguage(in); got != "ja" {
		t.Errorf("Japanese: got %q, want %q", got, "ja")
	}
}

func TestDetectLanguage_Chinese(t *testing.T) {
	in := `中文是世界上使用人数最多的语言之一，主要在中国大陆、台湾和新加坡使用。`
	if got := DetectLanguage(in); got != "zh" {
		t.Errorf("Chinese: got %q, want %q", got, "zh")
	}
}

func TestDetectLanguage_Korean(t *testing.T) {
	in := `한국어는 대한민국과 조선민주주의인민공화국에서 사용되는 공용어입니다.`
	if got := DetectLanguage(in); got != "ko" {
		t.Errorf("Korean: got %q, want %q", got, "ko")
	}
}

func TestDetectLanguage_Arabic(t *testing.T) {
	in := `اللغة العربية هي إحدى أكثر اللغات تحدثاً في العالم، يتحدث بها أكثر من أربعمائة مليون شخص.`
	if got := DetectLanguage(in); got != "ar" {
		t.Errorf("Arabic: got %q, want %q", got, "ar")
	}
}

func TestDetectLanguage_ShortTextReturnsEmpty(t *testing.T) {
	if got := DetectLanguage("Hello world short text!"); got != "" {
		t.Errorf("Short text: got %q, want empty", got)
	}
}

func TestDetectLanguage_CodeOnlyReturnsEmpty(t *testing.T) {
	in := "```go\nfunc foo() int { return 42 }\nvar x = make([]byte, 1024)\n```"
	if got := DetectLanguage(in); got != "" {
		t.Errorf("Code-only: got %q, want empty", got)
	}
}

func TestDetectLanguage_StripsCodeBlocks(t *testing.T) {
	in := "The English prose surrounds the code block with many function words. " +
		"This is a sentence that has been written for the test, and it would always work. " +
		"```de\nDer die das und ist nicht von mit dem den eine einen einer auch werden\n``` " +
		"And the prose continues here with more English text from the writer who likes to test."
	if got := DetectLanguage(in); got != "en" {
		t.Errorf("StripsCodeBlocks: got %q, want %q", got, "en")
	}
}

func TestDetectLanguage_StripsInlineCode(t *testing.T) {
	in := "The function `der die das und ist nicht von mit dem den` returns a value. " +
		"This is the way that you would use it for the test which has been written. " +
		"They have used `eine einen einer auch werden war sind` for many years from the start."
	if got := DetectLanguage(in); got != "en" {
		t.Errorf("StripsInlineCode: got %q, want %q", got, "en")
	}
}

func TestDetectLanguage_StripsMarkdownLinks(t *testing.T) {
	in := "The link [click here](http://der-die-das-und-ist-nicht-von-mit-dem.invalid/einen/einer) does not bias. " +
		"This is the English text that would have been classified as English regardless of the link. " +
		"They have written this for the test, and it has been used to verify that the markdown link strip works."
	if got := DetectLanguage(in); got != "en" {
		t.Errorf("StripsMarkdownLinks: got %q, want %q", got, "en")
	}
}
