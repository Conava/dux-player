// Package lastfm holds the Last.fm API account built into this app.
package lastfm

// APIKey and APISecret are the app's own Last.fm API account, in the source on purpose: a desktop binary can't hide
// them. Without them a user would have to bring an account of their own.
var APIKey, APISecret = "874749853ba717af083324e00494ae6a", "5009a1c899f67a9eae23ef4dacd4858b"
