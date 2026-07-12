// Package engine — corpus loader forwarding.
//
// The eval-corpus loader moved to internal/corpus: it is a benchmark/eval
// helper, not part of the extraction engine, and keeping it here froze the
// engine's exported surface for seabench's sake. These aliases keep the
// engine test suite (classify_test.go, latency_budget_test.go,
// eval_corpus_test.go) compiling; new code should import internal/corpus.
package engine

import "github.com/pinchtab/seaportal/internal/corpus"

// CorpusEntry aliases corpus.Entry for the engine tests.
type CorpusEntry = corpus.Entry

// LoadCorpus forwards to corpus.Load for the engine tests.
func LoadCorpus(path string) ([]CorpusEntry, error) {
	return corpus.Load(path)
}
