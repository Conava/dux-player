package tagwrite

import (
	"context"

	"github.com/Conava/dux-player/core/internal/library"
)

// ConfigKey turns writing on for a library, in its library config.
const ConfigKey = "writeMarks"

// Enabled says whether a library writes its marks to its files.
func Enabled(ctx context.Context, libs *library.Manager, lib string) bool {
	l, err := libs.Library(ctx, lib)
	on, _ := l.Config[ConfigKey].(bool)
	return err == nil && on
}
