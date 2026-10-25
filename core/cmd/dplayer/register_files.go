package main

import (
	"context"
	"path/filepath"

	"github.com/duxlabs-dev/dux-go/files"

	"github.com/Conava/dux-player/core/internal/ipc"
)

// The browsing half of dux-go/files, for DCommon's file picker (open and
// save dialogs). Nothing that changes files (mkdir, rename, trash) is offered.
func init() {
	var svc *files.Service
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) {
			path := ipc.Req("path", "absolute directory path")
			listing := []ipc.ParamSpec{path, ipc.Opt("includeHidden", "include dot-prefixed and .hidden entries"), ipc.Opt("sort", "name|size|mtime|type"),
				ipc.Opt("desc", "reverse the sort"), ipc.Opt("dirsFirst", "directories before files (default true)"), ipc.Opt("limit", "page size (default 500)"),
				ipc.Opt("filters", "file name globs, case-insensitive; directories always pass")}
			r.Group("files", func(ctx context.Context, w *ipc.ConnWriter, req ipc.Request) {
				if svc == nil {
					w.RespondError(req.ID, "file browsing is not running")
					return
				}
				svc.Handle(ctx, w, req)
			},
				ipc.MethodSpec{Name: "files.list", Desc: "List a directory, or page a live watch when watchId is given", Params: append(listing, ipc.Opt("watchId", "page from this watch instead of re-reading"), ipc.Opt("cursor", "cursor from the previous page")), Read: true},
				ipc.MethodSpec{Name: "files.watch", Desc: "List a directory and stream changes on files:<watchId>", Params: listing},
				ipc.MethodSpec{Name: "files.unwatch", Desc: "Close a watch and its event topic", Params: []ipc.ParamSpec{ipc.Req("watchId", "id from files.watch")}},
				ipc.MethodSpec{Name: "files.stat", Desc: "Metadata for a single path", Params: []ipc.ParamSpec{ipc.Req("path", "absolute path")}, Read: true},
				ipc.MethodSpec{Name: "files.count", Desc: "Child counts for directories, capped at 10000", Params: []ipc.ParamSpec{ipc.Req("paths", "absolute directory paths"), ipc.Opt("includeHidden", "count dot-prefixed entries")}, Read: true},
				ipc.MethodSpec{Name: "files.thumbnail", Desc: "Thumbnail cache paths, generating what is missing", Params: []ipc.ParamSpec{ipc.Req("paths", "absolute file paths"), ipc.Opt("size", "normal|large|x-large|xx-large"), ipc.Opt("watchId", "deliver late results on this watch topic")}, Read: true},
				ipc.MethodSpec{Name: "files.userDirs", Desc: "XDG user directories, home first", Read: true},
				ipc.MethodSpec{Name: "files.icon", Desc: "Icon name ladders for paths or mime types", Params: []ipc.ParamSpec{ipc.Opt("paths", "absolute paths"), ipc.Opt("mimes", "mime types")}, Read: true},
			)
		},
		start: func(d *daemon) error {
			// CacheDir is $XDG_CACHE_HOME/dplayer; thumbnails go to the shared
			// $XDG_CACHE_HOME/thumbnails.
			svc = files.NewService(d.Bus, filepath.Dir(d.CacheDir), nil)
			// Our bus has no per-topic subscribe hook, so watches never buffer:
			// a change in the moment between files.watch and subscribing to its
			// topic is missed.
			svc.AttachOnOpen()
			go func() {
				<-d.Ctx.Done()
				svc.Close()
			}()
			return nil
		},
	})
}
