package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/tagwrite"
)

// Ratings and favorites of local libraries that turned it on are copied
// into their files' tags.
func init() {
	var w *tagwrite.Writer
	var libs *library.Manager
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) {
			registerMarkFiles(r, func() *tagwrite.Writer { return w }, func() *library.Manager { return libs })
		},
		start: func(d *daemon) error {
			libs = d.Libraries
			w = tagwrite.New(d.Bus.Publish)
			go w.Run(d.Ctx)
			file := func(ref source.Ref) (tagwrite.Item, bool) {
				src, err := d.Libraries.Get(ref.Library)
				if err != nil {
					return tagwrite.Item{}, false
				}
				f, ok := src.(tagwrite.Files)
				if !ok {
					return tagwrite.Item{}, false
				}
				it, err := f.TrackFile(d.Ctx, ref.ID)
				return it, err == nil
			}
			var last source.Ref
			d.Bus.On(d.Ctx, func(topic string, data any) {
				switch topic {
				case player.TopicStatus:
					var ref source.Ref
					if st := data.(player.Status); st.Track != nil {
						ref = st.Track.Ref()
					}
					if ref == last {
						return
					}
					last = ref
					// In order, so the deferred write never misses a track change.
					w.SetCurrent(playingPath(d, ref))
				case ipc.TopicMarks:
					m := data.(map[string]any)
					ref := source.Ref{Library: m["library"].(string), ID: m["id"].(string)}
					if m["kind"] != source.ItemTrack {
						return
					}
					go func() {
						if !tagwrite.Enabled(d.Ctx, d.Libraries, ref.Library) {
							return
						}
						if it, ok := file(ref); ok {
							_, it.Fields.Favorite = m["favorite"]
							_, it.Fields.Rating = m["rating"]
							w.Enqueue(it)
						} else {
							log.Warnf("write marks: no file for track %s", ref.ID)
						}
					}()
				}
			}, player.TopicStatus, ipc.TopicMarks)
			return nil
		},
	})
}

// registerMarkFiles adds the methods; w is nil outside the daemon.
func registerMarkFiles(r *ipc.Registry, w func() *tagwrite.Writer, libs func() *library.Manager) {
	r.Topics(tagwrite.Topic)
	r.Group("marks", func(ctx context.Context, cw *ipc.ConnWriter, req ipc.Request) {
		w, libs := w(), libs()
		if w == nil {
			cw.RespondError(req.ID, "writing marks to files is not available")
			return
		}
		lib := ipc.Str(req.Params, "library")
		switch req.Method {
		case "marks.fileStatus":
			cw.Respond(req.ID, w.Status())
		case "marks.setWriteToFiles":
			on, ok := ipc.Bool(req.Params, "enabled")
			if !ok {
				cw.RespondError(req.ID, "enabled must be true or false")
				return
			}
			if _, err := ipc.Capability[tagwrite.Files](libs, lib, "writing marks to files"); err != nil {
				cw.RespondError(req.ID, err.Error())
				return
			}
			err := libs.SetConfig(ctx, lib, tagwrite.ConfigKey, on)
			if err == nil {
				libs.PublishChanges(ctx)
			}
			ipc.ReplyOK(cw, req.ID, err)
		case "marks.writeFiles":
			f, err := ipc.Capability[tagwrite.Files](libs, lib, "writing marks to files")
			switch {
			case err != nil:
			case !tagwrite.Enabled(ctx, libs, lib):
				err = errors.New("writing marks to files is off for this library")
			case w.Status().Job != nil:
				err = errors.New("marks are being written already")
			}
			if err != nil {
				cw.RespondError(req.ID, err.Error())
				return
			}
			go func() {
				items, err := f.MarkedFiles(context.WithoutCancel(ctx))
				if err != nil {
					log.Warnf("write marks of %s: %v", lib, err)
					w.Fail(lib, tagwrite.CodeListFailed, "Couldn't read the library's marked tracks")
					return
				}
				if err := w.WriteAll(lib, items); err != nil {
					w.Fail(lib, tagwrite.CodeBusy, "Marks are being written already")
				}
			}()
			cw.Respond(req.ID, map[string]bool{"ok": true})
		}
	},
		ipc.MethodSpec{Name: "marks.setWriteToFiles", Desc: "Also write a local library's ratings and favorites to its files' tags; rewrites nothing by itself",
			Params: []ipc.ParamSpec{ipc.Req("library", ""), ipc.Req("enabled", "true|false")}},
		ipc.MethodSpec{Name: "marks.writeFiles", Desc: "Write the existing ratings and favorites of a library to its files, in the background; progress on the markFiles topic",
			Params: []ipc.ParamSpec{ipc.Req("library", "")}},
		ipc.MethodSpec{Name: "marks.fileStatus", Desc: "Pending tag writes, the running pass and the files skipped or failed", Read: true},
	)
}

// playingPath is the local file a track plays from, also for files opened
// outside a library; "" for server tracks, whose stream URL is never read here.
func playingPath(d *daemon, ref source.Ref) string {
	src, err := d.Libraries.Get(ref.Library)
	if ref.ID == "" || err != nil {
		return ""
	}
	if _, remote := src.(source.Pinger); remote {
		return ""
	}
	p, err := src.StreamURL(d.Ctx, ref.ID)
	if err != nil || !filepath.IsAbs(p) {
		return ""
	}
	return p
}
