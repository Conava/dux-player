package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The CLI builds the catalog without a daemon; a feature that reads its
// dependencies while registering would panic here.
func TestCatalogBuildsWithoutADaemon(t *testing.T) {
	r := newRegistry(&daemon{})
	for _, name := range []string{"ping", "describe", "player.play", "queue.add", "libraries.list", "browse.search", "settings.set", "ui.show", "marks.setFavorite"} {
		_, ok := r.Find(name)
		assert.True(t, ok, name)
	}
}
