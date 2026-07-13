package engine

import "github.com/pinchtab/seaportal/internal/corpus"

type CorpusEntry = corpus.Entry

func LoadCorpus(path string) ([]CorpusEntry, error) {
	return corpus.Load(path)
}
