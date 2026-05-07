package notion

import "context"

// walkCursor paginates through a list endpoint by repeatedly calling fetch with
// the next cursor. It stops when hasMore is false or shouldStop returns true for
// an item. shouldStop may be nil (never stop early).
func walkCursor[T any](
	ctx context.Context,
	fetch func(cursor string) (items []T, nextCursor string, hasMore bool, err error),
	shouldStop func(T) bool,
) ([]T, error) {
	var all []T
	cursor := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		items, nextCursor, hasMore, err := fetch(cursor)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if shouldStop != nil && shouldStop(item) {
				return all, nil
			}
			all = append(all, item)
		}
		if !hasMore {
			return all, nil
		}
		cursor = nextCursor
	}
}
