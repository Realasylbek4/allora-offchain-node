package lib

import (
	"context"
	"time"
)

// DoneOrWait returns true if ctx.Done() arrived first
func DoneOrWait(ctx context.Context, seconds int64) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(time.Duration(seconds) * time.Second):
		return false
	}
}
