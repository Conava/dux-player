// Package ipc is the daemon's socket protocol: newline-delimited JSON
// requests and replies plus an event stream, on dux-go/ipc.
package ipc

import (
	duxipc "github.com/duxlabs-dev/dux-go/ipc"
)

const (
	AppName    = "dplayer"
	APIVersion = 1

	// TopicPlaylists says a server playlist changed through this player.
	TopicPlaylists = "playlists"
)

type (
	Request    = duxipc.Request
	ConnWriter = duxipc.ConnWriter
	Subscriber = duxipc.Subscriber
	Client     = duxipc.Client
)

var Dial = duxipc.Dial

func FindRunningSocket() (string, error) { return duxipc.FindRunningSocket(AppName) }
