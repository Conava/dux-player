package source

// Window returns ts[offset:offset+limit], clamped; limit <= 0 is the rest.
func Window[T any](ts []T, offset, limit int) []T {
	offset = min(max(offset, 0), len(ts))
	end := len(ts)
	if limit > 0 {
		end = min(offset+limit, end)
	}
	return ts[offset:end]
}
