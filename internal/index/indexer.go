package index

import (
	"fmt"
	"path/filepath"
	"sync/atomic"

	"github.com/qingyu-org/Gist/internal/config"
	"github.com/rs/zerolog/log"
)

var atomicIndexer atomic.Pointer[Indexer]
var rebuildHook atomic.Value

type Indexer interface {
	Init() error
	Close()
	Reset() error
	Add(gist *Gist) error
	Remove(gistID uint) error
	Search(metadata SearchGistMetadata, userId uint, page int) ([]uint, uint64, map[string]int, error)
}

type IndexerType string

const (
	Bleve       IndexerType = "bleve"
	Meilisearch IndexerType = "meilisearch"
	None        IndexerType = ""
)

func IndexType() IndexerType {
	switch config.C.Index {
	case "bleve":
		return Bleve
	case "meilisearch":
		return Meilisearch
	default:
		return None
	}
}

func IndexEnabled() bool {
	switch config.C.Index {
	case "bleve", "meilisearch":
		return true
	default:
		return false
	}
}

func NewIndexer(idxType IndexerType) {
	if !IndexEnabled() {
		return
	}
	atomicIndexer.Store(nil)

	var idx Indexer

	switch idxType {
	case Bleve:
		idx = NewBleveIndexer(filepath.Join(config.GetHomeDir(), "qingyugist.index"))
	case Meilisearch:
		idx = NewMeiliIndexer(config.C.MeiliHost, config.C.MeiliAPIKey, "qingyugist")
	default:
		log.Warn().Msgf("Failed to create indexer, unknown indexer type: %s", idxType)
		return
	}

	if err := idx.Init(); err != nil {
		return
	}
	atomicIndexer.Store(&idx)

	if bleveNeedsRebuild.Swap(false) {
		if hook, ok := rebuildHook.Load().(func()); ok && hook != nil {
			go hook()
		}
	}
}

// SetRebuildHook registers the function invoked when the on-disk index had to
// be recreated because its mapping changed. It lets the cli reindex every gist
// without making the index package depend on the action scheduler.
func SetRebuildHook(hook func()) {
	rebuildHook.Store(hook)
}

func Close() {
	if !IndexEnabled() {
		return
	}

	idx := atomicIndexer.Load()
	if idx == nil {
		return
	}

	(*idx).Close()
	atomicIndexer.Store(nil)
}

func ResetIndex() error {
	if !IndexEnabled() {
		return nil
	}

	idx := atomicIndexer.Load()
	if idx == nil {
		return fmt.Errorf("indexer is not initialized")
	}

	return (*idx).Reset()
}

func AddInIndex(gist *Gist) error {
	if !IndexEnabled() {
		return nil
	}

	idx := atomicIndexer.Load()
	if idx == nil {
		return fmt.Errorf("indexer is not initialized")
	}

	return (*idx).Add(gist)
}

func RemoveFromIndex(gistID uint) error {
	if !IndexEnabled() {
		return nil
	}

	idx := atomicIndexer.Load()
	if idx == nil {
		return fmt.Errorf("indexer is not initialized")
	}

	return (*idx).Remove(gistID)
}

// SearchGists returns a list of Gist IDs that match the given search metadata.
// If the indexer is not enabled, it returns nil, 0, nil, nil.
// If the indexer is not initialized, it returns nil, 0, nil, fmt.Errorf("indexer is not initialized").
// The function returns an error if any.
func SearchGists(metadata SearchGistMetadata, userId uint, page int) ([]uint, uint64, map[string]int, error) {
	if !IndexEnabled() {
		return nil, 0, nil, nil
	}

	idx := atomicIndexer.Load()
	if idx == nil {
		return nil, 0, nil, fmt.Errorf("indexer is not initialized")
	}

	return (*idx).Search(metadata, userId, page)
}

func DepreactionIndexDirname() {
	if config.C.IndexEnabled {
		log.Warn().Msg("The 'index.enabled'/'QYG_INDEX_ENABLED' configuration option is deprecated and will be removed in a future version. Please use 'index'/'QYG_INDEX' instead.")
	}

	if config.C.Index == "" {
		config.C.Index = "bleve"
	}

	if config.C.BleveDirname != "" {
		log.Warn().Msg("The 'index.dirname'/'QYG_INDEX_DIRNAME' configuration option is deprecated and will be removed in a future version.")
	}
}
