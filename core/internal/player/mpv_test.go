package player

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMessage(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    message
		reply   bool
		replyOK bool
		wantErr bool
	}{
		{
			name:    "success reply",
			line:    `{"data":{"playlist_entry_id":7},"request_id":3,"error":"success"}`,
			want:    message{RequestID: 3, Error: "success", Data: json.RawMessage(`{"playlist_entry_id":7}`)},
			reply:   true,
			replyOK: true,
		},
		{
			name:  "error reply",
			line:  `{"request_id":4,"error":"property unavailable"}`,
			want:  message{RequestID: 4, Error: "property unavailable"},
			reply: true,
		},
		{
			name: "end-file with reason and entry",
			line: `{"event":"end-file","reason":"error","playlist_entry_id":2,"file_error":"loading failed"}`,
			want: message{Event: "end-file", Reason: "error", EntryID: 2, FileError: "loading failed"},
		},
		{
			name: "property change",
			line: `{"event":"property-change","id":2,"name":"time-pos","data":12.5}`,
			want: message{Event: "property-change", ID: 2, Name: "time-pos", Data: json.RawMessage(`12.5`)},
		},
		{name: "not json", line: `{"event":`, wantErr: true},
		{name: "neither reply nor event", line: `{"data":1}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMessage([]byte(tt.line))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.reply, got.isReply())
			if tt.reply {
				assert.Equal(t, tt.replyOK, got.replyError() == nil)
			}
		})
	}
}

// A fake mpv answers two commands in reverse order with an event in
// between: each caller must get its own reply and the event must queue.
func TestConnMatchesRepliesByRequestID(t *testing.T) {
	client, server := net.Pipe()
	c := newConn(client)
	defer c.close()

	go func() {
		r := bufio.NewReader(server)
		var ids []int64
		for range 2 {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var req struct {
				Command   []any `json:"command"`
				RequestID int64 `json:"request_id"`
			}
			_ = json.Unmarshal(line, &req)
			ids = append(ids, req.RequestID)
		}
		write := func(s string) { _, _ = server.Write([]byte(s + "\n")) }
		write(`{"event":"start-file","playlist_entry_id":9}`)
		b, _ := json.Marshal(map[string]any{"request_id": ids[1], "error": "success", "data": "second"})
		write(string(b))
		b, _ = json.Marshal(map[string]any{"request_id": ids[0], "error": "invalid parameter"})
		write(string(b))
	}()

	type result struct {
		data string
		err  error
	}
	first := make(chan result, 1)
	go func() {
		d, err := c.command(context.Background(), "first")
		first <- result{string(d), err}
	}()
	// Make sure "first" is written before "second".
	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.pending) == 1
	}, time.Second, time.Millisecond)
	d, err := c.command(context.Background(), "second")
	require.NoError(t, err)
	assert.JSONEq(t, `"second"`, string(d))

	r := <-first
	assert.EqualError(t, r.err, "mpv: invalid parameter")

	evs := c.takeEvents()
	require.Len(t, evs, 1)
	assert.Equal(t, int64(9), evs[0].EntryID)
}

func TestConnFailsPendingCommandsWhenMPVGoesAway(t *testing.T) {
	client, server := net.Pipe()
	c := newConn(client)
	go func() {
		_, _ = bufio.NewReader(server).ReadBytes('\n')
		server.Close()
	}()
	_, err := c.command(context.Background(), "get_property", "pause")
	require.Error(t, err)
}
