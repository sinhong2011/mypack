package app

import (
	"context"
	"time"
)

// externalWatchInterval is how often an open MyPack looks for database
// writes made by another process (the CLI or MCP). The store is built to
// be polled about once a second. Tests shorten it.
var externalWatchInterval = time.Second

// externalWatcher is the production service's cross-process poll. The
// in-memory demo service does not implement it: nothing else can change
// its state.
type externalWatcher interface {
	WatchExternal(ctx context.Context, interval time.Duration)
}

// watchExternalChanges starts that poll so an already-open Library reloads
// projects, installations and jobs when the CLI or MCP writes the database.
// Those writes publish the same events an in-process change would, and
// onEvent re-reads state. The poll stops when ctx ends or the service
// closes. A service that cannot watch is left alone.
func (a *App) watchExternalChanges(ctx context.Context) {
	w, ok := a.svc.(externalWatcher)
	if !ok {
		return
	}
	go w.WatchExternal(ctx, externalWatchInterval)
}
